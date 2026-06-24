package gobridge

import (
	"crypto/ecdh"
	"crypto/rand"
	"encoding/hex"
	"testing"

	"github.com/cloudflare/circl/hpke"
)

// TestHPKECirclInterop 验证 Go 端 HPKE (circl) 与 iOS CryptoKit.HPKE 的互操作性。
// circl 是经过验证的 RFC 9180 实现，通过此测试间接证明 iOS 兼容性。
func TestHPKECirclInterop(t *testing.T) {
	recipientPriv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	pkR := recipientPriv.PublicKey().Bytes()
	skR := recipientPriv.Bytes()

	scheme := hpke.KEM_X25519_HKDF_SHA256.Scheme()
	skRCircl, err := scheme.UnmarshalBinaryPrivateKey(skR)
	if err != nil {
		t.Fatal(err)
	}
	pkRCircl := skRCircl.Public()

	suite := hpke.NewSuite(hpke.KEM_X25519_HKDF_SHA256, hpke.KDF_HKDF_SHA256, hpke.AEAD_ChaCha20Poly1305)

	info := []byte("cordcode-relay/pairing/v1")
	aad := []byte("pairing:test-device:2026-05-25")
	plaintext := []byte(`{"deviceId":"dev_interop","devicePubKey":"dGVzdA==","timestamp":1748000000}`)

	// circl Seal → our Open
	sender, _ := suite.NewSender(pkRCircl, info)
	enc, sealer, _ := sender.Setup(nil)
	ct, _ := sealer.Seal(plaintext, aad)

	ourCt := &HPKECiphertext{KEMOutput: enc, Ciphertext: ct}
	ourPt, _, err := HPKEOpen(skR, info, aad, ourCt)
	if err != nil {
		t.Fatalf("circl→our: %v", err)
	}
	if string(ourPt) != string(plaintext) {
		t.Errorf("circl→our decrypted mismatch")
	}

	// our Seal → circl Open
	ourCt2, _, err := HPKESeal(pkR, info, aad, plaintext)
	if err != nil {
		t.Fatal(err)
	}

	recv, _ := suite.NewReceiver(skRCircl, info)
	opener, _ := recv.Setup(ourCt2.KEMOutput)
	circlPt, err := opener.Open(ourCt2.Ciphertext, aad)
	if err != nil {
		t.Fatalf("our→circl: %v", err)
	}
	if string(circlPt) != string(plaintext) {
		t.Errorf("our→circl decrypted mismatch")
	}
}

// TestHPKERFC9180VectorCirclCrossCheck 用 circl 验证 test vector 可解密。
func TestHPKERFC9180VectorCirclCrossCheck(t *testing.T) {
	skRHex := "8057991eef8f1f1af18f4a9491d16a1ce333f695d4db8e38da75975c4478e0fb"
	pkEHex := "1afa08d3dec047a643885163f1180476fa7ddb54c6a8029ea33f95796bf2ac4a"

	skRBytes, _ := hex.DecodeString(skRHex)
	pkEBytes, _ := hex.DecodeString(pkEHex)

	scheme := hpke.KEM_X25519_HKDF_SHA256.Scheme()
	skR, _ := scheme.UnmarshalBinaryPrivateKey(skRBytes)

	suite := hpke.NewSuite(hpke.KEM_X25519_HKDF_SHA256, hpke.KDF_HKDF_SHA256, hpke.AEAD_ChaCha20Poly1305)

	info, _ := hex.DecodeString("4f6465206f6e2061204772656369616e2055726e")
	aad, _ := hex.DecodeString("436f756e742d30")
	ct, _ := hex.DecodeString("1c5250d8034ec2b784ba2cfd69dbdb8af406cfe3ff938e131f0def8c8b60b4db21993c62ce81883d2dd1b51a28")

	// circl 解密
	recv, _ := suite.NewReceiver(skR, info)
	opener, _ := recv.Setup(pkEBytes)
	pt, err := opener.Open(ct, aad)
	if err != nil {
		t.Fatalf("circl: %v", err)
	}
	if string(pt) != "Beauty is truth, truth beauty" {
		t.Errorf("circl unexpected plaintext: %s", pt)
	}

	// 我们的解密
	ourCt := &HPKECiphertext{KEMOutput: pkEBytes, Ciphertext: ct}
	ourPt, _, err := HPKEOpen(skRBytes, info, aad, ourCt)
	if err != nil {
		t.Fatalf("our: %v", err)
	}
	if string(ourPt) != "Beauty is truth, truth beauty" {
		t.Errorf("our unexpected plaintext: %s", ourPt)
	}
}
