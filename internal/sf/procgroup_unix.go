package sf

import (
	"os/exec"
	"syscall"
)

func setProcessGroup(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	pgid := cmd.Process.Pid
	if err := syscall.Kill(-pgid, syscall.SIGKILL); err != nil {
		return cmd.Process.Kill()
	}
	return nil
}
