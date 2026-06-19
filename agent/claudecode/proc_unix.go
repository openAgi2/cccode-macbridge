//go:build unix

package claudecode

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

// prepareCmdForProcessGroup puts the spawned command in its own process group
// (Setpgid) so Close() can reap the whole tree (the CLI plus any wrapper /
// sudo / plugin subprocesses it forks) with a single negative-PID signal.
// Mirrors agent/codex/proc_unix.go.
func prepareCmdForProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// forceKillProcessGroup sends SIGKILL to the process group led by cmd's PID.
// Safe to call when the process is already gone (ESRCH / ErrProcessDone).
func forceKillProcessGroup(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}

// signalProcessGroup sends sig to the process group led by cmd's PID.
func signalProcessGroup(cmd *exec.Cmd, sig syscall.Signal) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if err := syscall.Kill(-cmd.Process.Pid, sig); err != nil && !errors.Is(err, os.ErrProcessDone) && !errors.Is(err, syscall.ESRCH) {
		return err
	}
	return nil
}
