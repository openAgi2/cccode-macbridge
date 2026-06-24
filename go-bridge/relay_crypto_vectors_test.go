package gobridge

import (
	"crypto/ecdh"
	"crypto/hkdf"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/cloudflare/circl/hpke"
	"golang.org/x/crypto/chacha20poly1305"
)

type relayCryptoVectors struct {
	SchemaRevision string `json:"schemaRevision"`
	PaddingByteHex string `json:"paddingByteHex"`
	Identity       struct {
		MacPrivateKey    string `json:"macPrivateKey"`
		MacPublicKey     string `json:"macPublicKey"`
		IOSPrivateKey    string `json:"iosPrivateKey"`
		IOSPublicKey     string `json:"iosPublicKey"`
		ContextCanonical string `json:"contextCanonical"`
		IdentityAuthKey  string `json:"identityAuthKey"`
	} `json:"identity"`
	Online struct {
		IOSEphemeralPrivateKey string `json:"iosEphemeralPrivateKey"`
		IOSEphemeralPublicKey  string `json:"iosEphemeralPublicKey"`
		MacEphemeralPrivateKey string `json:"macEphemeralPrivateKey"`
		MacEphemeralPublicKey  string `json:"macEphemeralPublicKey"`
		ClientHelloCanonical   string `json:"clientHelloCanonical"`
		ClientAuthTag          string `json:"clientAuthTag"`
		ServerHelloCanonical   string `json:"serverHelloCanonical"`
		ServerAuthTag          string `json:"serverAuthTag"`
		TranscriptHash         string `json:"transcriptHash"`
		IOSToMacKey            string `json:"iosToMacKey"`
		MacToIOSKey            string `json:"macToIosKey"`
		InnerPayload           string `json:"innerPayload"`
		AADCanonical           string `json:"aadCanonical"`
		Nonce                  string `json:"nonce"`
		Ciphertext             string `json:"ciphertext"`
	} `json:"online"`
	Mailbox struct {
		IOSPrekeyPrivateKey  string `json:"iosPrekeyPrivateKey"`
		IOSPrekeyPublicKey   string `json:"iosPrekeyPublicKey"`
		MacEpochPrivateKey   string `json:"macEpochPrivateKey"`
		MacEpochPublicKey    string `json:"macEpochPublicKey"`
		ContextCanonical     string `json:"contextCanonical"`
		MacToIOSKey          string `json:"macToIosKey"`
		EpochHeaderCanonical string `json:"epochHeaderCanonical"`
		EpochAuthTag         string `json:"epochAuthTag"`
		EpochDigest          string `json:"epochDigest"`
		InnerPayload         string `json:"innerPayload"`
		AADCanonical         string `json:"aadCanonical"`
		Nonce                string `json:"nonce"`
		Ciphertext           string `json:"ciphertext"`
	} `json:"mailbox"`
	HPKEBaseMode struct {
		Source                 string `json:"source"`
		KEMID                  uint16 `json:"kemId"`
		KDFID                  uint16 `json:"kdfId"`
		AEADID                 uint16 `json:"aeadId"`
		InfoHex                string `json:"infoHex"`
		RecipientPrivateKeyHex string `json:"recipientPrivateKeyHex"`
		RecipientPublicKeyHex  string `json:"recipientPublicKeyHex"`
		EncapsulatedKeyHex     string `json:"encapsulatedKeyHex"`
		AADHex                 string `json:"aadHex"`
		PlaintextHex           string `json:"plaintextHex"`
		CiphertextHex          string `json:"ciphertextHex"`
	} `json:"hpkeBaseMode"`
}

func loadRelayCryptoVectors(t *testing.T) relayCryptoVectors {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", "relay-v1", "crypto_vectors.json"))
	if err != nil {
		t.Fatalf("read relay crypto vectors: %v", err)
	}
	var vectors relayCryptoVectors
	if err := json.Unmarshal(raw, &vectors); err != nil {
		t.Fatalf("decode relay crypto vectors: %v", err)
	}
	return vectors
}

func decodeVectorBytes(t *testing.T, value string) []byte {
	t.Helper()
	result, err := base64.StdEncoding.DecodeString(value)
	if err != nil {
		t.Fatalf("decode base64 vector: %v", err)
	}
	return result
}

func decodeVectorHex(t *testing.T, value string) []byte {
	t.Helper()
	result, err := hex.DecodeString(value)
	if err != nil {
		t.Fatalf("decode hex vector: %v", err)
	}
	return result
}

func relayHKDF(t *testing.T, secret, salt []byte, info string) []byte {
	t.Helper()
	result, err := hkdf.Key(sha256.New, secret, salt, info, 32)
	if err != nil {
		t.Fatalf("derive HKDF vector: %v", err)
	}
	return result
}

func relayVectorKeyAgreement(t *testing.T, privateRaw, expectedPublic string) *ecdh.PrivateKey {
	t.Helper()
	key, err := ecdh.X25519().NewPrivateKey(decodeVectorBytes(t, privateRaw))
	if err != nil {
		t.Fatalf("create X25519 private key: %v", err)
	}
	if got := base64.StdEncoding.EncodeToString(key.PublicKey().Bytes()); got != expectedPublic {
		t.Fatalf("X25519 public key mismatch: got %s want %s", got, expectedPublic)
	}
	return key
}

func relayVectorPaddedPayload(t *testing.T, raw string) []byte {
	t.Helper()
	padded := make([]byte, 4, 256)
	binary.BigEndian.PutUint32(padded, uint32(len(raw)))
	padded = append(padded, []byte(raw)...)
	for len(padded) < 256 {
		padded = append(padded, 0xa5)
	}
	if len(padded) != 256 {
		t.Fatalf("padded plaintext length = %d, want 256", len(padded))
	}
	payloadLength := binary.BigEndian.Uint32(padded[:4])
	if got := string(padded[4 : 4+payloadLength]); got != raw {
		t.Fatalf("padded payload differs: got %q want %q", got, raw)
	}
	for _, b := range padded[4+payloadLength:] {
		if b != 0xa5 {
			t.Fatal("fixture padding is not deterministic test byte 0xa5")
		}
	}
	return padded
}

func relayVectorSealAndOpen(t *testing.T, key []byte, nonce string, plaintext []byte, aad string, ciphertext string) {
	t.Helper()
	aead, err := chacha20poly1305.New(key)
	if err != nil {
		t.Fatalf("create ChaCha20-Poly1305: %v", err)
	}
	expectedCiphertext := decodeVectorBytes(t, ciphertext)
	gotCiphertext := aead.Seal(nil, decodeVectorBytes(t, nonce), plaintext, []byte(aad))
	if !hmac.Equal(gotCiphertext, expectedCiphertext) {
		t.Fatal("ChaCha20-Poly1305 sealed output differs from shared vector")
	}
	opened, err := aead.Open(nil, decodeVectorBytes(t, nonce), expectedCiphertext, []byte(aad))
	if err != nil {
		t.Fatalf("open shared vector: %v", err)
	}
	if !hmac.Equal(opened, plaintext) {
		t.Fatal("opened plaintext differs from shared vector")
	}

	tampered := append([]byte(nil), expectedCiphertext...)
	tampered[0] ^= 0x01
	if _, err := aead.Open(nil, decodeVectorBytes(t, nonce), tampered, []byte(aad)); err == nil {
		t.Fatal("tampered ciphertext unexpectedly opened")
	}
	if _, err := aead.Open(nil, decodeVectorBytes(t, nonce), expectedCiphertext, []byte(aad+"x")); err == nil {
		t.Fatal("tampered AAD unexpectedly opened")
	}
}

func TestRelayCryptoVectorOnlineECDHEAndEnvelope(t *testing.T) {
	vectors := loadRelayCryptoVectors(t)
	macIdentity := relayVectorKeyAgreement(t, vectors.Identity.MacPrivateKey, vectors.Identity.MacPublicKey)
	iosIdentity := relayVectorKeyAgreement(t, vectors.Identity.IOSPrivateKey, vectors.Identity.IOSPublicKey)
	identitySecret, err := macIdentity.ECDH(iosIdentity.PublicKey())
	if err != nil {
		t.Fatalf("derive identity secret: %v", err)
	}
	identityAuthKey := relayHKDF(t, identitySecret, nil, "cordcode-relay/identity-auth/v1"+vectors.Identity.ContextCanonical)
	if got := base64.StdEncoding.EncodeToString(identityAuthKey); got != vectors.Identity.IdentityAuthKey {
		t.Fatalf("identity auth key differs: got %s want %s", got, vectors.Identity.IdentityAuthKey)
	}

	clientTag := hmac.New(sha256.New, identityAuthKey)
	clientTag.Write([]byte(vectors.Online.ClientHelloCanonical))
	if got := base64.StdEncoding.EncodeToString(clientTag.Sum(nil)); got != vectors.Online.ClientAuthTag {
		t.Fatalf("client handshake tag differs: got %s want %s", got, vectors.Online.ClientAuthTag)
	}
	serverTag := hmac.New(sha256.New, identityAuthKey)
	serverTag.Write([]byte(vectors.Online.ServerHelloCanonical))
	if got := base64.StdEncoding.EncodeToString(serverTag.Sum(nil)); got != vectors.Online.ServerAuthTag {
		t.Fatalf("server handshake tag differs: got %s want %s", got, vectors.Online.ServerAuthTag)
	}

	transcript := sha256.Sum256([]byte(vectors.Online.ClientHelloCanonical + vectors.Online.ServerHelloCanonical))
	if got := base64.StdEncoding.EncodeToString(transcript[:]); got != vectors.Online.TranscriptHash {
		t.Fatalf("transcript hash differs: got %s want %s", got, vectors.Online.TranscriptHash)
	}
	iosEphemeral := relayVectorKeyAgreement(t, vectors.Online.IOSEphemeralPrivateKey, vectors.Online.IOSEphemeralPublicKey)
	macEphemeral := relayVectorKeyAgreement(t, vectors.Online.MacEphemeralPrivateKey, vectors.Online.MacEphemeralPublicKey)
	ephemeralSecret, err := iosEphemeral.ECDH(macEphemeral.PublicKey())
	if err != nil {
		t.Fatalf("derive online ephemeral secret: %v", err)
	}
	trafficRoot := relayHKDF(t, ephemeralSecret, transcript[:], "cordcode-relay/online/v1")
	iosToMac := relayHKDF(t, trafficRoot, nil, "ios-to-mac")
	macToIOS := relayHKDF(t, trafficRoot, nil, "mac-to-ios")
	if got := base64.StdEncoding.EncodeToString(iosToMac); got != vectors.Online.IOSToMacKey {
		t.Fatalf("ios-to-mac key differs: got %s want %s", got, vectors.Online.IOSToMacKey)
	}
	if got := base64.StdEncoding.EncodeToString(macToIOS); got != vectors.Online.MacToIOSKey {
		t.Fatalf("mac-to-ios key differs: got %s want %s", got, vectors.Online.MacToIOSKey)
	}

	plaintext := relayVectorPaddedPayload(t, vectors.Online.InnerPayload)
	relayVectorSealAndOpen(t, macToIOS, vectors.Online.Nonce, plaintext, vectors.Online.AADCanonical, vectors.Online.Ciphertext)
}

func TestRelayCryptoVectorMailboxEpochChainAndEnvelope(t *testing.T) {
	vectors := loadRelayCryptoVectors(t)
	macIdentity := relayVectorKeyAgreement(t, vectors.Identity.MacPrivateKey, vectors.Identity.MacPublicKey)
	iosIdentity := relayVectorKeyAgreement(t, vectors.Identity.IOSPrivateKey, vectors.Identity.IOSPublicKey)
	identitySecret, err := macIdentity.ECDH(iosIdentity.PublicKey())
	if err != nil {
		t.Fatalf("derive identity secret: %v", err)
	}
	identityAuthKey := relayHKDF(t, identitySecret, nil, "cordcode-relay/identity-auth/v1"+vectors.Identity.ContextCanonical)

	iosPrekey := relayVectorKeyAgreement(t, vectors.Mailbox.IOSPrekeyPrivateKey, vectors.Mailbox.IOSPrekeyPublicKey)
	macEpoch := relayVectorKeyAgreement(t, vectors.Mailbox.MacEpochPrivateKey, vectors.Mailbox.MacEpochPublicKey)
	mailboxSecret, err := macEpoch.ECDH(iosPrekey.PublicKey())
	if err != nil {
		t.Fatalf("derive mailbox secret: %v", err)
	}
	mailboxRoot := relayHKDF(t, mailboxSecret, nil, "cordcode-relay/mailbox/v1"+vectors.Mailbox.ContextCanonical)
	macToIOS := relayHKDF(t, mailboxRoot, nil, "mac-to-ios")
	if got := base64.StdEncoding.EncodeToString(macToIOS); got != vectors.Mailbox.MacToIOSKey {
		t.Fatalf("mailbox key differs: got %s want %s", got, vectors.Mailbox.MacToIOSKey)
	}

	tag := hmac.New(sha256.New, identityAuthKey)
	tag.Write([]byte(vectors.Mailbox.EpochHeaderCanonical))
	epochTag := tag.Sum(nil)
	if got := base64.StdEncoding.EncodeToString(epochTag); got != vectors.Mailbox.EpochAuthTag {
		t.Fatalf("epoch auth tag differs: got %s want %s", got, vectors.Mailbox.EpochAuthTag)
	}
	digestInput := append([]byte(vectors.Mailbox.EpochHeaderCanonical), epochTag...)
	epochDigest := sha256.Sum256(digestInput)
	if got := base64.StdEncoding.EncodeToString(epochDigest[:]); got != vectors.Mailbox.EpochDigest {
		t.Fatalf("epoch digest differs: got %s want %s", got, vectors.Mailbox.EpochDigest)
	}

	plaintext := relayVectorPaddedPayload(t, vectors.Mailbox.InnerPayload)
	relayVectorSealAndOpen(t, macToIOS, vectors.Mailbox.Nonce, plaintext, vectors.Mailbox.AADCanonical, vectors.Mailbox.Ciphertext)
}

func TestRelayCryptoVectorHPKEBaseModeRFC9180(t *testing.T) {
	vectors := loadRelayCryptoVectors(t)
	vector := vectors.HPKEBaseMode
	if vector.Source != "RFC9180-test-vectors" ||
		vector.KEMID != uint16(hpke.KEM_X25519_HKDF_SHA256) ||
		vector.KDFID != uint16(hpke.KDF_HKDF_SHA256) ||
		vector.AEADID != uint16(hpke.AEAD_ChaCha20Poly1305) {
		t.Fatalf("unexpected HPKE suite vector: %+v", vector)
	}

	suite := hpke.NewSuite(
		hpke.KEM_X25519_HKDF_SHA256,
		hpke.KDF_HKDF_SHA256,
		hpke.AEAD_ChaCha20Poly1305,
	)
	scheme := hpke.KEM_X25519_HKDF_SHA256.Scheme()
	privateKey, err := scheme.UnmarshalBinaryPrivateKey(decodeVectorHex(t, vector.RecipientPrivateKeyHex))
	if err != nil {
		t.Fatalf("decode HPKE receiver private key: %v", err)
	}
	receiver, err := suite.NewReceiver(privateKey, decodeVectorHex(t, vector.InfoHex))
	if err != nil {
		t.Fatalf("construct HPKE receiver: %v", err)
	}
	opener, err := receiver.Setup(decodeVectorHex(t, vector.EncapsulatedKeyHex))
	if err != nil {
		t.Fatalf("setup HPKE base-mode receiver: %v", err)
	}
	plaintext, err := opener.Open(decodeVectorHex(t, vector.CiphertextHex), decodeVectorHex(t, vector.AADHex))
	if err != nil {
		t.Fatalf("open RFC 9180 HPKE ciphertext: %v", err)
	}
	if got := hex.EncodeToString(plaintext); got != vector.PlaintextHex {
		t.Fatalf("HPKE plaintext differs: got %s want %s", got, vector.PlaintextHex)
	}
}
