package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/bradh11/escli/internal/config"
	"github.com/zalando/go-keyring"
)

const testKey = "dGVzdDprZXk=" // base64 of "test:key"

// fakeES serves a few Elasticsearch endpoints and records the last request body.
type fakeES struct {
	*httptest.Server
	lastPath string
	lastBody string
}

func newFakeES(t *testing.T) *fakeES {
	t.Helper()
	f := &fakeES{}
	f.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "ApiKey "+testKey {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"error":{"type":"security_exception","reason":"unable to authenticate"},"status":401}`)
			return
		}
		body, _ := io.ReadAll(r.Body)
		f.lastPath, f.lastBody = r.URL.RequestURI(), string(body)
		switch r.URL.Path {
		case "/_security/_authenticate":
			_, _ = io.WriteString(w, `{"username":"pat","authentication_type":"api_key","authentication_realm":{"name":"_es_api_key"},"api_key":{"id":"k1","name":"pat-laptop"}}`)
		case "/deals/_search":
			_, _ = io.WriteString(w, `{"hits":{"total":{"value":2,"relation":"eq"},"hits":[{"_index":"deals","_id":"1","_source":{"name":"Acme","amount":12000}},{"_index":"deals","_id":"2","_source":{"name":"Beta","amount":9000}}]}}`)
		case "/deals/_count":
			_, _ = io.WriteString(w, `{"count":12345678901234567,"_shards":{}}`)
		case "/deals/_mapping":
			_, _ = io.WriteString(w, `{"deals":{"mappings":{"properties":{"name":{"type":"text","fields":{"keyword":{"type":"keyword"}}},"owner":{"properties":{"email":{"type":"keyword"}}},"amount":{"type":"long"}}}}}`)
		case "/_query":
			_, _ = io.WriteString(w, `{"columns":[{"name":"stage","type":"keyword"},{"name":"total","type":"long"}],"values":[["won",21000],["lost",null]]}`)
		default:
			_, _ = io.WriteString(w, `{"path":"`+r.URL.Path+`"}`)
		}
	}))
	t.Cleanup(f.Close)
	return f
}

// run executes escli with args and stdin and returns stdout and stderr.
func run(t *testing.T, stdin string, args ...string) (stdout, stderr string, err error) {
	t.Helper()
	var out, errOut bytes.Buffer
	a := &app{in: strings.NewReader(stdin), out: &out, errOut: &errOut}
	root := a.rootCmd()
	root.SetArgs(args)
	err = root.ExecuteContext(context.Background())
	return out.String(), errOut.String(), err
}

// setup gives each test its own configuration directory and an in-memory keychain.
func setup(t *testing.T) string {
	t.Helper()
	keyring.MockInit()
	dir := t.TempDir()
	t.Setenv("ESCLI_CONFIG_DIR", dir)
	t.Setenv("ESCLI_PROFILE", "")
	t.Setenv("ESCLI_URL", "")
	t.Setenv("ESCLI_API_KEY", "")
	return dir
}

func initProfile(t *testing.T, srv *fakeES, args ...string) {
	t.Helper()
	args = append([]string{"init", "--url", srv.URL, "--api-key-stdin"}, args...)
	if _, stderr, err := run(t, testKey+"\n", args...); err != nil {
		t.Fatalf("init failed: %v\n%s", err, stderr)
	}
}

func TestNotSetUp(t *testing.T) {
	setup(t)
	_, _, err := run(t, "", "indices")
	if err == nil || !strings.Contains(err.Error(), "escli init") {
		t.Fatalf("err = %v, want a hint to run escli init", err)
	}
}

func TestInitInteractive(t *testing.T) {
	dir := setup(t)
	srv := newFakeES(t)

	_, stderr, err := run(t, srv.URL+"\n"+testKey+"\n", "init")
	if err != nil {
		t.Fatalf("init: %v\n%s", err, stderr)
	}
	if !strings.Contains(stderr, "Signed in as pat") {
		t.Errorf("stderr does not confirm the sign-in:\n%s", stderr)
	}
	cfg, err := config.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Current != "default" || cfg.Profiles["default"].URL != srv.URL {
		t.Errorf("config = %+v", cfg)
	}
	data, err := os.ReadFile(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), testKey) {
		t.Error("the API key was written to the config file")
	}
}

func TestInitRejectsBadKey(t *testing.T) {
	dir := setup(t)
	srv := newFakeES(t)

	_, _, err := run(t, "wrong-key\n", "init", "--url", srv.URL, "--api-key-stdin")
	if err == nil || !strings.Contains(err.Error(), "nothing was saved") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); !os.IsNotExist(err) {
		t.Error("a config file was written for a key that did not work")
	}
}

func TestProfiles(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)
	initProfile(t, srv, "--profile", "second")

	out, _, err := run(t, "", "config", "list")
	if err != nil {
		t.Fatal(err)
	}
	var list struct {
		Profiles []struct {
			Name    string `json:"name"`
			Current bool   `json:"current"`
		} `json:"profiles"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatalf("config list output is not JSON: %v\n%s", err, out)
	}
	if len(list.Profiles) != 2 || list.Profiles[0].Name != "default" || list.Profiles[0].Current || !list.Profiles[1].Current {
		t.Errorf("profiles = %+v", list.Profiles)
	}

	if _, _, err := run(t, "", "config", "use", "default"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "", "config", "use", "nope"); err == nil {
		t.Error("config use accepted a missing profile")
	}
	if _, _, err := run(t, "", "--profile", "nope", "info"); err == nil || !strings.Contains(err.Error(), `"nope" does not exist`) {
		t.Errorf("err = %v", err)
	}
}

func TestAuthStatusAndLogout(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	out, _, err := run(t, "", "auth", "status")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"username": "pat"`) || !strings.Contains(out, `"pat-laptop"`) {
		t.Errorf("auth status = %s", out)
	}
	if _, _, err := run(t, "", "auth", "logout"); err != nil {
		t.Fatal(err)
	}
	if _, _, err := run(t, "", "info"); err == nil || !strings.Contains(err.Error(), "no API key stored") {
		t.Errorf("after logout err = %v", err)
	}
}

func TestEnvironmentOverrides(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	t.Setenv("ESCLI_URL", srv.URL)
	t.Setenv("ESCLI_API_KEY", testKey)

	out, _, err := run(t, "", "info")
	if err != nil {
		t.Fatalf("info with env credentials: %v", err)
	}
	if !strings.Contains(out, `"path": "/"`) {
		t.Errorf("info = %s", out)
	}
}

func TestSearch(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	out, _, err := run(t, "", "search", "deals", "-q", "stage:won", "--size", "2", "--fields", "name,amount", "--sort", "amount:desc", "--compact")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"total":2,"hits":[{"_index":"deals","_id":"1","amount":12000,"name":"Acme"},{"_index":"deals","_id":"2","amount":9000,"name":"Beta"}]}` + "\n"
	if out != want {
		t.Errorf("search output\n got: %s\nwant: %s", out, want)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(srv.lastBody), &sent); err != nil {
		t.Fatal(err)
	}
	if sent["size"] != float64(2) || sent["track_total_hits"] != true {
		t.Errorf("request body = %s", srv.lastBody)
	}
	if !strings.Contains(srv.lastBody, `"query_string":{"query":"stage:won"}`) ||
		!strings.Contains(srv.lastBody, `"sort":[{"amount":{"order":"desc"}}]`) ||
		!strings.Contains(srv.lastBody, `"_source":["name","amount"]`) {
		t.Errorf("request body = %s", srv.lastBody)
	}
}

func TestSearchBodyMerge(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	// The body's size wins unless --size is given; the aggregation is kept.
	if _, _, err := run(t, `{"size":0,"aggs":{"a":{"terms":{"field":"stage"}}}}`, "search", "deals", "--body", "-"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(srv.lastBody, `"size":0`) || !strings.Contains(srv.lastBody, `"aggs"`) {
		t.Errorf("request body = %s", srv.lastBody)
	}
	if _, _, err := run(t, `{"size":0}`, "search", "deals", "--body", "-", "--size", "5"); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(srv.lastBody, `"size":5`) {
		t.Errorf("request body = %s", srv.lastBody)
	}
	if _, _, err := run(t, "", "search", "deals", "--body", "{nope"); err == nil {
		t.Error("search accepted invalid JSON")
	}
}

func TestCountKeepsLargeNumbers(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	out, _, err := run(t, "", "count", "deals", "--compact")
	if err != nil {
		t.Fatal(err)
	}
	if out != `{"count":12345678901234567}`+"\n" {
		t.Errorf("count = %s", out)
	}
}

func TestMappingFlat(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	out, _, err := run(t, "", "mapping", "deals", "--flat", "--compact")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"deals":{"amount":"long","name":"text","name.keyword":"keyword","owner.email":"keyword"}}` + "\n"
	if out != want {
		t.Errorf("mapping\n got: %s\nwant: %s", out, want)
	}
}

func TestESQL(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	out, _, err := run(t, "", "esql", "FROM deals | STATS total = SUM(amount) BY stage", "--compact")
	if err != nil {
		t.Fatal(err)
	}
	if out != `[{"stage":"won","total":21000},{"stage":"lost","total":null}]`+"\n" {
		t.Errorf("esql = %s", out)
	}
	if srv.lastBody != `{"query":"FROM deals | STATS total = SUM(amount) BY stage"}` {
		t.Errorf("request body = %s", srv.lastBody)
	}
}

func TestIndexValidation(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	for _, bad := range []string{"../_security", "a b", "x?y"} {
		if _, _, err := run(t, "", "search", bad); err == nil {
			t.Errorf("search accepted index %q", bad)
		}
	}
	if _, _, err := run(t, "", "indices", "logs-*,deals"); err != nil {
		t.Errorf("indices rejected a valid pattern: %v", err)
	}
	if srv.lastPath != "/_resolve/index/logs-*,deals" {
		t.Errorf("path = %s", srv.lastPath)
	}
}

func TestAPIGuard(t *testing.T) {
	setup(t)
	srv := newFakeES(t)
	initProfile(t, srv)

	if _, _, err := run(t, "", "api", "GET", "/_cluster/health?level=indices"); err != nil {
		t.Fatal(err)
	}
	if srv.lastPath != "/_cluster/health?level=indices" {
		t.Errorf("path = %s", srv.lastPath)
	}
	if _, _, err := run(t, "", "api", "DELETE", "/deals"); err == nil || !strings.Contains(err.Error(), "--allow-write") {
		t.Errorf("DELETE err = %v", err)
	}
	if _, _, err := run(t, "", "api", "DELETE", "/deals", "--allow-write"); err != nil {
		t.Errorf("DELETE with --allow-write: %v", err)
	}
	if _, _, err := run(t, "", "api", "GET", "https://elsewhere.example/"); err == nil {
		t.Error("api accepted a full URL")
	}
}

func TestIsReadOnly(t *testing.T) {
	tests := []struct {
		method, path string
		want         bool
	}{
		{"GET", "/anything", true},
		{"HEAD", "/deals", true},
		{"POST", "/deals/_search", true},
		{"POST", "/_query", true},
		{"POST", "/deals/_count", true},
		{"POST", "/_search/template", true},
		{"POST", "/deals/_doc", false},
		{"POST", "/deals/_update/_search", false},
		{"POST", "/deals/_delete_by_query", false},
		{"POST", "/_bulk", false},
		{"PUT", "/deals", false},
		{"DELETE", "/deals", false},
	}
	for _, tt := range tests {
		if got := isReadOnly(tt.method, tt.path); got != tt.want {
			t.Errorf("isReadOnly(%s %s) = %v, want %v", tt.method, tt.path, got, tt.want)
		}
	}
}
