//go:build windows

package claudecode

import (
	"os"
	"os/exec"
	"syscall"
)

// prepareCmdForProcessGroup puts the spawned command in its own process group
// on Windows so Close() can reap it. Mirrors agent/codex/proc_windows.go.
func prepareCmdForProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.CreationFlags |= syscall.CREATE_NEW_PROCESS_GROUP
}

// forceKillProcessGroup kills the process tree on Windows.
func forceKillProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

// signalProcessGroup: Windows has no SIGTERM analogue for a process group;
// fall back to Kill (callers only use this as escalation past graceful stop).
func signalProcessGroup(cmd *exec.Cmd, _ syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}

func isProcessRunning(pid int) bool {
	if pid <= 0 {
		return false
	}
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	_ = proc.Release()
	return true
}

