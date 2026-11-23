package secret

import (
	"errors"
	"os"
	"testing"

	"github.com/zalando/go-keyring"
)

func TestKeychainRoundTrip(t *testing.T) {
	keyring.MockInit()
	s := New(t.TempDir())

	if _, err := s.Get("default"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("Get before Set: err = %v, want ErrNotFound", err)
	}
	where, err := s.Set("default", "k1")
	if err != nil {
		t.Fatal(err)
	}
	if where != "the system keychain" {
		t.Errorf("Set stored the key in %q, want the keychain", where)
	}
	if got, err := s.Get("default"); err != nil || got != "k1" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := s.Delete("default"); err != nil {
		t.Fatal(err)
	}
	if err := s.Delete("default"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("second Delete: err = %v, want ErrNotFound", err)
	}
}

func TestFileFallback(t *testing.T) {
	keyring.MockInitWithError(errors.New("no secret service"))
	dir := t.TempDir()
	s := New(dir)

	where, err := s.Set("ci", "k2")
	if err != nil {
		t.Fatal(err)
	}
	if where == "the system keychain" {
		t.Fatal("Set claimed the keychain although it is unavailable")
	}
	if _, err := os.Stat(s.path()); err != nil {
		t.Fatalf("fallback file was not written: %v", err)
	}
	if got, err := s.Get("ci"); err != nil || got != "k2" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	if err := s.Delete("ci"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(s.path()); !os.IsNotExist(err) {
		t.Errorf("empty credentials file should be removed, stat err = %v", err)
	}
}

func TestSetRemovesStaleFileCopy(t *testing.T) {
	dir := t.TempDir()
	keyring.MockInitWithError(errors.New("no secret service"))
	if _, err := New(dir).Set("default", "old"); err != nil {
		t.Fatal(err)
	}

	keyring.MockInit()
	s := New(dir)
	if _, err := s.Set("default", "new"); err != nil {
		t.Fatal(err)
	}
	creds, err := s.readFile()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := creds["default"]; ok {
		t.Error("plain-text copy was kept after the key moved to the keychain")
	}
}
