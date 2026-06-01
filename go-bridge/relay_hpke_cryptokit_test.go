package gobridge

import (
	"encoding/base64"
	"testing"
)

// TestHPKEOpenCryptoKitSenderVector 验证真实 CryptoKit Sender 生成的密文可由 Mac runtime 解密。
func TestHPKEOpenCryptoKitSenderVector(t *testing.T) {
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
