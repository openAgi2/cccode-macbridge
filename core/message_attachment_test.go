package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestSaveFilesToDiskRejectsTraversal 验证 P2-3：FileName 的 ../ 与绝对路径不能逃逸出 attachment 目录。
func TestSaveFilesToDiskRejectsTraversal(t *testing.T) {
	workDir := t.TempDir()
	outsideTarget := filepath.Join(workDir, "escaped.txt")
	files := []FileAttachment{
		{FileName: "../../escaped.txt", Data: []byte("evil")},
		{FileName: "/etc/passwd", Data: []byte("evil-abs")},
		{FileName: "..\\..\\win-escape.txt", Data: []byte("win")},
		{FileName: "safe.txt", Data: []byte("ok")},
	}

	paths := SaveFilesToDisk(workDir, files)

	// escaped.txt 不应在 workDir 根下被创建。
	if _, err := os.Stat(outsideTarget); err == nil {
		t.Fatalf("traversal 文件被写到 workDir 根: %s", outsideTarget)
	}
	// 不应出现绝对路径写入。
	for _, p := range paths {
		rel, err := filepath.Rel(filepath.Join(workDir, ".cccode-macbridge", "attachments"), p)
		if err != nil || strings.HasPrefix(rel, "..") {
			t.Fatalf("文件逃逸出 attachment 目录: %s (rel=%q)", p, rel)
		}
	}
	// safe.txt 应被保留。
	found := false
	for _, p := range paths {
		if filepath.Base(p) == "safe.txt" {
			found = true
		}
	}
	if !found {
		t.Fatalf("safe.txt 应被正常保存, paths=%v", paths)
	}
}

// TestSafeAttachmentBaseName 覆盖各种恶意/边界文件名。
func TestSafeAttachmentBaseName(t *testing.T) {
	cases := []struct{ in, wantContains string; expectSynthetic bool }{
		{"", "", true},
		{"../../etc/passwd", "passwd", false},
		{"/abs/path/x.txt", "x.txt", false},
		{"normal.go", "normal.go", false},
		{"C:\\evil\\x.txt", "x.txt", false},
	}
	for i, c := range cases {
		got := safeAttachmentBaseName(c.in, i)
		if c.expectSynthetic {
			if !strings.HasPrefix(got, "file_") {
				t.Fatalf("case %d in=%q: 期望合成名, got %q", i, c.in, got)
			}
			continue
		}
		if c.wantContains != "" && !strings.Contains(got, c.wantContains) {
			t.Fatalf("case %d in=%q: got %q, want contains %q", i, c.in, got, c.wantContains)
		}
	}
}
