package gobridge

import "fmt"

const runtimeBinaryName = "cccode-bridge-runtime"

// 通过 -ldflags 在 build 时注入：
//
//	-ldflags "-X main.runtimeVersion=$(git describe --tags --always --dirty) -X main.runtimeCommit=$(git rev-parse HEAD) -X main.runtimeDate=$(date -u +%Y-%m-%dT%H:%M:%SZ)"
var (
	runtimeVersion = "0.1.0-dev"
	runtimeCommit  = "unknown"
	runtimeDate    = "unknown"
)

func runtimeVersionString() string {
	return fmt.Sprintf("%s %s (commit: %s, built: %s)", runtimeBinaryName, runtimeVersion, runtimeCommit, runtimeDate)
}
