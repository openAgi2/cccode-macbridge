package gobridge

import (
	"crypto/tls"
	"fmt"
	"log/slog"
)

// tailscaleRemoteDecision 表示 Tailscale 远程候选的解析结果。
type tailscaleRemoteDecision struct {
	tailscaleURL string
	tlsCert      *tls.Certificate
}

// resolveTailscaleRemote 决定 Tailscale 远程候选 URL 与证书（P1-4 fail-closed）。
//
// 规则：
//   - tsIP 为空：无 Tailscale 候选。
//   - tlsPort > 0 且证书生成成功：发布 wss:// 候选 + 证书。
//   - TLS 不可用（证书失败或 tlsPort==0）：
//   - devInsecureWS=true（开发）：发布明文 ws:// 候选。
//   - devInsecureWS=false（产品）：不发布候选，记录明确错误，绝不降级为明文。
//
// 这是 P1-4 的核心：产品模式下 TLS 失败不得自动降级为明文 ws://，避免在链路上
// 暴露 bearer token、RPC 与 agent 输出，也不把真实安全故障表现为“仍可用”。
func resolveTailscaleRemote(tsIP string, tlsPort, port int, devInsecureWS bool) tailscaleRemoteDecision {
	if tsIP == "" {
		return tailscaleRemoteDecision{}
	}
	if tlsPort > 0 {
		cert, err := generateSelfSignedCert(tsIP)
		if err == nil {
			return tailscaleRemoteDecision{
				tailscaleURL: fmt.Sprintf("wss://%s:%d/bridge", tsIP, tlsPort),
				tlsCert:      cert,
			}
		}
		if !devInsecureWS {
			slog.Error("go-bridge: Tailscale 自签名证书生成失败，远程候选已禁用（产品模式不降级为 ws://）", "ip", tsIP, "tlsPort", tlsPort, "error", err)
			return tailscaleRemoteDecision{}
		}
		slog.Warn("go-bridge: dev-insecure-ws 启用，自签名证书生成失败，Tailscale 远程候选降级为明文 ws://（仅限开发）", "error", err)
		return tailscaleRemoteDecision{tailscaleURL: fmt.Sprintf("ws://%s:%d/bridge", tsIP, port)}
	}
	// tlsPort == 0
	if !devInsecureWS {
		slog.Error("go-bridge: 检测到 Tailscale IP 但 tls-port=0，远程候选已禁用（产品模式不降级为 ws://）", "ip", tsIP)
		return tailscaleRemoteDecision{}
	}
	slog.Warn("go-bridge: dev-insecure-ws 启用，tls-port=0 时 Tailscale 远程候选使用明文 ws://（仅限开发）", "ip", tsIP)
	return tailscaleRemoteDecision{tailscaleURL: fmt.Sprintf("ws://%s:%d/bridge", tsIP, port)}
}
