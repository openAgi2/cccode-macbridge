package codex

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSessionListCacheStoresPostPatchMtime(t *testing.T) {
	codexHome, sessionPath := writeCodexSessionFixture(t)

	var cache sessionListCache
	sessions, err := cache.list(codexHome)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(sessions) != 1 {
		t.Fatalf("sessions length = %d, want 1", len(sessions))
	}

	stat, err := os.Stat(sessionPath)
	if err != nil {
		t.Fatalf("stat session: %v", err)
	}
	entry := cache.files[sessionPath]
	if entry == nil {
		t.Fatalf("cache entry missing for %s", sessionPath)
	}
	if !entry.mtime.Equal(stat.ModTime()) {
		t.Fatalf("cached mtime = %v, want post-patch mtime %v", entry.mtime, stat.ModTime())
	}

	data, err := os.ReadFile(sessionPath)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	firstLine := strings.SplitN(string(data), "\n", 2)[0]
	if !strings.Contains(firstLine, `"source":"cli"`) {
		t.Fatalf("session source was not patched: %s", firstLine)
	}
}

func TestSessionListCacheReturnsCopy(t *testing.T) {
	codexHome, _ := writeCodexSessionFixture(t)

	var cache sessionListCache
	first, err := cache.list(codexHome)
	if err != nil {
		t.Fatalf("first list failed: %v", err)
	}
	if len(first) != 1 {
		t.Fatalf("first length = %d, want 1", len(first))
	}

	first[0].Summary = "polluted"

	second, err := cache.list(codexHome)
	if err != nil {
		t.Fatalf("second list failed: %v", err)
	}
	if len(second) != 1 {
		t.Fatalf("second length = %d, want 1", len(second))
	}
	if second[0].Summary == "polluted" {
		t.Fatalf("cached session was mutated through returned slice")
	}
}

func writeCodexSessionFixture(t *testing.T) (string, string) {
	t.Helper()

	codexHome := filepath.Join(t.TempDir(), ".codex")
	sessionsDir := filepath.Join(codexHome, "sessions")
	if err := os.MkdirAll(sessionsDir, 0o755); err != nil {
		t.Fatalf("mkdir sessions: %v", err)
	}

	sessionID := "test-session-cache"
	sessionPath := filepath.Join(sessionsDir, "rollout-"+sessionID+".jsonl")
	content := strings.Join([]string{
		`{"timestamp":"2026-01-01T00:00:00Z","type":"session_meta","payload":{"id":"` + sessionID + `","source":"exec","originator":"codex_exec","cwd":"/tmp/project"}}`,
		`{"timestamp":"2026-01-01T00:00:01Z","type":"response_item","payload":{"role":"user","content":[{"type":"input_text","text":"real user prompt"}]}}`,
		``,
	}, "\n")
	if err := os.WriteFile(sessionPath, []byte(content), 0o644); err != nil {
		t.Fatalf("write session: %v", err)
	}

	return codexHome, sessionPath
}
