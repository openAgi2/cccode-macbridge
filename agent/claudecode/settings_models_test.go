package claudecode

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/openAgi2/cordcode-macbridge/core"
)

func writeClaudeSettings(t *testing.T, dir, body string) {
	t.Helper()
	cfgDir := filepath.Join(dir, ".claude")
	if err := os.MkdirAll(cfgDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cfgDir, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// settings.json 的别名映射应被解析成 (Name=claude 别名, Desc=GLM 显示名) 列表。
func TestSettingsModels_BuildsAliasPairsFromEnv(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "") // 走 HOME 路径
	t.Setenv("HOME", dir)
	writeClaudeSettings(t, dir, `{
		"model": "opus",
		"env": {
			"ANTHROPIC_API_KEY": "sk-secret-not-asserted",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-4-5",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME": "glm-4.7",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-6",
			"ANTHROPIC_DEFAULT_SONNET_MODEL_NAME": "glm-5-turbo",
			"ANTHROPIC_DEFAULT_OPUS_MODEL": "claude-opus-4-8[1M]",
			"ANTHROPIC_DEFAULT_OPUS_MODEL_NAME": "glm-5.2"
		}
	}`)

	got := (&Agent{}).settingsModels()
	want := []core.ModelOption{
		{Name: "haiku", Desc: "glm-4.7"},
		{Name: "sonnet", Desc: "glm-5-turbo"},
		{Name: "opus", Desc: "glm-5.2"},
	}
	if len(got) != len(want) {
		t.Fatalf("settingsModels = %+v, want %d 项", got, len(want))
	}
	for i, m := range got {
		if m.Name != want[i].Name || m.Desc != want[i].Desc {
			t.Errorf("[%d] = {Name:%q Desc:%q}, want {Name:%q Desc:%q}", i, m.Name, m.Desc, want[i].Name, want[i].Desc)
		}
	}
}

// 缺少 *_MODEL_NAME 的项应被跳过（不伪造显示名）。
func TestSettingsModels_SkipsPairsMissingName(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", dir)
	writeClaudeSettings(t, dir, `{
		"env": {
			"ANTHROPIC_DEFAULT_HAIKU_MODEL": "claude-haiku-4-5",
			"ANTHROPIC_DEFAULT_HAIKU_MODEL_NAME": "glm-4.7",
			"ANTHROPIC_DEFAULT_SONNET_MODEL": "claude-sonnet-4-6"
		}
	}`)

	got := (&Agent{}).settingsModels()
	if len(got) != 1 || got[0].Name != "haiku" || got[0].Desc != "glm-4.7" {
		t.Fatalf("settingsModels = %+v, want 仅 [haiku/glm-4.7]", got)
	}
}

// 无别名映射时应返回 nil，让 AvailableModels 走 fallback。
func TestSettingsModels_EmptyReturnsNil(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", dir)
	writeClaudeSettings(t, dir, `{"model":"opus","env":{"ANTHROPIC_API_KEY":"x"}}`)

	if got := (&Agent{}).settingsModels(); got != nil {
		t.Fatalf("settingsModels = %+v, want nil（无别名映射）", got)
	}
}

// settings.json 不存在时返回 nil（不 panic）。
func TestSettingsModels_MissingFileReturnsNil(t *testing.T) {
	t.Setenv("CLAUDE_CONFIG_DIR", "")
	t.Setenv("HOME", t.TempDir())
	if got := (&Agent{}).settingsModels(); got != nil {
		t.Fatalf("settingsModels = %+v, want nil（文件不存在）", got)
	}
}

// CLAUDE_CONFIG_DIR 优先于 HOME。
func TestSettingsModels_RespectsClaudeConfigDir(t *testing.T) {
	cfgDir := t.TempDir()
	t.Setenv("CLAUDE_CONFIG_DIR", cfgDir)
	t.Setenv("HOME", t.TempDir()) // 故意指向另一个空目录
	if err := os.WriteFile(filepath.Join(cfgDir, "settings.json"), []byte(`{
		"env": {
			"ANTHROPIC_DEFAULT_OPUS_MODEL": "claude-opus-4-8[1M]",
			"ANTHROPIC_DEFAULT_OPUS_MODEL_NAME": "glm-5.2"
		}
	}`), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got := (&Agent{}).settingsModels()
	if len(got) != 1 || got[0].Name != "opus" || got[0].Desc != "glm-5.2" {
		t.Fatalf("settingsModels = %+v, want [opus/glm-5.2] from CLAUDE_CONFIG_DIR", got)
	}
}
