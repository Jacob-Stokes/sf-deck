//go:build !windows

package sf

import (
	"os/exec"
	"syscall"
)

// setProcessGroup puts cmd in its own process group so the whole tree
// (sf → node → children) can be signaled at once.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

// killProcessGroup sends SIGKILL to the command's entire process group.
// The negative PID targets the group leader and every descendant that
// hasn't reparented out of it.
func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pgid := cmd.Process.Pid
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		// Fall back to killing just the leader if the group is already gone.
		return cmd.Process.Kill()
	}
	return nil
}
