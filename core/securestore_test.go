package core

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// TestSecureWriteJSONAtomicAndPermissions 验证 P2-5：原子写入 + 0600 权限。
func TestSecureWriteJSONAtomicAndPermissions(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	type payload struct {
		V string `json:"v"`
	}
	if err := SecureWriteJSON(path, &payload{V: "hello"}, 0o600); err != nil {
		t.Fatalf("SecureWriteJSON: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o, want 0600", info.Mode().Perm())
	}
	var got payload
	if err := json.Unmarshal(mustRead(t, path), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.V != "hello" {
		t.Fatalf("v = %q", got.V)
	}
}

// TestSecureWriteJSONBacksUpCorruptFile 验证覆盖已损坏文件前先备份。
func TestSecureWriteJSONBacksUpCorruptFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "state.json")
	// 写入损坏内容。
	if err := os.WriteFile(path, []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SecureWriteJSON(path, map[string]string{"v": "fresh"}, 0o600); err != nil {
		t.Fatalf("SecureWriteJSON: %v", err)
	}
	// 应存在一个 .corrupt-* 备份。
	matches, err := filepath.Glob(filepath.Join(dir, "state.json.corrupt-*"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("期望存在损坏备份, matches=%v err=%v", matches, err)
	}
	// 新文件应为合法 JSON。
	var got map[string]string
	if err := json.Unmarshal(mustRead(t, path), &got); err != nil {
		t.Fatalf("新文件不是合法 JSON: %v", err)
	}
	if got["v"] != "fresh" {
		t.Fatalf("v = %q", got["v"])
	}
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
