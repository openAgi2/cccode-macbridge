package gobridge

import (
	"net"
	"testing"
)

func TestBuildBridgeLocalURL_IncludesBridgePath(t *testing.T) {
	got := BuildBridgeLocalURL("192.168.1.25", 8777)
	want := "ws://192.168.1.25:8777/bridge"
	if got != want {
		t.Fatalf("BuildBridgeLocalURL() = %q, want %q", got, want)
	}
}

func TestSelectPreferredAdvertiseIP_PrefersPrivateIPv4(t *testing.T) {
	selected := selectPreferredAdvertiseIP([]net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("8.8.8.8"),
		net.ParseIP("192.168.31.8"),
	})
	if selected == nil || selected.String() != "192.168.31.8" {
		t.Fatalf("selected = %v, want 192.168.31.8", selected)
	}
}

func TestSelectPreferredAdvertiseIP_FallsBackToGlobalIPv4(t *testing.T) {
	selected := selectPreferredAdvertiseIP([]net.IP{
		net.ParseIP("127.0.0.1"),
		net.ParseIP("8.8.8.8"),
	})
	if selected == nil || selected.String() != "8.8.8.8" {
		t.Fatalf("selected = %v, want 8.8.8.8", selected)
	}
}
