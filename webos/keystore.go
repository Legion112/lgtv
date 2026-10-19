package webos

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// KeyStore persists pairing keys per TV under the user's config directory,
// so the on-screen prompt only has to be accepted once per TV.
type KeyStore struct{ dir string }

// NewKeyStore returns a store rooted at $XDG_CONFIG_HOME/lgtv (or the
// platform equivalent).
func NewKeyStore() (*KeyStore, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, fmt.Errorf("locate config directory: %w", err)
	}
	return &KeyStore{dir: filepath.Join(base, "lgtv")}, nil
}

// Dir reports where the store keeps its files.
func (s *KeyStore) Dir() string { return s.dir }

// Key returns the stored pairing key for host, or "" if there is none.
func (s *KeyStore) Key(host string) string {
	raw, err := os.ReadFile(s.keyPath(host))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SaveKey stores the pairing key for host with owner-only permissions.
func (s *KeyStore) SaveKey(host, key string) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", s.dir, err)
	}
	path := s.keyPath(host)
	if err := os.WriteFile(path, []byte(key+"\n"), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// Host returns the last host used, so plain `lgtv info` works with no flags.
func (s *KeyStore) Host() string {
	raw, err := os.ReadFile(filepath.Join(s.dir, "host"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(raw))
}

// SaveHost records host as the default for later invocations.
func (s *KeyStore) SaveHost(host string) error {
	if err := os.MkdirAll(s.dir, 0o700); err != nil {
		return fmt.Errorf("create %s: %w", s.dir, err)
	}
	path := filepath.Join(s.dir, "host")
	if err := os.WriteFile(path, []byte(host+"\n"), 0o600); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// keyPath keeps one file per TV. Hosts are IPs or hostnames, so only the
// path separator and leading dots need neutralising.
func (s *KeyStore) keyPath(host string) string {
	safe := strings.NewReplacer("/", "_", `\`, "_", ":", "_").Replace(host)
	safe = strings.TrimLeft(safe, ".")
	if safe == "" {
		safe = "default"
	}
	return filepath.Join(s.dir, safe+".key")
}
