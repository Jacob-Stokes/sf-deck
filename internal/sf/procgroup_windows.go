//go:build windows

package sf

import "os/exec"

// Windows has no POSIX process groups; rely on the default
// CommandContext kill plus cmd.WaitDelay to reclaim a hung child.
func setProcessGroup(cmd *exec.Cmd) {}

func killProcessGroup(cmd *exec.Cmd) error {
	if cmd.Process == nil {
		return nil
	}
	return cmd.Process.Kill()
}
