package gobridge

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// ResolveAdvertisedHost 返回给 iOS/外部客户端使用的 Bridge 主机地址。
// 优先级：显式环境变量 > 非回环私网 IPv4 > 非回环全局 IPv4 > 127.0.0.1。
func ResolveAdvertisedHost() string {
	if explicit := strings.TrimSpace(os.Getenv("GO_BRIDGE_ADVERTISE_HOST")); explicit != "" {
		return explicit
	}

	if host, ok := detectPreferredAdvertiseIP(); ok {
		return host
	}

	return "127.0.0.1"
}

// BuildBridgeLocalURL 构建对外广播的 Bridge WebSocket URL。
func BuildBridgeLocalURL(host string, port int) string {
	return fmt.Sprintf("ws://%s:%d/bridge", host, port)
}

func detectPreferredAdvertiseIP() (string, bool) {
	interfaces, err := net.Interfaces()
	if err != nil {
		return "", false
	}

	var candidates []net.IP
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			switch v := addr.(type) {
			case *net.IPNet:
				if ip := v.IP.To4(); ip != nil && !ip.IsLoopback() {
					candidates = append(candidates, ip)
				}
			case *net.IPAddr:
				if ip := v.IP.To4(); ip != nil && !ip.IsLoopback() {
					candidates = append(candidates, ip)
				}
			}
		}
	}

	if ip := selectPreferredAdvertiseIP(candidates); ip != nil {
		return ip.String(), true
	}

	return "", false
}

func selectPreferredAdvertiseIP(candidates []net.IP) net.IP {
	var fallback net.IP
	for _, candidate := range candidates {
		if candidate == nil {
			continue
		}
		if isPrivateIPv4(candidate) {
			return candidate
		}
		if fallback == nil && candidate.IsGlobalUnicast() {
			fallback = candidate
		}
	}
	return fallback
}

func isPrivateIPv4(ip net.IP) bool {
	ip = ip.To4()
	if ip == nil {
		return false
	}

	return ip[0] == 10 ||
		(ip[0] == 172 && ip[1] >= 16 && ip[1] <= 31) ||
		(ip[0] == 192 && ip[1] == 168)
}

// detectTailscaleIP 在所有非回环网络接口中查找 Tailscale CGNAT (100.64-127.x) 地址。
// 找到则返回 IP 字符串，否则返回空字符串。用于自动填充 QR 码中的 remote 字段。
func detectTailscaleIP() string {
	interfaces, err := net.Interfaces()
	if err != nil {
		return ""
	}

	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			var ip net.IP
			switch v := addr.(type) {
			case *net.IPNet:
				ip = v.IP
			case *net.IPAddr:
				ip = v.IP
			}
			if ipv4 := ip.To4(); ipv4 != nil && isTailscaleCGNAT(ipv4) {
				return ipv4.String()
			}
		}
	}

	return ""
}
