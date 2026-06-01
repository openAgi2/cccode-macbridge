package gobridge

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

func TestGenerateDeviceToken_PrefixAndLength(t *testing.T) {
	plain, _, err := GenerateDeviceToken()
	if err != nil {
		t.Fatalf("GenerateDeviceToken 返回错误: %v", err)
	}
	if !strings.HasPrefix(plain, "ccb1_") {
		t.Errorf("token 缺少 ccb1_ 前缀, got: %s", plain)
	}
	// ccb1_ + 32 bytes base64url(no padding) = 5 + 43 = 48 chars
	payload := plain[5:]
	if len(payload) != 43 {
		t.Errorf("payload 长度应为 43 (32 bytes base64url), got %d", len(payload))
	}
}

func TestGenerateDeviceToken_HashFormat(t *testing.T) {
	plain, hash, err := GenerateDeviceToken()
	if err != nil {
		t.Fatalf("GenerateDeviceToken 返回错误: %v", err)
	}
	if !strings.HasPrefix(hash, "sha256:") {
		t.Errorf("hash 缺少 sha256: 前缀, got: %s", hash)
	}
	// 手动验证哈希值
	h := sha256.Sum256([]byte(plain))
	expected := "sha256:" + hex.EncodeToString(h[:])
	if hash != expected {
		t.Errorf("hash 不匹配\ngot:      %s\nexpected: %s", hash, expected)
	}
}

func TestGenerateDeviceToken_Unique(t *testing.T) {
	plain1, _, err := GenerateDeviceToken()
	if err != nil {
		t.Fatalf("第一次调用失败: %v", err)
	}
	plain2, _, err := GenerateDeviceToken()
	if err != nil {
		t.Fatalf("第二次调用失败: %v", err)
	}
	if plain1 == plain2 {
		t.Error("两次生成的 token 不应相同")
	}
}

func TestHashToken_Consistency(t *testing.T) {
	plain, hash, _ := GenerateDeviceToken()
	hash2 := HashToken(plain)
	if hash != hash2 {
		t.Errorf("HashToken 和 GenerateDeviceToken 的哈希不一致:\n%s\n%s", hash, hash2)
	}
}

func TestValidateTokenPrefix_Accept(t *testing.T) {
	plain, _, _ := GenerateDeviceToken()
	if err := ValidateTokenPrefix(plain); err != nil {
		t.Errorf("合法 token 校验失败: %v", err)
	}
}

func TestValidateTokenPrefix_WrongPrefix(t *testing.T) {
	err := ValidateTokenPrefix("wrong_ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopq")
	if err == nil {
		t.Error("错误前缀应被拒绝")
	}
}

func TestValidateTokenPrefix_Empty(t *testing.T) {
	err := ValidateTokenPrefix("")
	if err == nil {
		t.Error("空 token 应被拒绝")
	}
}

func TestValidateTokenPrefix_WrongPayloadLength(t *testing.T) {
	// 前缀正确但 payload 解码后不是 32 字节
	err := ValidateTokenPrefix("ccb1_AQ") // 1 byte
	if err == nil {
		t.Error("短 payload 应被拒绝")
	}
}

func TestValidateTokenPrefix_InvalidBase64(t *testing.T) {
	err := ValidateTokenPrefix("ccb1_!!!invalid!!!")
	if err == nil {
		t.Error("非法 base64url 应被拒绝")
	}
}

func TestTrustedDeviceRecord_JSONTags(t *testing.T) {
	rec := TrustedDeviceRecord{
		DeviceID:    "dev1",
		DisplayName: "Test Device",
		Platform:    "ios",
		TokenHash:   "sha256:abc123",
	}
	data, err := json.Marshal(rec)
	if err != nil {
		t.Fatalf("JSON 序列化失败: %v", err)
	}
	s := string(data)
	for _, field := range []string{"deviceId", "displayName", "platform", "tokenHash", "createdAt", "lastSeenAt"} {
		if !strings.Contains(s, `"`+field+`"`) {
			t.Errorf("JSON 输出缺少 camelCase 字段 %q: %s", field, s)
		}
	}
	// 确认不出现 snake_case
	for _, bad := range []string{"device_id", "display_name", "token_hash", "last_seen_at"} {
		if strings.Contains(s, bad) {
			t.Errorf("JSON 输出包含 snake_case 字段 %q: %s", bad, s)
		}
	}
}
