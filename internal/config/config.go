// Package config reads and writes the escli configuration file, which holds
// named profiles. A profile stores the cluster address only; API keys are kept
// by the secret package, never in this file.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
)

const fileName = "config.json"

// DefaultProfile is the profile name used when none is given or selected.
const DefaultProfile = "default"

var profileName = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]{0,62}$`)

// Profile is one cluster that escli can talk to.
type Profile struct {
	URL string `json:"url"`
}

// Config is the content of the configuration file.
type Config struct {
	Current  string             `json:"current_profile,omitempty"`
	Profiles map[string]Profile `json:"profiles"`

	dir string
}

// Dir returns the configuration directory. ESCLI_CONFIG_DIR wins, then
// $XDG_CONFIG_HOME/escli, then %AppData%\escli on Windows and ~/.config/escli
// elsewhere.
func Dir() (string, error) {
	if d := os.Getenv("ESCLI_CONFIG_DIR"); d != "" {
		return d, nil
	}
	if d := os.Getenv("XDG_CONFIG_HOME"); d != "" {
		return filepath.Join(d, "escli"), nil
	}
	if runtime.GOOS == "windows" {
		d, err := os.UserConfigDir()
		if err != nil {
			return "", err
		}
		return filepath.Join(d, "escli"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "escli"), nil
}

// Load reads the configuration in dir. A missing file gives an empty Config.
func Load(dir string) (*Config, error) {
	c := &Config{Profiles: map[string]Profile{}, dir: dir}
	data, err := os.ReadFile(c.Path())
	if errors.Is(err, fs.ErrNotExist) {
		return c, nil
	}
	if err != nil {
		return nil, err
	}
	if err := json.Unmarshal(data, c); err != nil {
		return nil, fmt.Errorf("read %s: %w", c.Path(), err)
	}
	if c.Profiles == nil {
		c.Profiles = map[string]Profile{}
	}
	return c, nil
}

// Path returns the location of the configuration file.
func (c *Config) Path() string {
	return filepath.Join(c.dir, fileName)
}

// Save writes the configuration file with permissions for the owner only.
func (c *Config) Save() error {
	data, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	return WriteFileAtomic(c.Path(), append(data, '\n'))
}

// Lookup finds the profile to use. An empty requested name selects the current
// profile, and if there is none, DefaultProfile.
func (c *Config) Lookup(requested string) (name string, p Profile, ok bool) {
	name = requested
	if name == "" {
		name = c.Current
	}
	if name == "" {
		name = DefaultProfile
	}
	p, ok = c.Profiles[name]
	return name, p, ok
}

// ValidateProfileName reports whether name can be used as a profile name.
func ValidateProfileName(name string) error {
	if !profileName.MatchString(name) {
		return fmt.Errorf("profile name %q must start with a letter or digit and contain only letters, digits, '-' and '_'", name)
	}
	return nil
}

// WriteFileAtomic writes data to path through a temporary file in the same
// directory, so that a crash never leaves a half-written file. The directory is
// created with mode 0700 and the file with mode 0600.
func WriteFileAtomic(path string, data []byte) (err error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return err
	}
	defer func() {
		if err != nil {
			_ = os.Remove(f.Name())
		}
	}()
	if err := f.Chmod(0o600); err != nil && runtime.GOOS != "windows" {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}
