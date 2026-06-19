package config

import (
	"fmt"
	"os"
	"sync"

	"github.com/BurntSushi/toml"
)

// ConfigRepository is an instance-scoped config access point (T10 god-object
// minimal governance). It replaces implicit reliance on the package-level
// configMu / ConfigPath globals for NEW code: each repository owns its path,
// mutex, and filesystem, so tests do not share process-level state and multiple
// instances can coexist.
//
// This is intentionally minimal: it does NOT migrate the existing surgical-save
// helpers (SaveActiveProvider / SaveProviderModel / etc.), which still use the
// globals. Those are marked Deprecated; new config read/write should go through
// a ConfigRepository instance.
type ConfigRepository struct {
	path string
	mu   sync.Mutex
	fs   configFS
}

// configFS is the minimal filesystem abstraction ConfigRepository uses, so
// tests can inject an in-memory fs instead of touching real disk.
type configFS interface {
	ReadFile(name string) ([]byte, error)
	WriteFile(name string, data []byte, perm os.FileMode) error
}

// osFS is the production filesystem backed by the os package.
type osFS struct{}

func (osFS) ReadFile(name string) ([]byte, error)        { return os.ReadFile(name) }
func (osFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	return os.WriteFile(name, data, perm)
}

// NewConfigRepository creates a ConfigRepository backed by the real filesystem
// at path. New code should prefer this over reading/mutating ConfigPath +
// configMu directly.
func NewConfigRepository(path string) *ConfigRepository {
	return &ConfigRepository{path: path, fs: osFS{}}
}

// newConfigRepositoryWithFS is the test-injectable constructor (unexported so
// only same-package tests use it; production uses NewConfigRepository).
func newConfigRepositoryWithFS(path string, fs configFS) *ConfigRepository {
	return &ConfigRepository{path: path, fs: fs}
}

// Path returns the config file path this repository is bound to.
func (r *ConfigRepository) Path() string { return r.path }

// Read loads and parses the config file into a fresh Config. Returns an error
// if the path is unset or the file cannot be read/parsed.
func (r *ConfigRepository) Read() (*Config, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		return nil, fmt.Errorf("config path not set")
	}
	data, err := r.fs.ReadFile(r.path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	cfg := &Config{}
	if err := toml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return cfg, nil
}

// WriteRaw persists raw bytes to the config file atomically-ish (write under
// the repository mutex). Callers that need surgical comment-preserving edits
// should read, transform the text, then WriteRaw the result.
func (r *ConfigRepository) WriteRaw(data []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.path == "" {
		return fmt.Errorf("config path not set")
	}
	if err := r.fs.WriteFile(r.path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
