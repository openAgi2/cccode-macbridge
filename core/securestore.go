package core

import (
	"encoding/json"
	"time"
	"fmt"
	"os"
	"path/filepath"
)

// SecureWriteJSON 将 value 以 JSON 原子写入 path（P2-5）。
//
// 统一安全状态持久化契约：
//   - 原子写（临时文件 + fsync + rename + 目录 fsync，见 AtomicWriteFile），避免崩溃截断。
//   - 文件权限 perm（敏感状态用 0600，仅 owner 可读）。
//   - 若目标已存在但读取/解析失败（损坏），在覆盖前备份为 <path>.corrupt-<timestamp>，
//     便于事后取证与恢复，而不是静默吞掉。
//
// 调用方仍负责 schema version 与字段级明文策略；本函数只保证"持久化原子性 + 权限 + 损坏取证"。
func SecureWriteJSON(path string, value any, perm os.FileMode) error {
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("securestore: marshal %s: %w", filepath.Base(path), err)
	}
	data = append(data, '\n')

	if existing, rerr := os.ReadFile(path); rerr == nil && len(existing) > 0 {
		var probe any
		if jerr := json.Unmarshal(existing, &probe); jerr != nil {
			// 现有文件损坏：备份后再原子覆盖，避免损坏被无声抹除。
			backup := fmt.Sprintf("%s.corrupt-%d", path, time.Now().UnixMilli())
			_ = os.WriteFile(backup, existing, perm)
		}
	}
	return AtomicWriteFile(path, data, perm)
}

// SecureWriteBytes 以原子方式写入原始字节（与 SecureWriteJSON 同语义，但不做 JSON 序列化）。
func SecureWriteBytes(path string, data []byte, perm os.FileMode) error {
	return AtomicWriteFile(path, data, perm)
}
