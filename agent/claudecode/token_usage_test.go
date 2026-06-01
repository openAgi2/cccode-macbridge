package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestGetTokenUsage_AggregatesPerSessionAndDeduplicatesAssistantMessageIDs(t *testing.T) {
	homeDir := t.TempDir()
	workDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir): %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	writeClaudeTranscriptFixture(t, homeDir, workDir, "ses-a", []string{
		`{"type":"assistant","timestamp":"2026-05-02T10:00:00Z","message":{"id":"assistant-1","role":"assistant","content":[{"type":"thinking","thinking":"先想一想"}],"usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":2,"cache_creation_input_tokens":1}}}`,
		`{"type":"assistant","timestamp":"2026-05-02T10:00:01Z","message":{"id":"assistant-1","role":"assistant","content":[{"type":"text","text":"done"}],"usage":{"input_tokens":10,"output_tokens":4,"cache_read_input_tokens":2,"cache_creation_input_tokens":1}}}`,
		`{"type":"assistant","timestamp":"2026-05-02T10:00:02Z","message":{"id":"assistant-2","role":"assistant","content":[{"type":"text","text":"next"}],"usage":{"input_tokens":3,"output_tokens":7}}}`,
	})
	writeClaudeTranscriptFixture(t, homeDir, workDir, "ses-b", []string{
		`{"type":"assistant","timestamp":"2026-05-02T11:00:00Z","message":{"id":"assistant-3","role":"assistant","content":[{"type":"text","text":"another"}],"usage":{"input_tokens":5,"output_tokens":6,"cache_read_tokens":1}}}`,
		`{"type":"assistant",`,
	})

	agent := &Agent{workDir: workDir}
	report, err := agent.GetTokenUsage(context.Background())
	if err != nil {
		t.Fatalf("GetTokenUsage() error = %v", err)
	}
	if report == nil {
		t.Fatal("GetTokenUsage() = nil, want report")
	}

	if report.TotalTokensUsed != 39 {
		t.Fatalf("TotalTokensUsed = %d, want 39", report.TotalTokensUsed)
	}
	if report.InputTokens != 18 {
		t.Fatalf("InputTokens = %d, want 18", report.InputTokens)
	}
	if report.OutputTokens != 17 {
		t.Fatalf("OutputTokens = %d, want 17", report.OutputTokens)
	}
	if report.CacheReadTokens != 3 {
		t.Fatalf("CacheReadTokens = %d, want 3", report.CacheReadTokens)
	}
	if report.CacheCreationTokens != 1 {
		t.Fatalf("CacheCreationTokens = %d, want 1", report.CacheCreationTokens)
	}
	if len(report.PerSessionBreakdown) != 2 {
		t.Fatalf("PerSessionBreakdown length = %d, want 2", len(report.PerSessionBreakdown))
	}

	breakdown := make(map[string]bool)
	for _, session := range report.PerSessionBreakdown {
		breakdown[session.SessionID] = true
		switch session.SessionID {
		case "ses-a":
			if session.TokensUsed != 27 {
				t.Fatalf("ses-a TokensUsed = %d, want 27", session.TokensUsed)
			}
		case "ses-b":
			if session.TokensUsed != 12 {
				t.Fatalf("ses-b TokensUsed = %d, want 12", session.TokensUsed)
			}
		default:
			t.Fatalf("unexpected session id in breakdown: %q", session.SessionID)
		}
	}
	if !breakdown["ses-a"] || !breakdown["ses-b"] {
		t.Fatalf("missing session breakdown entries: %#v", breakdown)
	}
}

func TestGetTokenUsage_ReturnsEmptyReportWhenProjectDirMissing(t *testing.T) {
	homeDir := t.TempDir()
	workDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir): %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	agent := &Agent{workDir: workDir}
	report, err := agent.GetTokenUsage(context.Background())
	if err != nil {
		t.Fatalf("GetTokenUsage() error = %v", err)
	}
	if report == nil {
		t.Fatal("GetTokenUsage() = nil, want empty report")
	}
	if report.TotalTokensUsed != 0 || report.InputTokens != 0 || report.OutputTokens != 0 {
		t.Fatalf("unexpected non-zero empty report: %+v", *report)
	}
}
