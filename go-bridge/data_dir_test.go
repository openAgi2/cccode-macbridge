package gobridge

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestDataDir_Initialize 测试 Initialize 创建所有目录和文件。
func TestDataDir_Initialize(t *testing.T) {
	root := t.TempDir()
	dd := NewDataDir(root)

	if err := dd.Initialize(); err != nil {
		t.Fatalf("Initialize 失败: %v", err)
	}

	// 检查子目录存在
	for _, sub := range []string{"pairing", "logs"} {
		p := filepath.Join(root, sub)
		if fi, err := os.Stat(p); err != nil {
			t.Errorf("子目录 %s 不存在: %v", sub, err)
		} else if !fi.IsDir() {
			t.Errorf("子目录 %s 不是目录", sub)
		}
	}

	// 检查 identity.json 存在
	id, err := dd.ReadIdentity()
	if err != nil {
		t.Fatalf("ReadIdentity 失败: %v", err)
	}
	if id.BridgeID == "" {
		t.Error("BridgeID 为空")
	}
	if id.DisplayName == "" {
		t.Error("DisplayName 为空")
	}
	if id.Protocol.Name != BridgeProtocolName {
		t.Errorf("Protocol.Name = %q, want %q", id.Protocol.Name, BridgeProtocolName)
	}
	if id.Protocol.Version != BridgeProtocolVersion {
		t.Errorf("Protocol.Version = %d, want %d", id.Protocol.Version, BridgeProtocolVersion)
	}
	if id.Protocol.SchemaRevision != BridgeProtocolSchemaRevision {
		t.Errorf("Protocol.SchemaRevision = %q, want %q", id.Protocol.SchemaRevision, BridgeProtocolSchemaRevision)
	}
	if id.RuntimeVersion != runtimeVersion {
		t.Errorf("RuntimeVersion = %q, want %q", id.RuntimeVersion, runtimeVersion)
	}

	// 检查 config.json 存在
	cfg, err := dd.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig 失败: %v", err)
	}
	if cfg.Raw == nil || string(cfg.Raw) != "{}" {
		t.Errorf("初始 config 内容 = %q, want {}", string(cfg.Raw))
	}
}

// TestDataDir_Initialize_Idempotent 测试重复 Initialize 不覆盖 identity。
func TestDataDir_Initialize_Idempotent(t *testing.T) {
	root := t.TempDir()
	dd := NewDataDir(root)

	if err := dd.Initialize(); err != nil {
		t.Fatalf("第一次 Initialize 失败: %v", err)
	}
	origID, err := dd.ReadIdentity()
	if err != nil {
		t.Fatalf("第一次 ReadIdentity 失败: %v", err)
	}

	if err := dd.Initialize(); err != nil {
		t.Fatalf("第二次 Initialize 失败: %v", err)
	}
	secondID, err := dd.ReadIdentity()
	if err != nil {
		t.Fatalf("第二次 ReadIdentity 失败: %v", err)
	}

	if origID.BridgeID != secondID.BridgeID {
		t.Errorf("幂等性失败: BridgeID 从 %s 变为 %s", origID.BridgeID, secondID.BridgeID)
	}
	if !origID.CreatedAt.Equal(secondID.CreatedAt) {
		t.Errorf("幂等性失败: CreatedAt 从 %s 变为 %s", origID.CreatedAt, secondID.CreatedAt)
	}
}

// TestDataDir_ReadIdentity_ValidProtocol 测试 ReadIdentity 返回正确的协议字段。
func TestDataDir_ReadIdentity_ValidProtocol(t *testing.T) {
	root := t.TempDir()
	dd := NewDataDir(root)

	if err := dd.Initialize(); err != nil {
		t.Fatalf("Initialize 失败: %v", err)
	}

	id, err := dd.ReadIdentity()
	_ = id // 验证 ReadIdentity 能成功解析即可
	if err != nil {
		t.Fatalf("ReadIdentity 失败: %v", err)
	}

	// 验证 protocol 嵌套结构正确序列化
	raw, err := os.ReadFile(dd.IdentityPath())
	if err != nil {
		t.Fatalf("读取 identity.json 失败: %v", err)
	}
	var parsed map[string]interface{}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		t.Fatalf("解析 identity.json 失败: %v", err)
	}
	proto, ok := parsed["protocol"].(map[string]interface{})
	if !ok {
		t.Fatal("identity.json 缺少 protocol 对象")
	}
	if proto["name"] != BridgeProtocolName {
		t.Errorf("protocol.name = %v, want %s", proto["name"], BridgeProtocolName)
	}
	// JSON 数字默认解析为 float64
	if v, _ := proto["version"].(float64); int(v) != BridgeProtocolVersion {
		t.Errorf("protocol.version = %v, want %d", v, BridgeProtocolVersion)
	}
	if proto["schemaRevision"] != BridgeProtocolSchemaRevision {
		t.Errorf("protocol.schemaRevision = %v, want %s", proto["schemaRevision"], BridgeProtocolSchemaRevision)
	}
}

// TestDataDir_ReadConfig_InvalidJSON 测试损坏 JSON 返回 ConfigInvalidError。
func TestDataDir_ReadConfig_InvalidJSON(t *testing.T) {
	root := t.TempDir()
	dd := NewDataDir(root)

	// 写入损坏的 JSON
	badJSON := []byte("{this is not valid json!!!")
	configPath := dd.ConfigPath()
	if err := os.WriteFile(configPath, badJSON, 0o644); err != nil {
		t.Fatalf("写入损坏 config 失败: %v", err)
	}

	_, err := dd.ReadConfig()
	if err == nil {
		t.Fatal("ReadConfig 应该返回错误，但返回 nil")
	}

	var cfgErr *ConfigInvalidError
	if !errors.As(err, &cfgErr) {
		t.Errorf("错误类型 = %T, want *ConfigInvalidError", err)
	}
	if !strings.Contains(err.Error(), "config_invalid") {
		t.Errorf("错误消息 %q 不包含 config_invalid", err.Error())
	}
}

// TestDataDir_ConfigRoundTrip 测试 WriteConfig + ReadConfig 往返。
func TestDataDir_ConfigRoundTrip(t *testing.T) {
	root := t.TempDir()
	dd := NewDataDir(root)

	// 先创建目录结构
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatalf("创建目录失败: %v", err)
	}

	input := map[string]interface{}{
		"backends": map[string]interface{}{
			"claude": map[string]interface{}{
				"enabled": true,
				"model":   "opus",
			},
		},
		"theme": "dark",
	}
	raw, err := json.Marshal(input)
	if err != nil {
		t.Fatalf("序列化测试配置失败: %v", err)
	}

	cfg := &ConfigData{Raw: raw}
	if err := dd.WriteConfig(cfg); err != nil {
		t.Fatalf("WriteConfig 失败: %v", err)
	}

	got, err := dd.ReadConfig()
	if err != nil {
		t.Fatalf("ReadConfig 失败: %v", err)
	}

	var gotParsed map[string]interface{}
	if err := json.Unmarshal(got.Raw, &gotParsed); err != nil {
		t.Fatalf("解析读回的配置失败: %v", err)
	}

	backends, _ := gotParsed["backends"].(map[string]interface{})
	claude, _ := backends["claude"].(map[string]interface{})
	if claude["model"] != "opus" {
		t.Errorf("config round-trip: claude.model = %v, want opus", claude["model"])
	}
	if gotParsed["theme"] != "dark" {
		t.Errorf("config round-trip: theme = %v, want dark", gotParsed["theme"])
	}
}

// TestGenerateBridgeID 测试 ID 前缀和唯一性。
func TestGenerateBridgeID(t *testing.T) {
	seen := make(map[string]bool)

	for i := 0; i < 100; i++ {
		id := GenerateBridgeID()
		if !strings.HasPrefix(id, "brg_") {
			t.Errorf("GenerateBridgeID() = %q, 缺少 brg_ 前缀", id)
		}
		// brg_ + 16 bytes base64url = brg_ + 22 chars = 26 chars
		if len(id) != 26 {
			t.Errorf("GenerateBridgeID() = %q, 长度 = %d, want 26", id, len(id))
		}
		if seen[id] {
			t.Errorf("GenerateBridgeID() 产生重复 ID: %s", id)
		}
		seen[id] = true
	}
}

// TestDataDir_DoesNotWriteToCWD 测试 DataDir 不会在当前工作目录写入文件。
func TestDataDir_DoesNotWriteToCWD(t *testing.T) {
	// 使用明确的临时目录，不污染 cwd
	root := filepath.Join(t.TempDir(), "bridge_data")
	dd := NewDataDir(root)

	if err := dd.Initialize(); err != nil {
		t.Fatalf("Initialize 失败: %v", err)
	}

	// 验证所有文件都在 root 下，不在 cwd
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("获取 cwd 失败: %v", err)
	}

	// identity.json 不应在 cwd 中
	if _, err := os.Stat(filepath.Join(cwd, "identity.json")); err == nil {
		t.Error("Initialize 在 cwd 中创建了 identity.json")
	}
	// config.json 不应在 cwd 中
	if _, err := os.Stat(filepath.Join(cwd, "config.json")); err == nil {
		t.Error("Initialize 在 cwd 中创建了 config.json")
	}
	// 但应该在 root 中
	if _, err := os.Stat(dd.IdentityPath()); err != nil {
		t.Errorf("identity.json 不在数据目录中: %v", err)
	}
	if _, err := os.Stat(dd.ConfigPath()); err != nil {
		t.Errorf("config.json 不在数据目录中: %v", err)
	}
}
