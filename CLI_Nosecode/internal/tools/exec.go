package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const bashTimeout = 60 * time.Second

func runBash(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Command string `json:"command"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	command := strings.TrimSpace(a.Command)
	if command == "" {
		return errResult("el comando está vacío")
	}

	ctx, cancel := context.WithTimeout(ctx, bashTimeout)
	defer cancel()

	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "powershell.exe"
		if _, err := exec.LookPath(name); err != nil {
			name = "pwsh"
		}
		args = []string{"-NoProfile", "-NonInteractive", "-Command", command}
	} else {
		if bash, err := exec.LookPath("bash"); err == nil {
			name, args = bash, []string{"-lc", command}
		} else if sh, err := exec.LookPath("sh"); err == nil {
			name, args = sh, []string{"-c", command}
		} else {
			return errResult("no encontré bash ni sh en el PATH")
		}
	}

	cmd := exec.CommandContext(ctx, name, args...)
	prepareCommand(cmd)
	cmd.Cancel = func() error {
		killCommand(cmd)
		return nil
	}
	cmd.WaitDelay = 2 * time.Second
	var out limitedBuffer
	out.max = 8 << 10
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	text := strings.TrimRight(out.String(), "\n")
	if out.truncated {
		if text != "" {
			text += "\n"
		}
		text += "[salida recortada a 8 KB]"
	}
	if ctx.Err() == context.DeadlineExceeded {
		if text != "" {
			text += "\n"
		}
		text += "Error: el comando superó el timeout de 60 s"
		return Result{Text: text, Error: true}
	}
	if err != nil {
		if text != "" {
			text += "\n"
		}
		text += "Error: " + err.Error()
		return Result{Text: text, Error: true}
	}
	if text == "" {
		text = "(sin salida)"
	}
	return Result{Text: text}
}

type limitedBuffer struct {
	buf       bytes.Buffer
	max       int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if b.max > 0 && b.buf.Len() >= b.max {
		b.truncated = true
		return len(p), nil
	}
	room := len(p)
	if b.max > 0 && b.buf.Len()+room > b.max {
		room = b.max - b.buf.Len()
		b.truncated = true
	}
	_, _ = b.buf.Write(p[:room])
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buf.String() }
