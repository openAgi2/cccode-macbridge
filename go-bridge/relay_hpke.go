package gobridge

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"golang.org/x/crypto/chacha20poly1305"
	"log/slog"
	"sync"
	"time"

	"github.com/cloudflare/circl/hpke"
)

// ─── RFC 9180 HPKE Base Mode 配对实现 ────────────────────────────────────
//
// 方案 §6.2 / §11 Phase 3：
//   新设备 Relay 配对限定使用 RFC 9180 HPKE Base Mode；
//   X25519 + HKDF-SHA256 + ChaCha20-Poly1305。
//
// 配对流程：
//   1. Mac 生成配对 QR（包含 route ID + bridge identity public key fingerprint）
//   2. iOS 扫码，生成 HPKE claim（用 Mac public key 加密配对请求）
//   3. Mac 收到 claim，解密验证，发送 approve/reject
//   4. iOS 收到 approve，完成配对

const (
	hpkeModeBase   = 0 // HPKE Base Mode
	hpkeKEMX25519  = 0x0020
	hpkeKDFSHA256  = 0x0001
	hpkeAEADChaCha = 0x0003

	// HPKE label constants (RFC 9180 §4.1)
	hpkeLabelExtract   = "HPKE-v1/extract"
	hpkeLabelExpand    = "HPKE-v1/expand"
	hpkeLabelKEM       = "HPKE-v1/KEM"
	hpkeLabelHPKE      = "HPKE-v1"
	hpkeLabelPSKIDHash = "psk_id_hash"
	hpkeLabelInfoHash  = "info_hash"
	hpkeLabelKey       = "key"
	hpkeLabelBaseNonce = "base_nonce"
	hpkeLabelSecret    = "secret"
	hpkeLabelExporter  = "HPKE-v1/exporter"

	// KEM DHKEM(X25519, HKDF-SHA256) 常量
	kemLabelEAEPRK       = "eae_prk"
	kemLabelSharedSecret = "shared_secret"
	kemNsecret           = 32 // HKDF-SHA256 output length

	// 配对上下文
	pairingContextLabel = "cordcode-relay/pairing/v1"

	// 配对 claim 有效期
	pairingClaimTTL = 5 * time.Minute
)

// HPKECiphertext 是 HPKE 封装结果。
type HPKECiphertext struct {
	KEMOutput  []byte `json:"kem_output"` // encapsulated public key (32 bytes for X25519)
	Ciphertext []byte `json:"ciphertext"`
}

// PairingQR 是配对二维码内容。
type PairingQR struct {
	Version       uint8  `json:"version"`
	RouteID       string `json:"routeId"`
	BridgePubKey  string `json:"bridgePubKey"`      // base64, X25519 public key
	BridgeFP      string `json:"bridgeFingerprint"` // SHA-256 fingerprint (base64)
	RelayEndpoint string `json:"relayEndpoint,omitempty"`
	CreatedAt     int64  `json:"createdAt"` // Unix timestamp
}

// PairingClaim 是 iOS 端发送的配对请求。
type PairingClaim struct {
	Version        uint8           `json:"version"`
	DeviceID       string          `json:"deviceId"`
	DevicePubKey   string          `json:"devicePubKey"` // base64, X25519
	DeviceName     string          `json:"deviceName,omitempty"`
	Timestamp      int64           `json:"timestamp"` // Unix timestamp
	HPKECiphertext *HPKECiphertext `json:"hpke"`
}

// PairingApprove 是 Mac 端发送的配对批准。
type PairingApprove struct {
	Approved   bool   `json:"approved"`
	DeviceID   string `json:"deviceId"`
	RouteID    string `json:"routeId"`
	DeviceAuth string `json:"deviceAuth,omitempty"` // 生成的设备 auth credential
	Reason     string `json:"reason,omitempty"`     // 拒绝原因
}

// HPKEContext 是 HPKE sender/receiver 的共享上下文。
type HPKEContext struct {
	aeadKey        []byte
	baseNonce      []byte
	seq            uint64
	exporterSecret []byte
	sealer         hpke.Sealer
	opener         hpke.Opener
}

// PairingStore 管理 pending 配对请求。
type PairingStore struct {
	mu      sync.Mutex
	pending map[string]*pendingPairing // claimID -> pending
}

type pendingPairing struct {
	claim     *PairingClaim
	approved  bool
	createdAt time.Time
	expiresAt time.Time
}

// NewPairingStore 创建配对存储。
func NewPairingStore() *PairingStore {
	return &PairingStore{
		pending: make(map[string]*pendingPairing),
	}
}

// ── HPKE Seal/Open ──────────────────────────────────────────────────────

// HPKESeal 使用 RFC 9180 HPKE Base Mode 加密。
// pkR 是接收方公钥（X25519, 32 bytes）。
// info 是上下文信息。
// aad 是附加认证数据。
// plaintext 是待加密内容。
//
// RFC 9180 §4.1 (Base mode):
//  1. Generate ephemeral key pair (pkE, skE)
//  2. shared_secret = KEM.Encap(pkR)
//  3. key_schedule → (key, nonce, exporter_secret)
//  4. ciphertext = AEAD.Seal(key, nonce, aad, plaintext)
func HPKESeal(pkR, info, aad, plaintext []byte) (*HPKECiphertext, *HPKEContext, error) {
	suite := hpke.NewSuite(hpke.KEM_X25519_HKDF_SHA256, hpke.KDF_HKDF_SHA256, hpke.AEAD_ChaCha20Poly1305)

	scheme := hpke.KEM_X25519_HKDF_SHA256.Scheme()
	pkRKey, err := scheme.UnmarshalBinaryPublicKey(pkR)
	if err != nil {
		return nil, nil, fmt.Errorf("parse recipient public key: %w", err)
	}

	sender, err := suite.NewSender(pkRKey, info)
	if err != nil {
		return nil, nil, fmt.Errorf("create HPKE sender: %w", err)
	}

	enc, sealer, err := sender.Setup(nil)
	if err != nil {
		return nil, nil, fmt.Errorf("HPKE setup: %w", err)
	}

	ciphertext, err := sealer.Seal(plaintext, aad)
	if err != nil {
		return nil, nil, fmt.Errorf("HPKE seal: %w", err)
	}

	ctx := &HPKEContext{
		sealer: sealer,
	}

	return &HPKECiphertext{
		KEMOutput:  enc,
		Ciphertext: ciphertext,
	}, ctx, nil
}

// HPKEOpen 使用 RFC 9180 HPKE Base Mode 解密。
// skR 是接收方私钥（X25519, 32 bytes）。
func HPKEOpen(skR, info, aad []byte, ct *HPKECiphertext) ([]byte, *HPKEContext, error) {
	if ct == nil || len(ct.KEMOutput) != 32 {
		return nil, nil, fmt.Errorf("invalid KEM output")
	}
	if len(skR) != 32 {
		return nil, nil, fmt.Errorf("invalid recipient private key")
	}

	suite := hpke.NewSuite(hpke.KEM_X25519_HKDF_SHA256, hpke.KDF_HKDF_SHA256, hpke.AEAD_ChaCha20Poly1305)

	scheme := hpke.KEM_X25519_HKDF_SHA256.Scheme()
	normalizedPrivateKey := append([]byte(nil), skR...)
	normalizedPrivateKey[0] &= 248
	normalizedPrivateKey[31] &= 127
	normalizedPrivateKey[31] |= 64
	skRKey, err := scheme.UnmarshalBinaryPrivateKey(normalizedPrivateKey)
	if err != nil {
		return nil, nil, fmt.Errorf("parse recipient private key: %w", err)
	}

	recv, err := suite.NewReceiver(skRKey, info)
	if err != nil {
		return nil, nil, fmt.Errorf("create HPKE receiver: %w", err)
	}

	opener, err := recv.Setup(ct.KEMOutput)
	if err != nil {
		return nil, nil, fmt.Errorf("HPKE setup: %w", err)
	}

	plaintext, err := opener.Open(ct.Ciphertext, aad)
	if err != nil {
		return nil, nil, fmt.Errorf("HPKE open: %w", err)
	}

	return plaintext, &HPKEContext{opener: opener}, nil
}

// ── DHKEM ExtractAndExpand (RFC 9180 §4.1) ────────────────────────────

// kemExtractAndExpand 实现 DHKEM 的 ExtractAndExpand (RFC 9180 §4.1)。
// DHKEM(X25519, HKDF-SHA256):
//
//	eae_prk = LabeledExtract("", "eae_prk", dh)
//	shared_secret = LabeledExpand(eae_prk, "shared_secret", kem_context, Nsecret)
//
// 其中 kem_context = enc || pkRm。
func kemExtractAndExpand(dh, kemContext []byte) ([]byte, error) {
	// KEM suite_id = "KEM" || I2OSP(kem_id, 2)
	kemSuiteID := make([]byte, 0, 3+2)
	kemSuiteID = append(kemSuiteID, "KEM"...)
	kemSuiteID = appendUint16(kemSuiteID, hpkeKEMX25519)

	// eae_prk = LabeledExtract("", "eae_prk", dh)
	// LabeledExtract(salt, label, ikm) = HKDF-Extract(salt, HPKE-v1 || suite_id || label || ikm)
	labeledExtractInput := make([]byte, 0, len(hpkeLabelHPKE)+len(kemSuiteID)+len(kemLabelEAEPRK)+len(dh))
	labeledExtractInput = append(labeledExtractInput, hpkeLabelHPKE...)
	labeledExtractInput = append(labeledExtractInput, kemSuiteID...)
	labeledExtractInput = append(labeledExtractInput, kemLabelEAEPRK...)
	labeledExtractInput = append(labeledExtractInput, dh...)
	eaePRK := hkdfExtractHPKE(nil, labeledExtractInput)

	// shared_secret = LabeledExpand(eae_prk, "shared_secret", kem_context, Nsecret)
	// LabeledExpand(prk, label, info, L) = HKDF-Expand(prk, HPKE-v1 || suite_id || label || I2OSP(L, 2) || info, L)
	labeledExpandInput := make([]byte, 0, len(hpkeLabelHPKE)+len(kemSuiteID)+len(kemLabelSharedSecret)+2+len(kemContext))
	labeledExpandInput = append(labeledExpandInput, hpkeLabelHPKE...)
	labeledExpandInput = append(labeledExpandInput, kemSuiteID...)
	labeledExpandInput = append(labeledExpandInput, kemLabelSharedSecret...)
	labeledExpandInput = appendUint16(labeledExpandInput, uint16(kemNsecret))
	labeledExpandInput = append(labeledExpandInput, kemContext...)

	return hkdfExpandHPKE(eaePRK, labeledExpandInput, kemNsecret)
}

// ── HPKE Key Schedule (RFC 9180 §5.1) ─────────────────────────────────

func hpkeKeySchedule(sharedSecret, info []byte) (*HPKEContext, error) {
	// psk_id_hash = LabeledExtract("", "psk_id_hash", "")
	pskIDHash := labeledExtract(nil, []byte(hpkeLabelPSKIDHash), nil)

	// info_hash = LabeledExtract("", "info_hash", info)
	infoHash := labeledExtract(nil, []byte(hpkeLabelInfoHash), info)

	// key_schedule_context = mode || psk_id_hash || info_hash
	ksc := make([]byte, 0, 1+len(pskIDHash)+len(infoHash))
	ksc = append(ksc, hpkeModeBase)
	ksc = append(ksc, pskIDHash...)
	ksc = append(ksc, infoHash...)

	// secret = LabeledExtract(shared_secret, "secret", key_schedule_context)
	secret := labeledExtract(sharedSecret, []byte(hpkeLabelSecret), ksc)

	// key = LabeledExpand(secret, "key", key_schedule_context, Nk)
	key, err := labeledExpand(secret, []byte(hpkeLabelKey), ksc, 32)
	if err != nil {
		return nil, fmt.Errorf("derive key: %w", err)
	}

	// base_nonce = LabeledExpand(secret, "base_nonce", key_schedule_context, Nn)
	nonce, err := labeledExpand(secret, []byte(hpkeLabelBaseNonce), ksc, 12)
	if err != nil {
		return nil, fmt.Errorf("derive nonce: %w", err)
	}

	// exporter_secret = LabeledExpand(secret, "exporter", key_schedule_context, Nh)
	exporterSecret, err := labeledExpand(secret, []byte(hpkeLabelExporter), ksc, 32)
	if err != nil {
		return nil, fmt.Errorf("derive exporter secret: %w", err)
	}

	return &HPKEContext{
		aeadKey:        key,
		baseNonce:      nonce,
		seq:            0,
		exporterSecret: exporterSecret,
	}, nil
}

// ── HPKE Labeled Extract/Expand (RFC 9180 §4.1) ────────────────────────

func labeledExtract(salt, label, ikm []byte) []byte {
	// RFC 9180 §4.1:
	//   LabeledExtract(salt, label, ikm) =
	//     HKDF-Extract(salt, HPKE-v1 || suite_id || label || ikm)
	suiteID := buildSuiteID()

	labeledInput := make([]byte, 0, len(hpkeLabelHPKE)+len(suiteID)+len(label)+len(ikm))
	labeledInput = append(labeledInput, hpkeLabelHPKE...)
	labeledInput = append(labeledInput, suiteID...)
	labeledInput = append(labeledInput, label...)
	labeledInput = append(labeledInput, ikm...)

	return hkdfExtractHPKE(salt, labeledInput)
}

func labeledExpand(prk, label, info []byte, length int) ([]byte, error) {
	// RFC 9180 §4.1:
	//   LabeledExpand(prk, label, info, L) =
	//     HKDF-Expand(prk, HPKE-v1 || suite_id || label || I2OSP(L, 2) || info, L)
	suiteID := buildSuiteID()

	labeledInput := make([]byte, 0, len(hpkeLabelHPKE)+len(suiteID)+len(label)+2+len(info))
	labeledInput = append(labeledInput, hpkeLabelHPKE...)
	labeledInput = append(labeledInput, suiteID...)
	labeledInput = append(labeledInput, label...)
	labeledInput = appendUint16(labeledInput, uint16(length)) // I2OSP(L, 2)
	labeledInput = append(labeledInput, info...)

	return hkdfExpandHPKE(prk, labeledInput, length)
}

func buildSuiteID() []byte {
	// For HPKE mode (not KEM-only):
	// suite_id = "HPKE" || I2OSP(kem_id, 2) || I2OSP(kdf_id, 2) || I2OSP(aead_id, 2)
	suiteID := make([]byte, 0, 4+2+2+2)
	suiteID = append(suiteID, "HPKE"...)
	suiteID = appendUint16(suiteID, hpkeKEMX25519)
	suiteID = appendUint16(suiteID, hpkeKDFSHA256)
	suiteID = appendUint16(suiteID, hpkeAEADChaCha)
	return suiteID
}

func appendUint16(b []byte, v uint16) []byte {
	return append(b, byte(v>>8), byte(v))
}

// hkdfExtractHPKE 实现 HKDF-Extract (RFC 5869)，供 HPKE Key Schedule 使用。
func hkdfExtractHPKE(salt, ikm []byte) []byte {
	if salt == nil {
		salt = make([]byte, sha256.New().Size())
	}
	h := hmac.New(sha256.New, salt)
	h.Write(ikm)
	return h.Sum(nil)
}

// hkdfExpandHPKE 实现 HKDF-Expand (RFC 5869 §2.3)，供 HPKE Key Schedule 使用。
// 注意：不同于 hkdfDerive（使用 golang.org/x/crypto/hkdf 做 Extract+Expand），
// 这里只做单阶段 HKDF-Expand，以符合 RFC 9180 §4 规范。
func hkdfExpandHPKE(prk, info []byte, length int) ([]byte, error) {
	hashLen := sha256.New().Size()
	if length > 255*hashLen {
		return nil, fmt.Errorf("hkdf expand: length %d exceeds maximum", length)
	}
	// HKDF-Expand: T(1) = HMAC-Hash(PRK, info || 0x01)
	//              T(i) = HMAC-Hash(PRK, T(i-1) || info || I2OSP(i, 1))
	var prev, result []byte
	n := (length + hashLen - 1) / hashLen
	for i := 1; i <= n; i++ {
		h := hmac.New(sha256.New, prk)
		h.Write(prev)
		h.Write(info)
		h.Write([]byte{byte(i)})
		prev = h.Sum(nil)
		result = append(result, prev...)
	}
	return result[:length], nil
}

// ── HPKE AEAD Seal/Open ────────────────────────────────────────────────

func hpkeAEADSeal(ctx *HPKEContext, aad, plaintext []byte) ([]byte, error) {
	nonce := hpkeComputeNonce(ctx.baseNonce, ctx.seq)
	ctx.seq++

	// ChaCha20-Poly1305
	aead, err := createAEAD(ctx.aeadKey)
	if err != nil {
		return nil, err
	}
	return aead.Seal(nil, nonce, plaintext, aad), nil
}

func hpkeAEADOpen(ctx *HPKEContext, aad, ciphertext []byte) ([]byte, error) {
	nonce := hpkeComputeNonce(ctx.baseNonce, ctx.seq)
	ctx.seq++

	aead, err := createAEAD(ctx.aeadKey)
	if err != nil {
		return nil, err
	}
	return aead.Open(nil, nonce, ciphertext, aad)
}

func hpkeComputeNonce(baseNonce []byte, seq uint64) []byte {
	// RFC 9180 §5.2: nonce = base_nonce XOR I2OSP(seq, Nn)
	nonce := make([]byte, len(baseNonce))
	copy(nonce, baseNonce)

	// XOR sequence number into last 8 bytes (big-endian)
	for i := 0; i < 8; i++ {
		nonce[len(nonce)-1-i] ^= byte(seq >> (8 * i))
	}
	return nonce
}

// ── 配对流程 ────────────────────────────────────────────────────────────

// GeneratePairingQR 生成配对二维码内容。
func GeneratePairingQR(routeID string, bridgePubKey []byte, relayEndpoint string) (*PairingQR, error) {
	fp := sha256.Sum256(bridgePubKey)
	return &PairingQR{
		Version:       1,
		RouteID:       routeID,
		BridgePubKey:  base64.StdEncoding.EncodeToString(bridgePubKey),
		BridgeFP:      base64.RawURLEncoding.EncodeToString(fp[:]),
		RelayEndpoint: relayEndpoint,
		CreatedAt:     time.Now().Unix(),
	}, nil
}

// CreatePairingClaim iOS 端创建配对请求。
// 用 Mac 的 bridge public key 做 HPKE 加密配对数据。
func CreatePairingClaim(
	deviceID, deviceName string,
	devicePubKey []byte,
	bridgePubKey []byte,
) (*PairingClaim, error) {
	// 配对信息（被加密的内容）
	timestamp := time.Now().Unix()
	claimPayload := map[string]interface{}{
		"deviceId":     deviceID,
		"devicePubKey": base64.StdEncoding.EncodeToString(devicePubKey),
		"deviceName":   deviceName,
		"timestamp":    timestamp,
	}

	plaintext, err := json.Marshal(claimPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal claim payload: %w", err)
	}

	// HPKE 加密
	info := []byte(pairingContextLabel)
	aad := pairingClaimAAD(deviceID, timestamp)

	ct, _, err := HPKESeal(bridgePubKey, info, aad, plaintext)
	if err != nil {
		return nil, fmt.Errorf("HPKE seal: %w", err)
	}

	return &PairingClaim{
		Version:        1,
		DeviceID:       deviceID,
		DevicePubKey:   base64.StdEncoding.EncodeToString(devicePubKey),
		DeviceName:     deviceName,
		Timestamp:      timestamp,
		HPKECiphertext: ct,
	}, nil
}

// ProcessPairingClaim Mac 端处理配对请求。
// 用 bridge private key 解密 claim，验证内容。
func ProcessPairingClaim(
	claim *PairingClaim,
	bridgePrivKey []byte,
) (*PairingApprove, error) {
	// 检查时间戳
	claimTime := time.Unix(claim.Timestamp, 0)
	if time.Since(claimTime) > pairingClaimTTL {
		return &PairingApprove{
			Approved: false,
			DeviceID: claim.DeviceID,
			Reason:   "claim expired",
		}, nil
	}

	// HPKE 解密
	info := []byte(pairingContextLabel)
	aad := pairingClaimAAD(claim.DeviceID, claim.Timestamp)

	plaintext, _, err := HPKEOpen(bridgePrivKey, info, aad, claim.HPKECiphertext)
	if err != nil {
		return nil, fmt.Errorf("HPKE open: %w", err)
	}

	// 验证解密内容
	var payload map[string]interface{}
	if err := json.Unmarshal(plaintext, &payload); err != nil {
		return nil, fmt.Errorf("parse claim payload: %w", err)
	}

	// 验证 device ID 一致
	if did, ok := payload["deviceId"].(string); !ok || did != claim.DeviceID {
		return &PairingApprove{
			Approved: false,
			DeviceID: claim.DeviceID,
			Reason:   "device ID mismatch in encrypted payload",
		}, nil
	}

	// 验证 device public key 一致
	if dpk, ok := payload["devicePubKey"].(string); !ok || dpk != claim.DevicePubKey {
		return &PairingApprove{
			Approved: false,
			DeviceID: claim.DeviceID,
			Reason:   "device public key mismatch in encrypted payload",
		}, nil
	}

	slog.Info("pairing: claim verified",
		"deviceID", safeID(claim.DeviceID),
		"deviceName", claim.DeviceName,
	)

	return &PairingApprove{
		Approved:   true,
		DeviceID:   claim.DeviceID,
		DeviceAuth: generateRelayCredential(), // 生成设备 auth
	}, nil
}

func pairingClaimAAD(deviceID string, timestamp int64) []byte {
	return []byte(fmt.Sprintf("pairing:%s:%s", deviceID, time.Unix(timestamp, 0).UTC().Format(time.RFC3339)))
}

// ── HPKE Export Secret (用于派生配对后的共享密钥) ────────────────────────

// HPKEExportSecret 导出 HPKE exporter secret。
// 可用于派生配对后的长期 shared key。
func HPKEExportSecret(ctx *HPKEContext, exporterContext []byte, length int) ([]byte, error) {
	if ctx == nil {
		return nil, fmt.Errorf("nil HPKEContext")
	}
	if ctx.sealer != nil {
		return ctx.sealer.Export(exporterContext, uint(length)), nil
	}
	if ctx.opener != nil {
		return ctx.opener.Export(exporterContext, uint(length)), nil
	}
	return nil, fmt.Errorf("HPKE context does not support export")
}

// ── Hex 辅助（用于 test vector 验证） ────────────────────────────────────

// createAEAD 创建 ChaCha20-Poly1305 AEAD。
func createAEAD(key []byte) (chacha20poly1305AEAD, error) {
	return chacha20poly1305.New(key)
}

type chacha20poly1305AEAD interface {
	Seal(dst, nonce, plaintext, additionalData []byte) []byte
	Open(dst, nonce, ciphertext, additionalData []byte) ([]byte, error)
}

func mustDecodeHex(s string) []byte {
	b, err := hex.DecodeString(s)
	if err != nil {
		panic(fmt.Sprintf("hex decode: %v", err))
	}
	return b
}
