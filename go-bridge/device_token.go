package gobridge

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
)

const (
	// token 前缀，标识 token 版本
	tokenPrefix = "ccb1_"
	// 原始随机字节长度
	tokenByteLen = 32
)

// GenerateDeviceToken 生成 ccb1_ 前缀的 device token。
// 返回明文 token（用于一次性的安全传输）和 sha256 哈希（用于持久存储）。
func GenerateDeviceToken() (plain, hash string, err error) {
	raw := make([]byte, tokenByteLen)
	if _, err = rand.Read(raw); err != nil {
		return "", "", fmt.Errorf("生成随机字节失败: %w", err)
	}
	plain = tokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	hash = HashToken(plain)
	return plain, hash, nil
}

// HashToken 对明文 token 计算 sha256 哈希，返回 "sha256:<hex>" 格式。
func HashToken(plain string) string {
	h := sha256.Sum256([]byte(plain))
	return "sha256:" + hex.EncodeToString(h[:])
}

// ValidateTokenPrefix 校验 token 的 ccb1_ 前缀和 base64url payload 长度。
func ValidateTokenPrefix(token string) error {
	if !strings.HasPrefix(token, tokenPrefix) {
		return errors.New("token 缺少 ccb1_ 前缀")
	}
	payload := token[len(tokenPrefix):]
	decoded, err := base64.RawURLEncoding.DecodeString(payload)
	if err != nil {
		return fmt.Errorf("token payload 不是合法 base64url: %w", err)
	}
	if len(decoded) != tokenByteLen {
		return fmt.Errorf("token payload 长度应为 %d 字节，实际 %d", tokenByteLen, len(decoded))
	}
	return nil
}
