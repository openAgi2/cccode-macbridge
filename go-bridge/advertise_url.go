package gobridge

import (
	"fmt"
	"net"
	"os"
	"sort"
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

// isLanCandidateIPv4 判断一个 IPv4 是否适合作为 LAN 直连候选。
// 排除:回环、link-local(169.254.x)、Tailscale CGNAT(100.64-127.x,需独立 TLS pin)、
// 以及非私网地址(公网 IPv4 不作为 LAN 候选,走 relay/remote)。
// 仅保留 RFC1918 私网 IPv4(10.x / 172.16-31.x / 192.168.x)。纯函数,便于单测。
func isLanCandidateIPv4(ip net.IP) bool {
	ipv4 := ip.To4()
	if ipv4 == nil {
		return false
	}
	if ipv4.IsLoopback() || ipv4.IsLinkLocalUnicast() {
		return false
	}
	if isTailscaleCGNAT(ipv4) {
		return false
	}
	return isPrivateIPv4(ipv4)
}

// isLikelyVirtualInterface 判断接口名是否为明显的虚拟/容器网桥。
// 仅排除 Linux/容器风格接口(docker*/br-*/veth*/virbr*)——它们从不是 macOS 的 WiFi/以太网承载路径。
// 故意保留 macOS 的 bridge*/vmnet*/utun*/vnic*(共享网络/虚拟机网在某些配置下可能承载可达 IP):
// 即便它们对 iPhone 不可达,iOS direct race 会自然忽略失败的候选;而误排除真实接口会让整条 LAN-first 失效。
// 纯函数,便于单测。
func isLikelyVirtualInterface(name string) bool {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "docker"),
		strings.HasPrefix(n, "br-"),
		strings.HasPrefix(n, "veth"),
		strings.HasPrefix(n, "virbr"):
		return true
	}
	return false
}

// detectLanCandidateIPs 枚举本机适合作为 LAN 直连候选的 IPv4(去重,保留接口枚举顺序)。
func detectLanCandidateIPs() []net.IP {
	interfaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []net.IP
	seen := make(map[string]struct{})
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		if isLikelyVirtualInterface(iface.Name) {
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
			if !isLanCandidateIPv4(ip) {
				continue
			}
			s := ip.String()
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, ip)
		}
	}
	return out
}

// BuildBridgeLocalURLs 构建全部 LAN 直连候选 WebSocket URL(ws://<ip>:<port>/bridge),
// 主候选(ResolveAdvertisedHost)排在最前,其余按 IP 字符串稳定排序,便于测试与 iOS primary 选择。
// 显式 GO_BRIDGE_ADVERTISE_HOST 时只返回该 host 单条(尊重手动覆盖,不枚举其他接口)。
// 用于 relay-first 配对(RelayFirstResult.LocalURLs,iOS 取 [0] 为 primary)与 hello_ack locals。
func BuildBridgeLocalURLs(port int) []string {
	advertised := ResolveAdvertisedHost()
	if explicit := strings.TrimSpace(os.Getenv("GO_BRIDGE_ADVERTISE_HOST")); explicit != "" {
		return []string{BuildBridgeLocalURL(advertised, port)}
	}
	ips := detectLanCandidateIPs()
	if len(ips) == 0 {
		return nil
	}
	var primary, others []string
	seen := make(map[string]struct{})
	for _, ip := range ips {
		s := ip.String()
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		u := BuildBridgeLocalURL(s, port)
		if s == advertised {
			primary = append(primary, u)
		} else {
			others = append(others, u)
		}
	}
	sort.Strings(others)
	return append(primary, others...)
}
