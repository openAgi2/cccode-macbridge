package core

import (
	"strings"
	"testing"
)

func TestRedactStderr_RemovesControlPlaneAndSecrets(t *testing.T) {
	cases := []struct {
		name        string
		input       string
		mustNotContain []string
	}{
		{
			name:  "CCCODE management token",
			input: "error: config load failed CCCODE_MANAGEMENT_TOKEN=secret-mgmt-tok-123",
			mustNotContain: []string{"secret-mgmt-tok-123", "CCCODE_MANAGEMENT_TOKEN=secret-mgmt-tok-123"},
		},
		{
			name:  "CCCODE relay credential",
			input: "debug: relay CCCODE_RELAY_CREDENTIAL=opaque-relay-cred-456",
			mustNotContain: []string{"opaque-relay-cred-456"},
		},
		{
			name:  "Bearer token",
			input: "request header: Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.payload.sig",
			mustNotContain: []string{"eyJhbGciOiJIUzI1NiJ9.payload.sig", "Bearer eyJhbGciOiJIUzI1NiJ9"},
		},
		{
			name:  "API key assignment",
			input: "using ANTHROPIC_API_KEY=sk-ant-api03-longsecretvalue-here",
			mustNotContain: []string{"sk-ant-api03-longsecretvalue-here"},
		},
		{
			name:  "long base64 blob",
			input: "token=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA==",
			mustNotContain: []string{"AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"},
		},
		{
			name:  "long hex run",
			input: "digest=0123456789abcdef0123456789abcdef0123456789abcdef",
			mustNotContain: []string{"0123456789abcdef0123456789abcdef0123456789abcdef"},
		},
		{
			name:  "OPENCODE server password",
			input: "auth: OPENCODE_SERVER_PASSWORD=hunter2",
			mustNotContain: []string{"hunter2"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out := RedactStderr(c.input)
			for _, secret := range c.mustNotContain {
				if strings.Contains(out, secret) {
					t.Errorf("RedactStderr leaked secret %q in output: %q", secret, out)
				}
			}
		})
	}
}

func TestRedactStderr_PreservesDiagnostics(t *testing.T) {
	// Stable diagnostic content (exit code, short stable strings, length
	// markers) must survive so error classification still works.
	input := "Error: command exited with code 127\n  caused by: ENOENT"
	out := RedactStderr(input)
	for _, want := range []string{"exited with code 127", "ENOENT"} {
		if !strings.Contains(out, want) {
			t.Errorf("RedactStderr dropped diagnostic %q: got %q", want, out)
		}
	}
}

func TestRedactStderr_Deterministic(t *testing.T) {
	// Same input must produce same output every time (stable for assertions).
	input := "CCCODE_MANAGEMENT_TOKEN=abc CCODE_RELAY_CREDENTIAL=def Bearer ghi"
	first := RedactStderr(input)
	second := RedactStderr(input)
	if first != second {
		t.Errorf("RedactStderr not deterministic:\nfirst:  %q\nsecond: %q", first, second)
	}
}

func TestRedactStderr_LongBase64DoesNotEatShortText(t *testing.T) {
	// Short non-secret words (under 32 chars) must not be redacted.
	input := "hello world short error message"
	out := RedactStderr(input)
	if out != input {
		t.Errorf("RedactStderr altered non-secret text: got %q want %q", out, input)
	}
}

func TestRedactStderr_EmptyAndNoMatch(t *testing.T) {
	if out := RedactStderr(""); out != "" {
		t.Errorf("RedactStderr(empty) = %q want empty", out)
	}
	plain := "no secrets here at all"
	if out := RedactStderr(plain); out != plain {
		t.Errorf("RedactStderr altered plain text: got %q want %q", out, plain)
	}
}
