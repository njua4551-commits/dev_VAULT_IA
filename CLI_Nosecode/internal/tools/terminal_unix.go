//go:build !windows

package tools

import (
	"os"
	"os/exec"

	"github.com/creack/pty"
)

const ptySupported = true

func startPTY(cmd *exec.Cmd, cols, rows int) (*os.File, error) {
	return pty.StartWithSize(cmd, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func setPTYSize(f *os.File, cols, rows int) error {
	return pty.Setsize(f, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

// PTYAvailable dice si hay terminales interactivas en este sistema.
func PTYAvailable() bool { return true }
