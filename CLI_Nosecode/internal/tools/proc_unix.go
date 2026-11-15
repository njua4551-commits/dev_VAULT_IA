//go:build !windows

package tools

import (
	"os/exec"
	"syscall"
)

// prepareCommand arranca el comando en una sesión nueva, sin terminal de
// control: sudo y otros prompts fallan en vez de quedarse esperando.
func prepareCommand(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
}

func killCommand(cmd *exec.Cmd) {
	if cmd.Process == nil {
		return
	}
	_ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	_ = cmd.Process.Kill()
}
