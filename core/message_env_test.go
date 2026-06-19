package core

import (
	"sort"
	"strings"
	"testing"
)

func envSliceToMap(env []string) map[string]string {
	m := make(map[string]string, len(env))
	for _, e := range env {
		k, v, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		m[k] = v
	}
	return m
}

// controlPlaneSecrets is the canonical set of control-plane vars that must
// never reach an agent subprocess, regardless of which layer carries them.
func controlPlaneSecrets() map[string]string {
	return map[string]string{
		"CCCODE_MANAGEMENT_TOKEN": "mgmt-secret-token",
		"CCCODE_RELAY_CREDENTIAL": "relay-opaque-cred",
		"CCCODE_RELAY_ROUTE_ID":   "route-abc-123",
		"CCCODE_RELAY_ENDPOINT":   "wss://relay.example.com",
		"OPENCODE_SERVER_USERNAME": "oc-user",
		"OPENCODE_SERVER_PASSWORD": "oc-pass",
		"CLAUDECODE":              "nested-session-marker",
		// Other CCCODE_* control-plane vars must also be stripped.
		"CCCODE_DEV_INSECURE_WS":      "1",
		"CCCODE_RELAY_VPS_HOST":       "1.2.3.4",
		"CCCODE_RELAY_VPS_PASS":       "vps-password",
	}
}

func mapToSlice(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k, v := range m {
		out = append(out, k+"="+v)
	}
	sort.Strings(out)
	return out
}

func TestBuildAgentEnv_StripsControlPlaneFromBase(t *testing.T) {
	secrets := controlPlaneSecrets()
	// base simulates a leaked os.Environ() (the pre-fix bug).
	base := mapToSlice(secrets)
	base = append(base, "PATH=/usr/bin", "HOME=/root", "USER=root", "LANG=en_US.UTF-8")

	env := BuildAgentEnv(base, nil, nil)
	got := envSliceToMap(env)

	for k := range secrets {
		if v, ok := got[k]; ok {
			t.Errorf("control-plane var %q survived in agent env: %q", k, v)
		}
	}
	// Runtime-essential vars must survive.
	for _, k := range []string{"PATH", "HOME", "USER", "LANG"} {
		if _, ok := got[k]; !ok {
			t.Errorf("runtime var %q was wrongly stripped from agent env", k)
		}
	}
}

func TestBuildAgentEnv_StripsControlPlaneFromProviderAndSessionEnv(t *testing.T) {
	secrets := controlPlaneSecrets()
	// Provider env legitimately carries data-plane credentials (MUST survive),
	// but a buggy/malicious provider layer smuggles control-plane secrets.
	providerEnv := []string{
		"ANTHROPIC_API_KEY=sk-ant-data-plane-key",
		"ANTHROPIC_BASE_URL=https://api.example.com",
		"CCCODE_MANAGEMENT_TOKEN=smuggled-mgmt-token",
		"OPENCODE_SERVER_PASSWORD=smuggled-oc-pass",
	}
	sessionEnv := []string{
		"CUSTOM_SESSION_FLAG=1",
		"CCCODE_RELAY_CREDENTIAL=smuggled-relay-cred",
		"CLAUDECODE=smuggled-marker",
	}
	base := append(mapToSlice(map[string]string{"PATH": "/usr/bin", "HOME": "/root"}), mapToSlice(secrets)...)

	env := BuildAgentEnv(base, providerEnv, sessionEnv)
	got := envSliceToMap(env)

	// All control-plane secrets gone.
	for k := range secrets {
		if _, ok := got[k]; ok {
			t.Errorf("control-plane var %q from base survived", k)
		}
	}
	if v, ok := got["CCCODE_MANAGEMENT_TOKEN"]; ok {
		t.Errorf("smuggled provider CCCODE_MANAGEMENT_TOKEN survived: %q", v)
	}
	if v, ok := got["OPENCODE_SERVER_PASSWORD"]; ok {
		t.Errorf("smuggled provider OPENCODE_SERVER_PASSWORD survived: %q", v)
	}
	if v, ok := got["CCCODE_RELAY_CREDENTIAL"]; ok {
		t.Errorf("smuggled session CCCODE_RELAY_CREDENTIAL survived: %q", v)
	}
	if v, ok := got["CLAUDECODE"]; ok {
		t.Errorf("smuggled session CLAUDECODE survived: %q", v)
	}

	// Data-plane provider credentials MUST survive (agents need them to auth).
	for _, k := range []string{"ANTHROPIC_API_KEY", "ANTHROPIC_BASE_URL", "CUSTOM_SESSION_FLAG"} {
		if _, ok := got[k]; !ok {
			t.Errorf("legitimate provider/session var %q was wrongly stripped", k)
		}
	}
	if got["ANTHROPIC_API_KEY"] != "sk-ant-data-plane-key" {
		t.Errorf("ANTHROPIC_API_KEY value corrupted: %q", got["ANTHROPIC_API_KEY"])
	}
}

func TestBuildAgentEnv_OverridesAndDedup(t *testing.T) {
	base := []string{"PATH=/usr/bin", "HOME=/old", "FOO=base"}
	providerEnv := []string{"HOME=/new", "BAR=provider"}
	sessionEnv := []string{"FOO=session"}

	env := BuildAgentEnv(base, providerEnv, sessionEnv)
	got := envSliceToMap(env)

	// provider overrides base; session overrides base; no duplicates.
	if got["HOME"] != "/new" {
		t.Errorf("HOME override failed: got %q want /new", got["HOME"])
	}
	if got["FOO"] != "session" {
		t.Errorf("FOO override failed: got %q want session", got["FOO"])
	}
	if got["BAR"] != "provider" {
		t.Errorf("provider BAR dropped: %q", got["BAR"])
	}
	// Ensure no duplicate keys (MergeEnv contract).
	seen := make(map[string]bool)
	for _, e := range env {
		k, _, ok := strings.Cut(e, "=")
		if !ok {
			continue
		}
		if seen[k] {
			t.Errorf("duplicate env key after merge: %q", k)
		}
		seen[k] = true
	}
}

func TestFilterEnvToAllowlist(t *testing.T) {
	env := []string{"PATH=/usr/bin", "HOME=/root", "SECRET=leak", "TOKEN=leak"}
	got := FilterEnvToAllowlist(env, []string{"PATH", "HOME"})
	want := map[string]string{"PATH": "/usr/bin", "HOME": "/root"}
	gotMap := envSliceToMap(got)
	for k, v := range want {
		if gotMap[k] != v {
			t.Errorf("allowlist dropped/changed %q: got %q want %q", k, gotMap[k], v)
		}
	}
	for _, k := range []string{"SECRET", "TOKEN"} {
		if _, ok := gotMap[k]; ok {
			t.Errorf("non-allowlisted var %q survived filter", k)
		}
	}
}

func TestIsControlPlaneEnv(t *testing.T) {
	cases := []struct {
		entry string
		want  bool
	}{
		{"CCCODE_MANAGEMENT_TOKEN=x", true},
		{"CCCODE_RELAY_CREDENTIAL=x", true},
		{"CCCODE_RELAY_ROUTE_ID=x", true},
		{"CCCODE_RELAY_ENDPOINT=x", true},
		{"CCCODE_DEV_INSECURE_WS=x", true},
		{"CCCODE_RELAY_VPS_PASS=x", true},
		{"OPENCODE_SERVER_USERNAME=x", true},
		{"OPENCODE_SERVER_PASSWORD=x", true},
		{"CLAUDECODE=x", true},
		{"ANTHROPIC_API_KEY=sk-ant-x", false}, // data-plane, must survive
		{"PATH=/usr/bin", false},
		{"HOME=/root", false},
		{"malformed-no-equals", false},
	}
	for _, c := range cases {
		if got := isControlPlaneEnv(c.entry); got != c.want {
			t.Errorf("isControlPlaneEnv(%q) = %v want %v", c.entry, got, c.want)
		}
	}
}
