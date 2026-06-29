package gobridge

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestFile(t *testing.T, path, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func TestReadClaudeSettingsEffortFromPath(t *testing.T) {
	t.Run("effortLevel field", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"effortLevel":"xhigh","model":"opus"}`)
		if got := readClaudeSettingsEffortFromPath(p); got != "xhigh" {
			t.Fatalf("got %q, want xhigh", got)
		}
	})
	t.Run("falls back to env CLAUDE_CODE_EFFORT_LEVEL", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"env":{"CLAUDE_CODE_EFFORT_LEVEL":"max"}}`)
		if got := readClaudeSettingsEffortFromPath(p); got != "max" {
			t.Fatalf("got %q, want max", got)
		}
	})
	t.Run("effortLevel takes precedence over env", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"effortLevel":"xhigh","env":{"CLAUDE_CODE_EFFORT_LEVEL":"max"}}`)
		if got := readClaudeSettingsEffortFromPath(p); got != "xhigh" {
			t.Fatalf("got %q, want xhigh (field should win over env)", got)
		}
	})
	t.Run("both empty returns empty", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"model":"opus"}`)
		if got := readClaudeSettingsEffortFromPath(p); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("missing file returns empty", func(t *testing.T) {
		if got := readClaudeSettingsEffortFromPath(filepath.Join(t.TempDir(), "nope.json")); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("invalid JSON returns empty", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{not json`)
		if got := readClaudeSettingsEffortFromPath(p); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("empty path returns empty", func(t *testing.T) {
		if got := readClaudeSettingsEffortFromPath(""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestSaveLoadClaudeEffortOverride(t *testing.T) {
	t.Run("round-trip", func(t *testing.T) {
		dir := t.TempDir()
		saveClaudeEffortOverride(dir, "high")
		if got := loadClaudeEffortOverride(dir); got != "high" {
			t.Fatalf("got %q, want high", got)
		}
	})
	t.Run("normalizes on save", func(t *testing.T) {
		dir := t.TempDir()
		saveClaudeEffortOverride(dir, "  ULTRACODE  ")
		if got := loadClaudeEffortOverride(dir); got != "ultra" {
			t.Fatalf("got %q, want ultra (normalized)", got)
		}
	})
	t.Run("missing override returns empty", func(t *testing.T) {
		if got := loadClaudeEffortOverride(t.TempDir()); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("empty dataDir is no-op", func(t *testing.T) {
		saveClaudeEffortOverride("", "high") // must not panic / write
		if got := loadClaudeEffortOverride(""); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
}

func TestResolveClaudeDefaultEffortFrom(t *testing.T) {
	settingsWith := func(effortLevel string) string {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"effortLevel":"`+effortLevel+`"}`)
		return p
	}
	t.Run("override wins over settings.json", func(t *testing.T) {
		dir := t.TempDir()
		saveClaudeEffortOverride(dir, "ultra")
		settings := settingsWith("xhigh")
		if got := resolveClaudeDefaultEffortFrom(dir, settings); got != "ultra" {
			t.Fatalf("got %q, want ultra (override should win)", got)
		}
	})
	t.Run("falls back to settings.json when no override", func(t *testing.T) {
		settings := settingsWith("xhigh")
		if got := resolveClaudeDefaultEffortFrom(t.TempDir(), settings); got != "xhigh" {
			t.Fatalf("got %q, want xhigh", got)
		}
	})
	t.Run("falls back to env when no override and no effortLevel", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"env":{"CLAUDE_CODE_EFFORT_LEVEL":"max"}}`)
		if got := resolveClaudeDefaultEffortFrom(t.TempDir(), p); got != "max" {
			t.Fatalf("got %q, want max", got)
		}
	})
	t.Run("empty when neither override nor settings", func(t *testing.T) {
		p := filepath.Join(t.TempDir(), "settings.json")
		writeTestFile(t, p, `{"model":"opus"}`)
		if got := resolveClaudeDefaultEffortFrom(t.TempDir(), p); got != "" {
			t.Fatalf("got %q, want empty", got)
		}
	})
	t.Run("unparseable override value falls back to settings.json", func(t *testing.T) {
		dir := t.TempDir()
		// 直接写入一个 normalize 后为空的原始值，模拟脏 override
		writeTestFile(t, filepath.Join(dir, claudeEffortOverrideFile), `{"reasoningEffort":"garbage"}`)
		settings := settingsWith("high")
		if got := resolveClaudeDefaultEffortFrom(dir, settings); got != "high" {
			t.Fatalf("got %q, want high (dirty override should fall back to settings)", got)
		}
	})
}
