package gobridge

import (
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/openAgi2/cordcode-macbridge/core"
)

// claudeEffortOverrideFile 持久化 iOS 端为 Claude Code backend 显式选择（或最近一次
// 实际使用）的 reasoning effort，使该选择在 go-bridge 重启后仍然生效。文件位于 Bridge
// data 目录（--data-dir；产品模式为 ~/Library/Application Support/CordCode Link/）。
//
// 背景：Claude Code 的 transcript（~/.claude/projects/*/<id>.jsonl）不记录 per-session
// effort，MacBridge 无法从历史 session 恢复各自当时的 effort。因此 Claude Code 模式下
// 「该 session 的 effort」实际等价于「Mac 端 Claude Code 当前会使用的全局 effort」，
// 其真值源是 ~/.claude/settings.json 的 effortLevel（用户在 Claude Code 里的偏好）。
// iOS 端显式改动 effort 时再以此 override 文件覆盖，优先级高于 settings.json。
const claudeEffortOverrideFile = "claude-effort.json"

// claudeSettingsSchema 仅解析 MacBridge 关心的 ~/.claude/settings.json 子集。
type claudeSettingsSchema struct {
	EffortLevel string            `json:"effortLevel"`
	Env         map[string]string `json:"env"`
}

// claudeSettingsPath 返回用户全局 Claude Code settings 路径。
func claudeSettingsPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "settings.json")
}

// readClaudeSettingsEffortFromPath 解析给定 settings.json，返回 Mac 端 Claude Code 的
// 全局 effort 偏好：优先 effortLevel 字段，回退 env.CLAUDE_CODE_EFFORT_LEVEL。
// 文件缺失或无法解析时返回 ""（原始未标准化值）。
func readClaudeSettingsEffortFromPath(path string) string {
	if path == "" {
		return ""
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var s claudeSettingsSchema
	if json.Unmarshal(b, &s) != nil {
		return ""
	}
	if e := strings.TrimSpace(s.EffortLevel); e != "" {
		return e
	}
	if s.Env != nil {
		return strings.TrimSpace(s.Env["CLAUDE_CODE_EFFORT_LEVEL"])
	}
	return ""
}

// readClaudeSettingsEffort 读取默认路径（~/.claude/settings.json）的 effort 偏好。
func readClaudeSettingsEffort() string {
	return readClaudeSettingsEffortFromPath(claudeSettingsPath())
}

// loadClaudeEffortOverride 读取 iOS 持久化的 effort 覆盖；dataDir 为空、文件不存在或
// 无法解析时返回 ""（原始未标准化值）。
func loadClaudeEffortOverride(dataDir string) string {
	if dataDir == "" {
		return ""
	}
	b, err := os.ReadFile(filepath.Join(dataDir, claudeEffortOverrideFile))
	if err != nil {
		return ""
	}
	var v struct {
		ReasoningEffort string `json:"reasoningEffort"`
	}
	if json.Unmarshal(b, &v) != nil {
		return ""
	}
	return strings.TrimSpace(v.ReasoningEffort)
}

// saveClaudeEffortOverride 原子写入 iOS 选择/最近使用的 effort（标准化后）；dataDir 为
// 空时为 no-op。失败仅记录日志，不影响发消息主流程。
func saveClaudeEffortOverride(dataDir, effort string) {
	if dataDir == "" {
		return
	}
	effort = normalizeClaudeRuntimeEffort(effort)
	b, err := json.Marshal(struct {
		ReasoningEffort string `json:"reasoningEffort"`
	}{ReasoningEffort: effort})
	if err != nil {
		slog.Warn("claudecode: failed to marshal effort override", "error", err)
		return
	}
	if err := core.AtomicWriteFile(filepath.Join(dataDir, claudeEffortOverrideFile), b, 0o600); err != nil {
		slog.Warn("claudecode: failed to persist effort override", "error", err)
	}
}

// resolveClaudeDefaultEffortFrom 返回启动时应注入 claude agent 的 effort，优先级：
//  1. iOS 持久化覆盖（claude-effort.json）
//  2. ~/.claude/settings.json 的 effortLevel 字段
//  3. settings.json env 块中的 CLAUDE_CODE_EFFORT_LEVEL
//
// 返回值已标准化（low/medium/high/xhigh/max/ultra）；无任何来源时返回 ""。
// settingsPath 参数用于测试注入；生产代码用 resolveClaudeDefaultEffort。
func resolveClaudeDefaultEffortFrom(dataDir, settingsPath string) string {
	if v := normalizeClaudeRuntimeEffort(loadClaudeEffortOverride(dataDir)); v != "" {
		return v
	}
	return normalizeClaudeRuntimeEffort(readClaudeSettingsEffortFromPath(settingsPath))
}

// resolveClaudeDefaultEffort 用默认 settings 路径解析启动 effort。
func resolveClaudeDefaultEffort(dataDir string) string {
	return resolveClaudeDefaultEffortFrom(dataDir, claudeSettingsPath())
}
