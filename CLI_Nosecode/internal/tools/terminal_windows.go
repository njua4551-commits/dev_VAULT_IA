//go:build windows

package tools

import (
	"errors"
	"os"
	"os/exec"
)

// En Windows la herramienta terminal no se ofrece (haría falta ConPTY).
const ptySupported = false

func startPTY(*exec.Cmd, int, int) (*os.File, error) {
	return nil, errors.New("terminal no está disponible en Windows")
}

func setPTYSize(*os.File, int, int) error { return nil }

// PTYAvailable dice si hay terminales interactivas en este sistema.
func PTYAvailable() bool { return false }
