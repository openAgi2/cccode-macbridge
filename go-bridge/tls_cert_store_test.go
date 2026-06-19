package gobridge

import (
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestComputeSPKIPin_MatchesManualPKIXDer 锁定 ComputeSPKIPin 的口径：
// 必须等于 base64(SHA256(x509.MarshalPKIXPublicKey(pubkey)))。
// 这是 OpenSSL `x509 -pubkey | pkey -outform DER | dgst -sha256 | base64` 的 Go 等价。
// 若未来误改成哈希整个证书或哈希原始公钥字节，本测试会红。
func TestComputeSPKIPin_MatchesManualPKIXDer(t *testing.T) {
	cert, err := generateSelfSignedCert("100.64.1.2")
	if err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	got, err := ComputeSPKIPin(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ComputeSPKIPin: %v", err)
	}

	// 手动用同一口径计算期望值。
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatalf("parse cert: %v", err)
	}
	spkiDER, err := x509.MarshalPKIXPublicKey(leaf.PublicKey)
	if err != nil {
		t.Fatalf("marshal PKIX: %v", err)
	}
	sum := sha256.Sum256(spkiDER)
	want := base64.StdEncoding.EncodeToString(sum[:])

	if got != want {
		t.Fatalf("SPKI pin 口径不一致: ComputeSPKIPin=%q manual=%q", got, want)
	}
}

// TestComputeSPKIPin_OutputIsBase64SHA256 断言输出是 44 字符 base64（32 字节 SHA-256）。
func TestComputeSPKIPin_OutputIsBase64SHA256(t *testing.T) {
	cert, err := generateSelfSignedCert("127.0.0.1")
	if err != nil {
		t.Fatalf("generateSelfSignedCert: %v", err)
	}
	got, err := ComputeSPKIPin(cert.Certificate[0])
	if err != nil {
		t.Fatalf("ComputeSPKIPin: %v", err)
	}
	decoded, err := base64.StdEncoding.DecodeString(got)
	if err != nil {
		t.Fatalf("pin 不是合法 base64: %v (pin=%q)", err, got)
	}
	if len(decoded) != 32 {
		t.Fatalf("pin 应为 32 字节 SHA-256, got %d 字节 (pin=%q)", len(decoded), got)
	}
}

// TestLoadOrCreateTLSCert_PersistsAcrossCalls 证明证书持久化：
// 同一 dataDir 两次 LoadOrCreate 返回同一 SPKI（跨调用稳定，不是每次随机）。
func TestLoadOrCreateTLSCert_PersistsAcrossCalls(t *testing.T) {
	dir := t.TempDir()
	dataDir := NewDataDir(dir)

	cert1, pin1, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("first LoadOrCreateTLSCert: %v", err)
	}
	cert2, pin2, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("second LoadOrCreateTLSCert: %v", err)
	}

	// 证书 DER 应字节相同（来自磁盘）。
	if string(cert1.Certificate[0]) != string(cert2.Certificate[0]) {
		t.Fatal("两次 LoadOrCreate 返回不同证书，持久化失败")
	}
	// pin 值应相同。
	if pin1.Value != pin2.Value {
		t.Fatalf("两次 pin 不同: %q vs %q", pin1.Value, pin2.Value)
	}
	if pin1.Generation != 1 || pin2.Generation != 1 {
		t.Fatalf("首次生成 generation 应为 1, got %d / %d", pin1.Generation, pin2.Generation)
	}

	// 文件确实落盘。
	if _, err := os.Stat(filepath.Join(dir, "tls-cert.json")); err != nil {
		t.Fatalf("tls-cert.json 未落盘: %v", err)
	}
}

// TestLoadOrCreateTLSCert_GeneratesValidPin 断言返回的 BridgeV1TLSPin 字段齐全。
func TestLoadOrCreateTLSCert_GeneratesValidPin(t *testing.T) {
	dir := t.TempDir()
	dataDir := NewDataDir(dir)

	_, pin, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("LoadOrCreateTLSCert: %v", err)
	}
	if pin.Algorithm != TLSPinAlgorithm {
		t.Fatalf("algorithm=%q want %q", pin.Algorithm, TLSPinAlgorithm)
	}
	if pin.Value == "" {
		t.Fatal("pin value 为空")
	}
	if pin.Generation != 1 {
		t.Fatalf("generation=%d want 1", pin.Generation)
	}
	if pin.PreviousValue != "" {
		t.Fatalf("首次生成不应有 previousValue, got %q", pin.PreviousValue)
	}
	if pin.PreviousValidUntilMillis != 0 {
		t.Fatalf("首次生成不应有 previousValidUntil, got %d", pin.PreviousValidUntilMillis)
	}
}

// TestLoadOrCreateTLSCert_NilDataDirNoPin dataDir=nil 时不持久化、不派生 pin
// （开发/测试退化路径，保持与原 generateSelfSignedCert 行为一致）。
func TestLoadOrCreateTLSCert_NilDataDirNoPin(t *testing.T) {
	cert, pin, err := LoadOrCreateTLSCert(nil, "100.64.1.2")
	if err != nil {
		t.Fatalf("LoadOrCreateTLSCert nil: %v", err)
	}
	if cert == nil {
		t.Fatal("cert 为 nil")
	}
	if pin != nil {
		t.Fatalf("dataDir=nil 不应派生 pin, got %+v", pin)
	}
}

// TestLoadOrCreateTLSCert_CorruptFileRegenerates 损坏的 tls-cert.json 应触发重新生成
// 而非致命错误（启动韧性）。
func TestLoadOrCreateTLSCert_CorruptFileRegenerates(t *testing.T) {
	dir := t.TempDir()
	dataDir := NewDataDir(dir)
	// 写入损坏 JSON。
	if err := os.WriteFile(filepath.Join(dir, "tls-cert.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatalf("write corrupt file: %v", err)
	}
	cert, pin, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("损坏文件应触发重新生成而非报错: %v", err)
	}
	if cert == nil || pin == nil {
		t.Fatal("重新生成后 cert/pin 不应为 nil")
	}
	if pin.Generation != 1 {
		t.Fatalf("重新生成 generation 应为 1, got %d", pin.Generation)
	}
}

// TestRotateTLSCert_PopulatesPreviousWindow 轮换后：
//   - generation 递增
//   - previousValue = 旧证书 SPKI
//   - previousValidUntil 在 now ~ now+window 之间
//   - value = 新证书 SPKI（不同于 previous）
func TestRotateTLSCert_PopulatesPreviousWindow(t *testing.T) {
	dir := t.TempDir()
	dataDir := NewDataDir(dir)

	// 先建立初始证书。
	_, oldPin, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("initial LoadOrCreate: %v", err)
	}

	before := time.Now()
	newCert, newPin, err := RotateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("RotateTLSCert: %v", err)
	}
	after := time.Now()

	if newPin.Generation != oldPin.Generation+1 {
		t.Fatalf("generation 应递增: old=%d new=%d", oldPin.Generation, newPin.Generation)
	}
	if newPin.PreviousValue != oldPin.Value {
		t.Fatalf("previousValue 应等于旧 pin: prev=%q old=%q", newPin.PreviousValue, oldPin.Value)
	}
	if newPin.Value == newPin.PreviousValue {
		t.Fatal("新 pin value 不应等于 previous（说明没真正轮换）")
	}

	// 验证新 pin value 确实对应新证书。
	newSPI, err := ComputeSPKIPin(newCert.Certificate[0])
	if err != nil {
		t.Fatalf("compute new cert SPKI: %v", err)
	}
	if newPin.Value != newSPI {
		t.Fatalf("新 pin value 不匹配新证书 SPKI: pin=%q cert=%q", newPin.Value, newSPI)
	}

	// previousValidUntil 应在 [before+window-1s, after+window+1s] 容差内。
	validUntil := time.UnixMilli(newPin.PreviousValidUntilMillis)
	windowMinusSlack := before.Add(tlsPinRotationWindow - time.Second)
	windowPlusSlack := after.Add(tlsPinRotationWindow + time.Second)
	if validUntil.Before(windowMinusSlack) || validUntil.After(windowPlusSlack) {
		t.Fatalf("previousValidUntil %v 不在轮换窗口 [%v, %v] 内",
			validUntil, windowMinusSlack, windowPlusSlack)
	}
}

// TestPinFromStored_RoundTrip 存储→加载→pin 派生应稳定可复现。
func TestPinFromStored_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	dataDir := NewDataDir(dir)

	_, pin1, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("first load: %v", err)
	}
	// 第二次加载（从磁盘），pin 应完全一致。
	_, pin2, err := LoadOrCreateTLSCert(dataDir, "100.64.1.2")
	if err != nil {
		t.Fatalf("second load: %v", err)
	}
	if pin1.Value != pin2.Value || pin1.Generation != pin2.Generation {
		t.Fatalf("pin 不一致: %+v vs %+v", pin1, pin2)
	}
}
