package core

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// MergeEnv returns base env with entries from extra overriding same-key entries.
// This prevents duplicate keys (e.g. two PATH entries) which cause the override
// to be silently ignored on Linux (getenv returns the first match).
func MergeEnv(base, extra []string) []string {
	keys := make(map[string]bool, len(extra))
	for _, e := range extra {
		if k, _, ok := strings.Cut(e, "="); ok {
			keys[k] = true
		}
	}
	merged := make([]string, 0, len(base)+len(extra))
	for _, e := range base {
		if k, _, ok := strings.Cut(e, "="); ok && keys[k] {
			continue
		}
		merged = append(merged, e)
	}
	return append(merged, extra...)
}

// CheckAllowFrom logs a security warning at startup when allow_from is not
// configured (defaults to permit-all). Platforms should call this during init.
func CheckAllowFrom(platform, allowFrom string) {
	if strings.TrimSpace(allowFrom) == "" {
		slog.Warn("allow_from is not set — all users are permitted. "+
			"Set allow_from in config to restrict access.",
			"platform", platform)
	}
}

// RedactToken replaces a secret token in text with [REDACTED] to prevent
// token leakage in logs or error messages.
func RedactToken(text, token string) string {
	if token == "" || text == "" {
		return text
	}
	return strings.ReplaceAll(text, token, "[REDACTED]")
}

// AllowList checks whether a user ID is permitted based on a comma-separated
// allow_from string. Returns true if allowFrom is empty or "*" (allow all),
// or if the userID is in the list. Comparison is case-insensitive.
func AllowList(allowFrom, userID string) bool {
	allowFrom = strings.TrimSpace(allowFrom)
	if allowFrom == "" || allowFrom == "*" {
		return true
	}
	for _, id := range strings.Split(allowFrom, ",") {
		if strings.EqualFold(strings.TrimSpace(id), userID) {
			return true
		}
	}
	return false
}

// ImageAttachment represents an image sent by the user.
type ImageAttachment struct {
	MimeType string // e.g. "image/png", "image/jpeg"
	Data     []byte // raw image bytes
	FileName string // original filename (optional)
}

// 附件文件名清理用的常量（P2-3），避免在字符串字面量中混用转义。
const (
	backslash      = "\\"
	forwardSlash   = "/"
	pathSeparators = "/\\:"
)

// FileAttachment represents a file (PDF, doc, spreadsheet, etc.) sent by the user.
type FileAttachment struct {
	MimeType string // e.g. "application/pdf", "text/plain"
	Data     []byte // raw file bytes
	FileName string // original filename
}

// SaveFilesToDisk saves file attachments to workDir/.cccode-macbridge/attachments/
// and returns the list of absolute file paths. Agents can reference these paths
// in their prompts so the CLI can read them with built-in tools.
func SaveFilesToDisk(workDir string, files []FileAttachment) []string {
	if len(files) == 0 {
		return nil
	}
	attachDir := filepath.Join(workDir, ".cccode-macbridge", "attachments")
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		slog.Warn("SaveFilesToDisk: mkdir failed", "dir", attachDir, "error", err)
	}

	var paths []string
	for i, f := range files {
		fname := safeAttachmentBaseName(f.FileName, i)
		fpath := filepath.Join(attachDir, fname)
		// P2-3: basename 化后再校验最终路径仍在 attachDir 内（防御 symlink/eval 场景）。
		if !isWithinDir(attachDir, fpath) {
			slog.Error("SaveFilesToDisk: rejected path escaping attachment dir", "name", f.FileName, "resolved", fpath)
			continue
		}
		if err := os.WriteFile(fpath, f.Data, 0o644); err != nil {
			slog.Error("SaveFilesToDisk: write failed", "error", err)
			continue
		}
		paths = append(paths, fpath)
		slog.Debug("SaveFilesToDisk: file saved", "path", fpath, "name", f.FileName, "mime", f.MimeType, "size", len(f.Data))
	}
	return paths
}

// safeAttachmentBaseName 将客户端提供的文件名收敛为安全的 basename（P2-3）。
// 拒绝绝对路径与 ../ 逃逸：只取 Base，并对 Windows 风格分隔符与盘符做兜底处理。
// 空名或纯分隔符名回退为时间戳+索引的合成名。
func safeAttachmentBaseName(name string, index int) string {
	// 规范化 Windows 分隔符，避免 filepath.Base 在 unix 上漏判 "C:\\evil"。
	cleaned := strings.ReplaceAll(name, backslash, forwardSlash)
	base := filepath.Base(cleaned)
	// filepath.Base("/") == "/"，filepath.Base("C:") == "C:" 等：回退。
	if base == "" || base == "." || base == "/" || base == string(filepath.Separator) {
		return fmt.Sprintf("file_%d_%d", time.Now().UnixMilli(), index)
	}
	// 再防御一层：若 base 仍含路径分隔符或盘符冒号，回退。
	if strings.ContainsAny(base, pathSeparators) {
		return fmt.Sprintf("file_%d_%d", time.Now().UnixMilli(), index)
	}
	return base
}

// isWithinDir 判断 target（已 Clean）是否位于 dir（已 Clean）之下。
func isWithinDir(dir, target string) bool {
	absDir, err := filepath.Abs(filepath.Clean(dir))
	if err != nil {
		return false
	}
	absTarget, err := filepath.Abs(filepath.Clean(target))
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(absDir, absTarget)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// AppendFileRefs appends file path references to a prompt string.
func AppendFileRefs(prompt string, filePaths []string) string {
	if len(filePaths) == 0 {
		return prompt
	}
	if prompt == "" {
		prompt = "Please analyze the attached file(s)."
	}
	return prompt + "\n\n(Files saved locally, please read them: " + strings.Join(filePaths, ", ") + ")"
}

// AudioAttachment represents a voice/audio message sent by the user.
type AudioAttachment struct {
	MimeType string // e.g. "audio/amr", "audio/ogg", "audio/mp4"
	Data     []byte // raw audio bytes
	Format   string // short format hint: "amr", "ogg", "m4a", "mp3", "wav", etc.
	Duration int    // duration in seconds (if known)
}

// LocationAttachment represents a geographical location sent by the user.
type LocationAttachment struct {
	Latitude             float64 // latitude coordinate
	Longitude            float64 // longitude coordinate
	HorizontalAccuracy   float64 // accuracy radius in meters (optional)
	LivePeriod           int     // time period for live location updates in seconds (optional)
	Heading              int     // direction of movement in degrees (optional)
	ProximityAlertRadius int     // maximum distance for proximity alerts in meters (optional)
}

// Message represents a unified incoming message from any platform.
type Message struct {
	SessionKey   string // unique key for user context, e.g. "feishu:{chatID}:{userID}"
	Platform     string
	MessageID    string // platform message ID for tracing
	UserID       string
	UserName     string
	ChatName     string // human-readable chat/group name (optional)
	Content      string
	Images       []ImageAttachment   // attached images (if any)
	Files        []FileAttachment    // attached files (if any)
	Audio        *AudioAttachment    // voice message (if any)
	Location     *LocationAttachment // geographical location (if any)
	ExtraContent string              // platform-enriched content (e.g. location text, reply quote) prepended for the agent
	ChannelKey   string              // platform-provided channel identifier for workspace binding (optional)
	ReplyCtx     any                 // platform-specific context needed for replying
	FromVoice    bool                // true if message originated from voice transcription
	ModeOverride string              // if set, temporarily override agent permission mode for this message
}

// EventType distinguishes different kinds of agent output.
type EventType string

const (
	EventText                EventType = "text"                  // intermediate or final text
	EventTextReplace         EventType = "text_replace"          // full text replacement (non-incremental update)
	EventToolUse             EventType = "tool_use"              // tool invocation info
	EventToolResult          EventType = "tool_result"           // tool execution result
	EventPlan                EventType = "plan"                  // todo/plan update
	EventResult              EventType = "result"                // final aggregated result
	EventError               EventType = "error"                 // error occurred
	EventPermissionRequest   EventType = "permission_request"    // agent requests permission via stdio protocol
	EventThinking            EventType = "thinking"              // thinking/processing status
	EventTurnStarted         EventType = "turn_started"          // new turn started (for passive broadcast)
	EventContextCompressing  EventType = "context_compressing"   // context compression started
	EventContextCompressed   EventType = "context_compressed"    // context compression completed
	EventContextUsageUpdated EventType = "context_usage_updated" // runtime context usage changed
	EventQuestionAsked       EventType = "question_asked"        // agent asks user a question (Codex)
	EventQuestionResolved    EventType = "question_resolved"     // question was answered or cancelled
)

// UserQuestion represents a structured question from AskUserQuestion.
type UserQuestion struct {
	Question    string               `json:"question"`
	Header      string               `json:"header"`
	Options     []UserQuestionOption `json:"options"`
	MultiSelect bool                 `json:"multiSelect"`
}

// UserQuestionOption is one choice in a UserQuestion.
type UserQuestionOption struct {
	Label       string `json:"label"`
	Description string `json:"description"`
}

// QuestionOption is one selectable option in a Codex question ask event.
type QuestionOption struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// FileChange describes one structured file mutation emitted by an agent.
type FileChange struct {
	Path     string
	Kind     string
	Diff     string
	MovePath string
}

// Event represents a single piece of agent output streamed back to the engine.
type Event struct {
	Type         EventType
	Content      string
	ToolName     string         // populated for EventToolUse, EventPermissionRequest
	ToolInput    string         // human-readable summary of tool input
	ToolInputRaw map[string]any // raw tool input (for EventPermissionRequest, used in allow response)
	ToolResult   string         // populated for EventToolResult
	ToolStatus   string         // optional status for EventToolResult (e.g. completed/failed)
	ToolExitCode *int           // optional exit code for EventToolResult
	ToolSuccess  *bool          // optional success flag for EventToolResult
	SessionID    string         // agent-managed session ID for conversation continuity
	RequestID    string         // unique request ID for EventPermissionRequest
	Questions    []UserQuestion // populated when ToolName == "AskUserQuestion"
	Plan         []Todo         `json:",omitempty"`
	Done         bool
	Error        error
	InputTokens  int // token usage from agent result events
	OutputTokens int
	ContextUsage *ContextUsage
	FileChanges  []FileChange
	// question 相关字段
	QuestionID   string           // question 唯一标识 (Codex ask)
	QuestionText string           // question prompt 文本
	QuestionOpts []QuestionOption // 可选项
	Required     bool             // 是否必须回答
	ThreadID     string           // Codex thread id
}

// HistoryEntry is one turn in a conversation.
type HistoryEntry struct {
	Role      string    `json:"role"` // "user" or "assistant"
	Content   string    `json:"content"`
	Timestamp time.Time `json:"timestamp"`
}

// RichHistoryEntry is a backward-compatible superset for backends that can
// return structured message history with parts, steps, and thinking blocks.
// Callers should continue to honor Role/Content/Timestamp as the minimal
// compatibility surface and treat the richer fields as optional enhancements.
type RichHistoryEntry struct {
	ID         string           `json:"id,omitempty"`
	Role       string           `json:"role"`
	Content    string           `json:"content"`
	Thinking   string           `json:"thinking,omitempty"`
	Parts      []map[string]any `json:"parts,omitempty"`
	Steps      []map[string]any `json:"steps,omitempty"`
	Files      []map[string]any `json:"files,omitempty"`
	Timestamp  time.Time        `json:"timestamp"`
	AgentName  string           `json:"agentName,omitempty"`
	ModelID    string           `json:"modelId,omitempty"`
	ProviderID string           `json:"providerId,omitempty"`
	ModelName  string           `json:"modelName,omitempty"`
}

// Todo represents one backend-managed todo item for a session.
type Todo struct {
	Content  string `json:"content"`
	Status   string `json:"status"`
	Priority string `json:"priority,omitempty"`
}

// AgentDescriptor describes an available agent profile exposed by a backend.
type AgentDescriptor struct {
	Name        string `json:"name"`
	Mode        string `json:"mode,omitempty"`
	Hidden      bool   `json:"hidden,omitempty"`
	Native      bool   `json:"native,omitempty"`
	Description string `json:"description,omitempty"`
}

// AgentSessionInfo describes one session as reported by the agent backend.
type AgentSessionInfo struct {
	ID           string
	Summary      string
	MessageCount int
	ModifiedAt   time.Time `json:"modified_at"`
	ArchivedAt   time.Time `json:"archived_at,omitempty"`
	GitBranch    string
	Directory    string
}
