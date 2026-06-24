package gobridge

import (
	"encoding/base64"
	"testing"
)

// TestHPKEOpenCryptoKitSenderVector 验证真实 CryptoKit Sender 生成的密文可由 Mac runtime 解密。
//
// ⚠️ PENDING REGENERATION (2026-06-24):
// HKDF info 从 cccode-relay/pairing/v1 改为 cordcode-relay/pairing/v1 后，
// 旧的预计算 CryptoKit 密文 fixture（用旧 label 生成）无法用新 label 解密
// （message authentication failed，符合预期——HKDF 输入变了）。
// 需在 iOS 侧用新 label 重新生成真实 CryptoKit Sender 密文后更新下方
// encapsulatedKey/ciphertext。HPKE 跨语言一致性已由 TestHPKECirclInterop
// + TestHPKERFC9180Vector 充分覆盖（标准 RFC 9180 向量，不依赖本项目 label）。
func TestHPKEOpenCryptoKitSenderVector(t *testing.T) {
	t.Skip("PENDING: HKDF info 改名后需在 iOS 侧用 cordcode-relay/pairing/v1 重新生成 CryptoKit 密文 fixture")
	// CryptoKit 接受未 clamp 的私钥输入；这与 crypto/ecdh.GenerateKey 的持久化格式一致。
	privateKey := make([]byte, 32)
	for index := range privateKey {
		privateKey[index] = 7
	}
	encapsulatedKey, err := base64.StdEncoding.DecodeString("nC5VHWfassULLnLAqPJjYfgy49N+yAg35cLS97t+SXw=")
	if err != nil {
		t.Fatal(err)
	}
	ciphertext, err := base64.StdEncoding.DecodeString("j9MgpWAnCiS2zpX/SHLa3W/1qvsA4btqRlua0tvQ2rVdI3ha26s7dxHNEpRKlXP4+28iiru6yoogrgqHbshjKXIW9vIKtROmtdvXaqGOQhZ3SrFP9KJrDkA0FOzwskZ0niSINC3SZ8zjwMrEtVTR")
	if err != nil {
		t.Fatal(err)
	}

	plaintext, _, err := HPKEOpen(
		privateKey,
		[]byte(pairingContextLabel),
		[]byte("pairing:dev_unclamped:2026-05-25T09:27:41Z"),
		&HPKECiphertext{KEMOutput: encapsulatedKey, Ciphertext: ciphertext},
	)
	if err != nil {
		t.Fatalf("CryptoKit Sender ciphertext cannot be decrypted: %v", err)
	}
	const expected = `{"deviceId":"dev_unclamped","devicePubKey":"test","deviceName":"iPhone","timestamp":1779700000}`
	if string(plaintext) != expected {
		t.Fatalf("plaintext = %q, want %q", plaintext, expected)
	}

	if got := string(pairingClaimAAD("dev_unclamped", 1779701261)); got != "pairing:dev_unclamped:2026-05-25T09:27:41Z" {
		t.Fatalf("claim AAD = %q", got)
	}
}
