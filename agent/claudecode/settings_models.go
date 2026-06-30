package claudecode

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/openAgi2/cordcode-macbridge/core"
)

// settingsModelsPath 返回 Claude Code settings.json 路径：优先 CLAUDE_CONFIG_DIR，
// 否则 ~/.claude/settings.json。空串表示无法解析（如无 HOME）。
func claudeSettingsPath() string {
	if dir := os.Getenv("CLAUDE_CONFIG_DIR"); dir != "" {
		return filepath.Join(dir, "settings.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// claudeSettingsFile 只解析我们关心的字段，避免把密钥反序列化进内存结构。
// ANTHROPIC_API_KEY / ANTHROPIC_AUTH_TOKEN 等敏感字段不在此声明，因此不会被读到程序变量里。
type claudeSettingsFile struct {
	Model string            `json:"model"` // 顶层默认模型（别名，如 "opus"）
	Env   map[string]string `json:"env"`
}

// modelAliasPairs 把 settings.json 的 ANTHROPIC_DEFAULT_<X>_MODEL 映射到 claude CLI 别名。
// 顺序即 iOS 选择器展示顺序（haiku→sonnet→opus，由快到强）。
var claudeModelAliasPairs = []struct{ suffix, alias string }{
	{"HAIKU", "haiku"},
	{"SONNET", "sonnet"},
	{"OPUS", "opus"},
}

// settingsModels 读 ~/.claude/settings.json，把 ANTHROPIC_DEFAULT_{HAIKU,SONNET,OPUS}_MODEL +
// *_MODEL_NAME 配对成模型列表：
//   - Name（送给 `claude --model` 的 id）= claude 别名（haiku/sonnet/opus）
//   - Desc（iOS 显示名）= *_MODEL_NAME（如 glm-4.7）
//
// claude 收到别名后按 settings.json 的 ANTHROPIC_DEFAULT_*_MODEL 解析成真实 id 再送网关，
// 与用户顶层 "model": "opus" 同机制。详见 docs/2026-06-30-claudecode-models-from-settings-json.md。
//
// 缓存 + mtime 懒重载：每次调用检查 settings.json 的 mtime，变了（或首次）才重读。
// 不引入 fsnotify 依赖、不启后台 goroutine。settings.json 缺失/无映射返回 nil（由调用方 fallback）。
func (a *Agent) settingsModels() []core.ModelOption {
	path := claudeSettingsPath()
	if path == "" {
		return nil
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil
	}

	a.mu.RLock()
	cachedMtime := a.settingsModelsMtime
	cached := a.settingsModelsCache
	a.mu.RUnlock()
	if cached != nil && info.ModTime().Equal(cachedMtime) {
		return cached
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cached // 读失败时保留上次的缓存（若有），避免短暂抖动
	}
	var sf claudeSettingsFile
	if err := json.Unmarshal(data, &sf); err != nil || sf.Env == nil {
		return cached
	}

	var models []core.ModelOption
	for _, p := range claudeModelAliasPairs {
		modelKey := "ANTHROPIC_DEFAULT_" + p.suffix + "_MODEL"
		nameKey := "ANTHROPIC_DEFAULT_" + p.suffix + "_MODEL_NAME"
		modelVal := sf.Env[modelKey]
		nameVal := sf.Env[nameKey]
		if modelVal == "" || nameVal == "" {
			continue
		}
		models = append(models, core.ModelOption{
			Name: p.alias,
			Desc: nameVal,
		})
	}
	if len(models) == 0 {
		models = nil
	}

	a.mu.Lock()
	a.settingsModelsCache = models
	a.settingsModelsMtime = info.ModTime()
	a.mu.Unlock()
	return models
}

// forceSettingsModelsCacheForTest 仅供单测注入缓存，绕过文件读取。
func (a *Agent) forceSettingsModelsCacheForTest(models []core.ModelOption) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.settingsModelsCache = models
}
