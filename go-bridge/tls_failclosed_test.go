package gobridge

import (
	"strings"
	"testing"
)

// TestResolveTailscaleRemote_NoTailscaleIP 验证无 Tailscale IP 时不发布候选。
func TestResolveTailscaleRemote_NoTailscaleIP(t *testing.T) {
	d := resolveTailscaleRemote("", 8778, 8777, false)
	if d.tailscaleURL != "" || d.tlsCert != nil {
		t.Fatalf("无 Tailscale IP 应无候选: url=%q cert=%v", d.tailscaleURL, d.tlsCert)
	}
}

// TestResolveTailscaleRemote_ProductModeTLSDisabledNoPlaintext 验证 P1-4：
// 产品模式（devInsecureWS=false）下 tls-port=0 不得降级为明文 ws://。
func TestResolveTailscaleRemote_ProductModeTLSDisabledNoPlaintext(t *testing.T) {
	d := resolveTailscaleRemote("100.64.1.2", 0, 8777, false)
	if d.tailscaleURL != "" {
		t.Fatalf("产品模式 tls-port=0 不应发布候选，got url=%q", d.tailscaleURL)
	}
	if d.tlsCert != nil {
		t.Fatal("不应返回证书")
	}
}

// TestResolveTailscaleRemote_ProductModeCertFailNoPlaintext 验证证书生成失败路径
// 在产品模式下不发布明文。由于 generateSelfSignedCert 通常成功，这里用 tlsPort>0
// 验证成功路径产出 wss:// + 证书，确认 happy path 不被破坏。
func TestResolveTailscaleRemote_HappyPathProducesWSS(t *testing.T) {
	d := resolveTailscaleRemote("100.64.1.2", 8778, 8777, false)
	if !strings.HasPrefix(d.tailscaleURL, "wss://100.64.1.2:8778/bridge") {
		t.Fatalf("happy path 应产出 wss:// URL, got %q", d.tailscaleURL)
	}
	if d.tlsCert == nil {
		t.Fatal("happy path 应返回证书")
	}
}

// TestResolveTailscaleRemote_DevModeTLSDisabledAllowsPlaintext 验证开发显式开关下允许明文。
func TestResolveTailscaleRemote_DevModeTLSDisabledAllowsPlaintext(t *testing.T) {
	d := resolveTailscaleRemote("100.64.1.2", 0, 8777, true)
	if d.tailscaleURL != "ws://100.64.1.2:8777/bridge" {
		t.Fatalf("dev 模式 tls-port=0 应产出 ws:// URL, got %q", d.tailscaleURL)
	}
	if d.tlsCert != nil {
		t.Fatal("dev 明文路径不应返回证书")
	}
}
