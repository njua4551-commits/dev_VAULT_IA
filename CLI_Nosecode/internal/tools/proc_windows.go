//go:build windows

package tools

import "os/exec"

func prepareCommand(cmd *exec.Cmd) {}

func killCommand(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
