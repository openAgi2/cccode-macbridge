package config

import (
	"os"
	"strings"
	"testing"
)

// memFS is an in-memory configFS for testing ConfigRepository without disk.
type memFS struct {
	files map[string][]byte
}

func (m *memFS) ReadFile(name string) ([]byte, error) {
	data, ok := m.files[name]
	if !ok {
		return nil, os.ErrNotExist
	}
	return data, nil
}

func (m *memFS) WriteFile(name string, data []byte, perm os.FileMode) error {
	m.files[name] = append([]byte(nil), data...)
	return nil
}

// TestConfigRepository_ReadParsesConfig verifies the instance-scoped repository
// reads and parses config without touching the package-level ConfigPath global.
func TestConfigRepository_ReadParsesConfig(t *testing.T) {
	// Ensure the global is unset so the repository is the only source of path.
	ConfigPath = ""
	const path = "/test/config.toml"
	fs := &memFS{files: map[string][]byte{
		path: []byte(`data_dir = "/tmp/test-data"` + "\n" +
			`language = "zh"` + "\n"),
	}}
	r := newConfigRepositoryWithFS(path, fs)

	cfg, err := r.Read()
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if cfg.DataDir != "/tmp/test-data" {
		t.Errorf("DataDir = %q, want /tmp/test-data", cfg.DataDir)
	}
	if cfg.Language != "zh" {
		t.Errorf("Language = %q, want zh", cfg.Language)
	}
	// Global must remain unset — repository is independent.
	if ConfigPath != "" {
		t.Errorf("ConfigPath = %q after Read; repository must not mutate the global", ConfigPath)
	}
}

// TestConfigRepository_WriteRoundTrip verifies WriteRaw then Read returns the
// written bytes parsed back.
func TestConfigRepository_WriteRoundTrip(t *testing.T) {
	ConfigPath = ""
	const path = "/test/write.toml"
	fs := &memFS{files: map[string][]byte{}}
	r := newConfigRepositoryWithFS(path, fs)

	raw := []byte(`data_dir = "/tmp/x"` + "\n")
	if err := r.WriteRaw(raw); err != nil {
		t.Fatalf("WriteRaw: %v", err)
	}
	cfg, err := r.Read()
	if err != nil {
		t.Fatalf("Read after WriteRaw: %v", err)
	}
	if cfg.DataDir != "/tmp/x" {
		t.Errorf("DataDir = %q, want /tmp/x", cfg.DataDir)
	}
}

// TestConfigRepository_EmptyPathErrors verifies an unset path fails fast.
func TestConfigRepository_EmptyPathErrors(t *testing.T) {
	r := newConfigRepositoryWithFS("", &memFS{files: map[string][]byte{}})
	if _, err := r.Read(); err == nil {
		t.Error("Read with empty path returned nil error")
	}
	if err := r.WriteRaw([]byte("x")); err == nil {
		t.Error("WriteRaw with empty path returned nil error")
	}
}

// TestConfigGlobalsMarkedDeprecated verifies the Deprecated doc comment is
// present on the globals (T10 governance signal: new code must not add uses).
func TestConfigGlobalsMarkedDeprecated(t *testing.T) {
	data, err := os.ReadFile("config.go")
	if err != nil {
		t.Fatalf("read config.go: %v", err)
	}
	src := string(data)
	for _, global := range []string{"configMu", "ConfigPath"} {
		// Each global declaration block should carry a Deprecated: marker.
		idx := strings.Index(src, "var "+global)
		if idx < 0 {
			t.Errorf("global %q not found in config.go", global)
			continue
		}
		block := src[:idx]
		if !strings.Contains(block, "Deprecated:") {
			t.Errorf("global %q missing Deprecated: doc comment (T10)", global)
		}
	}
}
