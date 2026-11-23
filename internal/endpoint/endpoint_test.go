package endpoint

import (
	"encoding/base64"
	"testing"
)

func TestResolve(t *testing.T) {
	cloudID := "my-deploy:" + base64.StdEncoding.EncodeToString([]byte("us-east-2.aws.elastic-cloud.com:443$abc123$def456"))
	cloudIDPort := "my-deploy:" + base64.StdEncoding.EncodeToString([]byte("example.com:9243$abc123$def456"))

	tests := []struct {
		name, in, want string
		wantNote       bool
	}{
		{"kibana url is converted", "https://acme-x1.kb.us-east-2.aws.elastic.cloud/", "https://acme-x1.es.us-east-2.aws.elastic.cloud", true},
		{"kibana page path is dropped", "https://acme-x1.kb.us-east-2.aws.elastic.cloud/app/discover#/view", "https://acme-x1.es.us-east-2.aws.elastic.cloud", true},
		{"es url is kept", "https://acme-x1.es.us-east-2.aws.elastic.cloud", "https://acme-x1.es.us-east-2.aws.elastic.cloud", false},
		{"found.io kibana url", "https://dep.kb.us-east-1.aws.found.io:9243", "https://dep.es.us-east-1.aws.found.io:9243", true},
		{"scheme is added", "acme-x1.es.us-east-2.aws.elastic.cloud", "https://acme-x1.es.us-east-2.aws.elastic.cloud", false},
		{"self-hosted keeps path", "http://localhost:9200/proxy/", "http://localhost:9200/proxy", false},
		{"host and port without scheme", "localhost:9200", "https://localhost:9200", false},
		{"credentials are stripped", "https://user:pw@es.internal:9200", "https://es.internal:9200", false},
		{"cloud id", cloudID, "https://abc123.us-east-2.aws.elastic-cloud.com", true},
		{"cloud id with port", cloudIDPort, "https://abc123.example.com:9243", true},
		{"surrounding spaces", "  https://es.internal  ", "https://es.internal", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, note, err := Resolve(tt.in)
			if err != nil {
				t.Fatalf("Resolve(%q) error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Errorf("Resolve(%q) = %q, want %q", tt.in, got, tt.want)
			}
			if (note != "") != tt.wantNote {
				t.Errorf("Resolve(%q) note = %q, want note: %v", tt.in, note, tt.wantNote)
			}
		})
	}
}

func TestResolveErrors(t *testing.T) {
	for _, in := range []string{"", "   ", "ftp://es.internal", "https://"} {
		if got, _, err := Resolve(in); err == nil {
			t.Errorf("Resolve(%q) = %q, want an error", in, got)
		}
	}
}

func TestKibanaURL(t *testing.T) {
	tests := map[string]string{
		"https://acme-x1.es.us-east-2.aws.elastic.cloud": "https://acme-x1.kb.us-east-2.aws.elastic.cloud",
		"https://dep.es.us-east-1.aws.found.io:9243":     "https://dep.kb.us-east-1.aws.found.io:9243",
		"https://es.internal:9200":                       "",
		"https://abc123.us-east-2.aws.elastic-cloud.com": "",
	}
	for in, want := range tests {
		if got := KibanaURL(in); got != want {
			t.Errorf("KibanaURL(%q) = %q, want %q", in, got, want)
		}
	}
}
