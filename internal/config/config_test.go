package config

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestLoadMissingFile(t *testing.T) {
	c, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Profiles) != 0 || c.Current != "" {
		t.Fatalf("want empty config, got %+v", c)
	}
}

func TestSaveAndLoad(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nested", "escli")
	c, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	c.Profiles["work"] = Profile{URL: "https://es.example.com"}
	c.Current = "work"
	if err := c.Save(); err != nil {
		t.Fatal(err)
	}

	if runtime.GOOS != "windows" {
		info, err := os.Stat(c.Path())
		if err != nil {
			t.Fatal(err)
		}
		if perm := info.Mode().Perm(); perm != 0o600 {
			t.Errorf("config file mode = %o, want 600", perm)
		}
	}

	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Current != "work" || got.Profiles["work"].URL != "https://es.example.com" {
		t.Fatalf("round trip lost data: %+v", got)
	}
}

func TestLookup(t *testing.T) {
	c := &Config{Profiles: map[string]Profile{
		"default": {URL: "https://a"},
		"other":   {URL: "https://b"},
	}}

	if name, p, ok := c.Lookup(""); !ok || name != "default" || p.URL != "https://a" {
		t.Errorf("empty lookup without current = %q %v %v", name, p, ok)
	}
	c.Current = "other"
	if name, _, ok := c.Lookup(""); !ok || name != "other" {
		t.Errorf("empty lookup with current = %q %v", name, ok)
	}
	if name, _, ok := c.Lookup("default"); !ok || name != "default" {
		t.Errorf("explicit lookup = %q %v", name, ok)
	}
	if _, _, ok := c.Lookup("missing"); ok {
		t.Error("lookup of a missing profile reported ok")
	}
}

func TestValidateProfileName(t *testing.T) {
	for _, ok := range []string{"default", "prod-2", "team_a", "X"} {
		if err := ValidateProfileName(ok); err != nil {
			t.Errorf("%q: unexpected error %v", ok, err)
		}
	}
	for _, bad := range []string{"", "-x", "a b", "a/b", "ü"} {
		if err := ValidateProfileName(bad); err == nil {
			t.Errorf("%q: want an error", bad)
		}
	}
}
