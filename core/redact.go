package core

import (
	"regexp"
	"strings"
)

// RedactEnv returns a copy of env with values of sensitive keys masked.
// Only env vars whose key contains a sensitive substring are redacted.
func RedactEnv(env []string) []string {
	sensitiveKeys := []string{
		"KEY", "TOKEN", "SECRET", "PASSWORD", "CREDENTIAL",
	}
	out := make([]string, len(env))
	for i, e := range env {
		idx := strings.IndexByte(e, '=')
		if idx < 0 {
			out[i] = e
			continue
		}
		key := strings.ToUpper(e[:idx])
		redact := false
		for _, s := range sensitiveKeys {
			if strings.Contains(key, s) {
				redact = true
				break
			}
		}
		if redact {
			out[i] = e[:idx+1] + "***"
		} else {
			out[i] = e
		}
	}
	return out
}

// redactStderrPatterns strips likely-secret substrings from agent stderr
// before it enters logs or is wrapped into EventError. We match:
//   - CCCODE_* / OPENCODE_SERVER_* KEY=VALUE leakage
//   - KEY=VALUE where the value looks like a long secret (api keys/tokens)
//   - "Bearer <token>" / "Authorization: ..." headers
//   - long base64 / hex runs that are almost certainly credentials
//
// This is content-level defense; the authoritative fix is that the secrets
// never enter the agent environment in the first place (BuildAgentEnv). The
// redactor exists so a misbehaving agent that prints its own env still can't
// exfiltrate via the bridge's error channel.
var redactStderrPatterns = []*regexp.Regexp{
	// CCCODE_*=... and OPENCODE_SERVER_*=... assignments
	regexp.MustCompile(`(?i)(CCCODE_[A-Z_]*|OPENCODE_SERVER_[A-Z_]*)=[^\s]*`),
	// Generic KEY=secret for api-key/token/secret/password/credential keys
	regexp.MustCompile(`(?i)([A-Z0-9_]*(?:API_KEY|TOKEN|SECRET|PASSWORD|CREDENTIAL|AUTH)[A-Z0-9_]*)=[^\s]+`),
	// Bearer / Authorization headers
	regexp.MustCompile(`(?i)(Bearer\s+)[A-Za-z0-9._\-=+/]+`),
	regexp.MustCompile(`(?i)(Authorization:\s*)[^\r\n]*`),
	// Long base64/hex blobs (>= 32 chars) — covers most opaque tokens
	regexp.MustCompile(`[A-Za-z0-9+/]{32,}={0,2}`),
	// Long hex runs (>= 32)
	regexp.MustCompile(`[0-9a-fA-F]{32,}`),
}

// redactedPlaceholder replaces matched secret substrings.
const redactedPlaceholder = "[REDACTED]"

// RedactStderr returns s with likely-secret substrings removed.
//
// Agent stderr historically flowed verbatim into slog.Error and was wrapped
// into EventError, which can surface on the wire to clients. This keeps only
// non-secret diagnostic content (exit codes, stable error classes, length).
// The output is deterministic for a given input.
//
// The full stderr is NOT retained here; if a raw dump is needed for
// debugging, the caller controls that via CCCODE_DEBUG_STDERR (the bridge
// writes it to a 0600 file instead of routing it through EventError/slog).
func RedactStderr(s string) string {
	out := s
	for _, re := range redactStderrPatterns {
		out = re.ReplaceAllString(out, redactedPlaceholder)
	}
	return out
}

// RedactArgs returns a copy of args with values after sensitive flag names masked.
// Sensitive flags: --api-key, --api_key, --token, --secret, -k, etc.
func RedactArgs(args []string) []string {
	out := make([]string, len(args))
	copy(out, args)

	sensitiveFlags := []string{
		"--api-key", "--api_key", "--apikey",
		"--token", "--secret", "--password",
		"-k",
	}

	for i := 0; i < len(out); i++ {
		arg := strings.ToLower(out[i])

		// --flag=value format
		for _, f := range sensitiveFlags {
			if strings.HasPrefix(arg, f+"=") {
				out[i] = out[i][:strings.Index(out[i], "=")+1] + "***"
				break
			}
		}

		// --flag value format
		for _, f := range sensitiveFlags {
			if arg == f && i+1 < len(out) {
				out[i+1] = "***"
				i++
				break
			}
		}
	}
	return out
}
