package es

import (
	"context"
	"encoding/base64"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDoSendsAPIKeyAndBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "ApiKey abc" {
			t.Errorf("Authorization = %q", got)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}
		if r.Method != http.MethodPost || r.URL.Path != "/idx/_search" || r.URL.Query().Get("size") != "1" {
			t.Errorf("request = %s %s", r.Method, r.URL)
		}
		body, _ := io.ReadAll(r.Body)
		if string(body) != `{"q":1}` {
			t.Errorf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	c := New(srv.URL+"/", "abc", 5*time.Second)
	got, err := c.Do(context.Background(), http.MethodPost, "/idx/_search", url.Values{"size": {"1"}}, []byte(`{"q":1}`))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != `{"ok":true}` {
		t.Errorf("response = %s", got)
	}
}

func TestDoErrors(t *testing.T) {
	tests := []struct {
		name   string
		status int
		body   string
		want   []string
	}{
		{"structured", 401, `{"error":{"type":"security_exception","reason":"missing credentials"},"status":401}`,
			[]string{"HTTP 401", "security_exception", "missing credentials", "escli init"}},
		{"string error", 400, `{"error":"bad thing","status":400}`, []string{"HTTP 400", "bad thing"}},
		{"plain body", 404, `{"_index":"x","found":false}`, []string{"HTTP 404", `"found":false`}},
		{"forbidden", 403, `{"error":{"type":"security_exception","reason":"no"}}`, []string{"no privilege"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.status)
				_, _ = w.Write([]byte(tt.body))
			}))
			defer srv.Close()

			_, err := New(srv.URL, "k", time.Second).Do(context.Background(), http.MethodGet, "/", nil, nil)
			if err == nil {
				t.Fatal("want an error")
			}
			if !IsStatus(err, tt.status) {
				t.Errorf("IsStatus(err, %d) = false", tt.status)
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not contain %q", err, w)
				}
			}
		})
	}
}

func TestFormatQuerySkipsAcceptHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Accept"); got != "" {
			t.Errorf("Accept = %q, want none", got)
		}
		_, _ = w.Write([]byte("a,b\n"))
	}))
	defer srv.Close()

	if _, err := New(srv.URL, "k", time.Second).Do(context.Background(), http.MethodPost, "/_query", url.Values{"format": {"csv"}}, []byte(`{}`)); err != nil {
		t.Fatal(err)
	}
}

func TestNormalizeAPIKey(t *testing.T) {
	pair := base64.StdEncoding.EncodeToString([]byte("id1:secret1"))
	tests := map[string]string{
		"  " + pair + "\n": pair,
		"ApiKey " + pair:   pair,
		"id1:secret1":      pair,
		`{"id":"id1","name":"n","api_key":"secret1","encoded":"` + pair + `"}`: pair,
		`{"id":"id1","api_key":"secret1"}`:                                     pair,
	}
	for in, want := range tests {
		got, err := NormalizeAPIKey(in)
		if err != nil {
			t.Errorf("NormalizeAPIKey(%q) error: %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("NormalizeAPIKey(%q) = %q, want %q", in, got, want)
		}
	}
	for _, bad := range []string{"", "   ", "{not json", `{"name":"x"}`, "two words"} {
		if _, err := NormalizeAPIKey(bad); err == nil {
			t.Errorf("NormalizeAPIKey(%q): want an error", bad)
		}
	}
}
