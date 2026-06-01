package claudecode

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestListMemoryFiles_OnlyReturnsExistingStableClaudeFiles(t *testing.T) {
	homeDir := t.TempDir()
	workDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir): %v", err)
	}
	if err := os.MkdirAll(filepath.Join(homeDir, ".claude"), 0o755); err != nil {
		t.Fatalf("MkdirAll(globalClaudeDir): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "CLAUDE.md"), []byte("# project\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(project CLAUDE.md): %v", err)
	}
	if err := os.WriteFile(filepath.Join(homeDir, ".claude", "CLAUDE.md"), []byte("# global\n"), 0o644); err != nil {
		t.Fatalf("WriteFile(global CLAUDE.md): %v", err)
	}
	if err := os.WriteFile(filepath.Join(workDir, "IGNORED.md"), []byte("ignore"), 0o644); err != nil {
		t.Fatalf("WriteFile(IGNORED.md): %v", err)
	}

	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	agent := &Agent{workDir: workDir}
	files, err := agent.ListMemoryFiles(context.Background())
	if err != nil {
		t.Fatalf("ListMemoryFiles() error = %v", err)
	}
	if len(files) != 2 {
		t.Fatalf("file count = %d, want 2", len(files))
	}
	if files[0].ID != projectClaudeMemoryFileID {
		t.Fatalf("files[0].ID = %q, want %q", files[0].ID, projectClaudeMemoryFileID)
	}
	if files[0].Scope != "project" {
		t.Fatalf("files[0].Scope = %q, want project", files[0].Scope)
	}
	if files[1].ID != globalClaudeMemoryFileID {
		t.Fatalf("files[1].ID = %q, want %q", files[1].ID, globalClaudeMemoryFileID)
	}
	if files[1].Scope != "global" {
		t.Fatalf("files[1].Scope = %q, want global", files[1].Scope)
	}
	if files[0].ETag == "" || files[1].ETag == "" {
		t.Fatal("expected non-empty etag for listed memory files")
	}
}

func TestReadMemoryFile_UsesStableIDsAndReturnsContent(t *testing.T) {
	homeDir := t.TempDir()
	workDir := filepath.Join(t.TempDir(), "project")
	if err := os.MkdirAll(workDir, 0o755); err != nil {
		t.Fatalf("MkdirAll(workDir): %v", err)
	}
	t.Setenv("HOME", homeDir)
	t.Setenv("USERPROFILE", homeDir)

	projectContent := "# Project Instructions\nAlways test changes.\n"
	if err := os.WriteFile(filepath.Join(workDir, "CLAUDE.md"), []byte(projectContent), 0o644); err != nil {
		t.Fatalf("WriteFile(project CLAUDE.md): %v", err)
	}

	agent := &Agent{workDir: workDir}
	file, err := agent.ReadMemoryFile(context.Background(), projectClaudeMemoryFileID)
	if err != nil {
		t.Fatalf("ReadMemoryFile() error = %v", err)
	}
	if file == nil {
		t.Fatal("ReadMemoryFile() = nil, want file")
	}
	if file.ID != projectClaudeMemoryFileID {
		t.Fatalf("file.ID = %q, want %q", file.ID, projectClaudeMemoryFileID)
	}
	if file.Name != "CLAUDE.md" {
		t.Fatalf("file.Name = %q, want CLAUDE.md", file.Name)
	}
	if file.Content != projectContent {
		t.Fatalf("file.Content = %q, want %q", file.Content, projectContent)
	}
	if file.ContentType != "text/markdown" {
		t.Fatalf("file.ContentType = %q, want text/markdown", file.ContentType)
	}
}
