package gobridge

import (
	"crypto/ecdh"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"sync"

	"golang.org/x/crypto/hkdf"
)

const (
	identityAuthLabel  = "cordcode-relay/identity-auth/v1"
	onlineTrafficLabel = "cordcode-relay/online/v1"
	iosToMacLabel      = "ios-to-mac"
	macToIosLabel      = "mac-to-ios"
	trafficKeyLen      = 32
	relayProtocolName  = "cccode-relay"
	relayProtocolRev   = "2026-05-24-r1"
)

type OnlineClientHello struct {
	Type                  string `json:"type"`
	BridgeID              string `json:"bridgeId"`
	DeviceID              string `json:"deviceId"`
	ChannelGeneration     uint64 `json:"channelGeneration"`
	IOSEphemeralPublicKey string `json:"iosEphemeralPublicKey"`
	ClientRandom          string `json:"clientRandom"`
	AuthTag               string `json:"authTag"`
}

type OnlineServerHello struct {
	Type                  string `json:"type"`
	BridgeID              string `json:"bridgeId"`
	DeviceID              string `json:"deviceId"`
	ChannelGeneration     uint64 `json:"channelGeneration"`
	ClientHelloHash       string `json:"clientHelloHash"`
	MacEphemeralPublicKey string `json:"macEphemeralPublicKey"`
	ServerRandom          string `json:"serverRandom"`
	KeyEpochID            string `json:"keyEpochId"`
	AuthTag               string `json:"authTag"`
}

// RelayCryptoIdentity 管理 Mac bridge 的长期 X25519 identity key pair。
// 方案 §5.3：长期 identity secret 认证临时公钥绑定，但不直接作为 traffic key 的秘密材料。
type RelayCryptoIdentity struct {
	mu         sync.Mutex
	privateKey *ecdh.PrivateKey
	publicKey  *ecdh.PublicKey
}

// LoadOrCreateRelayCryptoIdentity 从 data-dir 加载或新建 bridge crypto identity。
func LoadOrCreateRelayCryptoIdentity(secureDataDir string) (*RelayCryptoIdentity, error) {
	keyPath := secureDataDir + "/relay_identity.key"

	priv, err := loadX25519PrivateKey(keyPath)
	if err != nil {
		slog.Info("go-bridge: creating new relay crypto identity key pair", "path", keyPath)
		priv, err = generateAndSaveX25519PrivateKey(keyPath)
		if err != nil {
			return nil, fmt.Errorf("relay crypto identity: %w", err)
		}
	}

	return &RelayCryptoIdentity{
		privateKey: priv,
		publicKey:  priv.PublicKey(),
	}, nil
}

// PublicKey 返回 bridge 的长期公钥。
func (ci *RelayCryptoIdentity) PublicKey() *ecdh.PublicKey {
	return ci.publicKey
}

// PublicKeyBytes 返回 bridge 长期公钥的原始字节。
func (ci *RelayCryptoIdentity) PublicKeyBytes() []byte {
	if ci.publicKey == nil {
		return nil
	}
	return ci.publicKey.Bytes()
}

// Fingerprint 返回 bridge 长期公钥的 SHA-256 fingerprint（base64）。
func (ci *RelayCryptoIdentity) Fingerprint() string {
	if ci.publicKey == nil {
		return ""
	}
	h := sha256.Sum256(ci.publicKey.Bytes())
	return base64.RawURLEncoding.EncodeToString(h[:])
}

// DeriveIdentityAuthKey 从双方长期 identity keys 派生认证密钥。
// 方案 §5.3：
//
//	identityAuthKey = HKDF(X25519(localIdentityPrivateKey, remoteIdentityPublicKey),
//	                        "cordcode-relay/identity-auth/v1" + canonical([bridgeID, deviceID]))
func (ci *RelayCryptoIdentity) DeriveIdentityAuthKey(
	remoteIdentityPubKey []byte,
	bridgeID, deviceID string,
) ([]byte, error) {
	ci.mu.Lock()
	defer ci.mu.Unlock()

	remotePub, err := ecdh.X25519().NewPublicKey(remoteIdentityPubKey)
	if err != nil {
		return nil, fmt.Errorf("parse remote public key: %w", err)
	}

	shared, err := ci.privateKey.ECDH(remotePub)
	if err != nil {
		return nil, fmt.Errorf("identity ECDH: %w", err)
	}

	context, err := json.Marshal([]string{bridgeID, deviceID})
	if err != nil {
		return nil, fmt.Errorf("marshal identity context: %w", err)
	}
	return hkdfExpand(shared, append([]byte(identityAuthLabel), context...), 32)
}

// OnlineHandshakeState 管理一次在线 ECDHE 握手的状态。
// 方案 §5.3。
type OnlineHandshakeState struct {
	identityAuthKey []byte

	ephemeralPrivate *ecdh.PrivateKey
	ephemeralPublic  []byte
	serverRandom     []byte
	clientHello      []byte
	serverHello      []byte

	iosToMacKey []byte
	macToIosKey []byte
	completed   bool
}

// NewOnlineHandshake 创建 Mac 端的在线握手状态。
func NewOnlineHandshake(identityAuthKey []byte) (*OnlineHandshakeState, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate ephemeral: %w", err)
	}

	serverRandom := make([]byte, 32)
	if _, err := io.ReadFull(rand.Reader, serverRandom); err != nil {
		return nil, fmt.Errorf("server random: %w", err)
	}

	return &OnlineHandshakeState{
		identityAuthKey:  identityAuthKey,
		ephemeralPrivate: priv,
		ephemeralPublic:  priv.PublicKey().Bytes(),
		serverRandom:     serverRandom,
	}, nil
}

// AcceptClientHello 验证 canonical client hello，返回经认证的 server hello 并派生 traffic keys。
func (hs *OnlineHandshakeState) AcceptClientHello(hello OnlineClientHello) (*OnlineServerHello, error) {
	clientHello, err := canonicalOnlineClientHello(hello)
	if err != nil {
		return nil, err
	}
	clientAuthTag, err := base64.StdEncoding.DecodeString(hello.AuthTag)
	if err != nil || !hmac.Equal(clientAuthTag, hmacSHA256(hs.identityAuthKey, clientHello)) {
		return nil, fmt.Errorf("client hello auth tag verification failed")
	}
	iosEphemeralPublic, err := base64.StdEncoding.DecodeString(hello.IOSEphemeralPublicKey)
	if err != nil {
		return nil, fmt.Errorf("decode iOS ephemeral public: %w", err)
	}
	iosEphPub, err := ecdh.X25519().NewPublicKey(iosEphemeralPublic)
	if err != nil {
		return nil, fmt.Errorf("parse iOS ephemeral public: %w", err)
	}
	ephemeralShared, err := hs.ephemeralPrivate.ECDH(iosEphPub)
	if err != nil {
		return nil, fmt.Errorf("ephemeral ECDH: %w", err)
	}

	clientHelloHash := sha256.Sum256(clientHello)
	response := &OnlineServerHello{
		Type:                  "online_server_hello",
		BridgeID:              hello.BridgeID,
		DeviceID:              hello.DeviceID,
		ChannelGeneration:     hello.ChannelGeneration,
		ClientHelloHash:       base64.StdEncoding.EncodeToString(clientHelloHash[:]),
		MacEphemeralPublicKey: base64.StdEncoding.EncodeToString(hs.ephemeralPublic),
		ServerRandom:          base64.StdEncoding.EncodeToString(hs.serverRandom),
		KeyEpochID:            fmt.Sprintf("online:%d", hello.ChannelGeneration),
	}
	serverHello, err := canonicalOnlineServerHello(*response)
	if err != nil {
		return nil, err
	}
	response.AuthTag = base64.StdEncoding.EncodeToString(hmacSHA256(hs.identityAuthKey, serverHello))

	transcript := sha256.New()
	_, _ = transcript.Write(clientHello)
	_, _ = transcript.Write(serverHello)
	trafficRoot, err := hkdfDerive(ephemeralShared, transcript.Sum(nil), []byte(onlineTrafficLabel), 32)
	if err != nil {
		return nil, fmt.Errorf("traffic root HKDF: %w", err)
	}

	hs.iosToMacKey, err = hkdfExpand(trafficRoot, []byte(iosToMacLabel), trafficKeyLen)
	if err != nil {
		return nil, fmt.Errorf("ios-to-mac key: %w", err)
	}

	hs.macToIosKey, err = hkdfExpand(trafficRoot, []byte(macToIosLabel), trafficKeyLen)
	if err != nil {
		return nil, fmt.Errorf("mac-to-ios key: %w", err)
	}

	hs.clientHello = clientHello
	hs.serverHello = serverHello
	hs.completed = true
	return response, nil
}

func canonicalOnlineClientHello(hello OnlineClientHello) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"type":                  "online_client_hello",
		"protocol":              map[string]interface{}{"name": relayProtocolName, "version": 1, "schemaRevision": relayProtocolRev},
		"bridgeId":              hello.BridgeID,
		"deviceId":              hello.DeviceID,
		"channelGeneration":     hello.ChannelGeneration,
		"iosEphemeralPublicKey": hello.IOSEphemeralPublicKey,
		"clientRandom":          hello.ClientRandom,
	})
}

func canonicalOnlineServerHello(hello OnlineServerHello) ([]byte, error) {
	return json.Marshal(map[string]interface{}{
		"type":                  "online_server_hello",
		"bridgeId":              hello.BridgeID,
		"deviceId":              hello.DeviceID,
		"channelGeneration":     hello.ChannelGeneration,
		"clientHelloHash":       hello.ClientHelloHash,
		"macEphemeralPublicKey": hello.MacEphemeralPublicKey,
		"serverRandom":          hello.ServerRandom,
		"keyEpochId":            hello.KeyEpochID,
	})
}

// MacToIosKey 返回 Mac→iOS 方向的 traffic key。
func (hs *OnlineHandshakeState) MacToIosKey() []byte { return hs.macToIosKey }

// IosToMacKey 返回 iOS→Mac 方向的 traffic key。
func (hs *OnlineHandshakeState) IosToMacKey() []byte { return hs.iosToMacKey }

// Completed 返回握手是否完成。
func (hs *OnlineHandshakeState) Completed() bool { return hs.completed }

// Destroy 擦除所有密钥材料。
func (hs *OnlineHandshakeState) Destroy() {
	if hs.ephemeralPrivate != nil {
		zeroBytes(hs.ephemeralPrivate.Bytes())
	}
	zeroBytes(hs.iosToMacKey)
	zeroBytes(hs.macToIosKey)
	zeroBytes(hs.identityAuthKey)
	hs.completed = false
}

// ── 密钥文件 I/O ────────────────────────────────────────────────────────────

func generateAndSaveX25519PrivateKey(path string) (*ecdh.PrivateKey, error) {
	priv, err := ecdh.X25519().GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate X25519: %w", err)
	}
	if err := os.WriteFile(path, priv.Bytes(), 0600); err != nil {
		return nil, fmt.Errorf("write key file: %w", err)
	}
	return priv, nil
}

func loadX25519PrivateKey(path string) (*ecdh.PrivateKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if len(data) != 32 {
		return nil, fmt.Errorf("invalid X25519 key length: %d", len(data))
	}
	return ecdh.X25519().NewPrivateKey(data)
}

// ── 工具函数 ─────────────────────────────────────────────────────────────────

func hkdfExpand(secret, info []byte, length int) ([]byte, error) {
	return hkdfDerive(secret, nil, info, length)
}

func hkdfDerive(secret, salt, info []byte, length int) ([]byte, error) {
	reader := hkdf.New(sha256.New, secret, salt, info)
	out := make([]byte, length)
	if _, err := io.ReadFull(reader, out); err != nil {
		return nil, err
	}
	return out, nil
}

func hmacSHA256(key, data []byte) []byte {
	mac := hmac.New(sha256.New, key)
	mac.Write(data)
	return mac.Sum(nil)
}

func zeroBytes(b []byte) {
	for i := range b {
		b[i] = 0
	}
}

// PrivateKeyBytes 返回 bridge 长期私钥的原始字节。
// 仅用于 HPKE 配对解密等需要私钥的场景。
func (ci *RelayCryptoIdentity) PrivateKeyBytes() []byte {
	ci.mu.Lock()
	defer ci.mu.Unlock()
	if ci.privateKey == nil {
		return nil
	}
	return ci.privateKey.Bytes()
}
