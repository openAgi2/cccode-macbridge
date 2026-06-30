package claudecode

import (
	"strings"
	"testing"
)

// runtimeEnvLocked must tag Claude Code transcripts with the IDE-visible
// entrypoint (claude-desktop-3p) so iOS/MacBridge-initiated sessions show up in
// VSCode / JetBrains / Claude desktop session lists. Without it, claude tags
// stream-json-spawned sessions "sdk-cli", which every Anthropic IDE surface
// filters out of its list — sessions are written correctly to disk yet never
// appear on the Mac. See claudecode.go:runtimeEnvLocked for the full rationale.
func TestRuntimeEnvTagsClaudeEntrypointForIDEVisibility(t *testing.T) {
	// activeIdx = -1 => no provider selected, so providerEnvLocked returns nil
	// without touching the provider proxy. Mirrors a freshly-init'd bridge.
	a := &Agent{activeIdx: -1}
	env := a.runtimeEnvLocked()

	got := ""
	count := 0
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok && k == "CLAUDE_CODE_ENTRYPOINT" {
			count++
			got = v
		}
	}
	if count == 0 {
		t.Fatalf("runtimeEnvLocked missing CLAUDE_CODE_ENTRYPOINT; env=%v", env)
	}
	if got != "claude-desktop-3p" {
		t.Fatalf("CLAUDE_CODE_ENTRYPOINT = %q, want %q", got, "claude-desktop-3p")
	}
}

// A provider/session-configured CLAUDE_CODE_ENTRYPOINT must be overridden (and
// not leave a duplicate KEY line). The IDE-visible value always wins; otherwise a
// stale "sdk-cli" override would re-hide sessions.
func TestRuntimeEnvClaudeEntrypointOverridesAndDedupes(t *testing.T) {
	a := &Agent{activeIdx: -1, sessionEnv: []string{"CLAUDE_CODE_ENTRYPOINT=sdk-cli"}}
	env := a.runtimeEnvLocked()

	count := 0
	last := ""
	for _, e := range env {
		if k, v, ok := strings.Cut(e, "="); ok && k == "CLAUDE_CODE_ENTRYPOINT" {
			count++
			last = v
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly 1 CLAUDE_CODE_ENTRYPOINT entry (deduped), got %d: %v", count, env)
	}
	if last != "claude-desktop-3p" {
		t.Fatalf("CLAUDE_CODE_ENTRYPOINT = %q, want claude-desktop-3p (override)", last)
	}
}
