package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	bashDefaultTimeout = 60
	bashMaxTimeout     = 600
)

// BashProcess representa un comando que sigue vivo después de un timeout
// suave. El usuario puede seguir esperando o cancelarlo desde la TUI.
type BashProcess struct {
	cmd   *exec.Cmd
	out   *limitedBuffer
	done  chan struct{}
	mu    sync.Mutex
	err   error
	start time.Time
}

// Done se cierra cuando el proceso termina.
func (p *BashProcess) Done() <-chan struct{} { return p.done }

// Output devuelve la salida recibida hasta ahora.
func (p *BashProcess) Output() string {
	if p == nil || p.out == nil {
		return ""
	}
	return bashText(p.out)
}

// Err devuelve el error final del proceso (nil si terminó bien).
func (p *BashProcess) Err() error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.err
}

// ExitCode devuelve el código de salida real.
func (p *BashProcess) ExitCode() int { return exitCodeOf(p.Err()) }

// Started devuelve cuándo arrancó el proceso.
func (p *BashProcess) Started() time.Time {
	if p == nil {
		return time.Time{}
	}
	return p.start
}

// Kill termina el árbol de procesos.
func (p *BashProcess) Kill() {
	if p != nil && p.cmd != nil {
		killCommand(p.cmd)
	}
}

type bashArgs struct {
	Command  string `json:"command"`
	TimeoutS int    `json:"timeout_s"`
}

func parseBashArgs(raw json.RawMessage) (bashArgs, error) {
	var a bashArgs
	if err := decode(raw, &a); err != nil {
		return a, err
	}
	a.Command = strings.TrimSpace(a.Command)
	switch {
	case a.TimeoutS <= 0:
		a.TimeoutS = bashDefaultTimeout
	case a.TimeoutS > bashMaxTimeout:
		a.TimeoutS = bashMaxTimeout
	}
	return a, nil
}

func runBash(ctx context.Context, raw json.RawMessage) Result {
	res := runBashInner(ctx, raw)
	if pw := sudoPassword(ctx); pw != "" {
		res.Text = strings.ReplaceAll(res.Text, pw, "••••")
		res.SudoBad = sudoBadPassword(res.Text)
		if res.SudoBad {
			res.Text += "\nLa contraseña de sudo fue rechazada. El usuario puede intentarlo de nuevo; no pidas la contraseña en el chat."
		}
	}
	return res
}

func runBashInner(ctx context.Context, raw json.RawMessage) Result {
	a, err := parseBashArgs(raw)
	if err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	if a.Command == "" {
		return errResult("el comando está vacío")
	}

	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "powershell.exe"
		if _, err := exec.LookPath(name); err != nil {
			name = "pwsh"
		}
		args = []string{"-NoProfile", "-NonInteractive", "-Command", a.Command}
	} else {
		if bash, err := exec.LookPath("bash"); err == nil {
			name, args = bash, []string{"-lc", a.Command}
		} else if sh, err := exec.LookPath("sh"); err == nil {
			name, args = sh, []string{"-c", a.Command}
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
	cmd.Env = nonInteractiveEnv()
	cmd.Stdin = nil // sin terminal: sudo y los prompts fallan en vez de colgarse
	pw := ""
	if runtime.GOOS != "windows" && UsesSudo(a.Command) {
		if pw = sudoPassword(ctx); pw != "" {
			cmd.Args[len(cmd.Args)-1] = rewriteSudo(a.Command)
			cmd.Stdin = strings.NewReader(sudoStdin(pw))
		}
	}
	var out limitedBuffer
	out.secret = pw
	out.max = 8 << 10
	cmd.Stdout = &out
	cmd.Stderr = &out

	start := time.Now()
	if err := cmd.Start(); err != nil {
		return Result{
			Text:     "Error: " + err.Error(),
			Error:    true,
			ExitCode: exitCodeOf(err),
			TimeoutS: a.TimeoutS,
		}
	}
	proc := &BashProcess{cmd: cmd, out: &out, done: make(chan struct{}), start: start}
	go func() {
		err := cmd.Wait()
		proc.mu.Lock()
		proc.err = err
		proc.mu.Unlock()
		close(proc.done)
	}()

	deadline := time.NewTimer(time.Duration(a.TimeoutS) * time.Second)
	defer deadline.Stop()
	ticker := time.NewTicker(300 * time.Millisecond)
	defer ticker.Stop()

	for {
		select {
		case <-proc.done:
			return finalBashResult(proc, a.TimeoutS)
		case <-ctx.Done():
			proc.Kill()
			<-proc.done
			text := bashText(&out)
			if text != "" {
				text += "\n"
			}
			text += "Error: cancelado por el usuario"
			return Result{
				Text:     text,
				Error:    true,
				ExitCode: 130,
				Bytes:    len(text),
				TimeoutS: a.TimeoutS,
			}
		case <-ticker.C:
			if interactivePrompt(out.String()) {
				proc.Kill()
				<-proc.done
				return interactiveResult(proc, a.TimeoutS)
			}
		case <-deadline.C:
			if interactivePrompt(out.String()) {
				proc.Kill()
				<-proc.done
				return interactiveResult(proc, a.TimeoutS)
			}
			// Timeout suave: no matar; devolver la salida parcial y el proceso.
			text := bashText(&out)
			return Result{
				Text:     text,
				Bytes:    len(text),
				TimeoutS: a.TimeoutS,
				Soft:     true,
				Process:  proc,
			}
		}
	}
}

func finalBashResult(proc *BashProcess, timeoutS int) Result {
	text := bashText(proc.out)
	err := proc.Err()
	bytes := len(text)
	if err != nil {
		if text != "" {
			text += "\n"
		}
		text += "Error: " + err.Error()
		return Result{Text: text, Error: true, ExitCode: exitCodeOf(err), Bytes: bytes, TimeoutS: timeoutS}
	}
	if text == "" {
		text = "(sin salida)"
	}
	return Result{Text: text, Bytes: bytes, TimeoutS: timeoutS}
}

func interactiveResult(proc *BashProcess, timeoutS int) Result {
	text := bashText(proc.out)
	if text != "" {
		text += "\n"
	}
	text += "Error: el comando espera una respuesta interactiva (confirmación o contraseña que el cliente no puede dar). Usa banderas no interactivas (-y, --yes) o pídele al usuario que lo ejecute él."
	return Result{Text: text, Error: true, ExitCode: 1, Bytes: len(text), TimeoutS: timeoutS}
}

// interactivePrompt detecta salidas que indican que el comando espera a una
// persona (sudo, contraseñas o confirmaciones).
func interactivePrompt(out string) bool {
	low := strings.ToLower(out)
	for _, marker := range []string{
		"[sudo] password",
		"sudo: a terminal is required",
		"sudo: a password is required",
		"sudo: no tty present",
		"[y/n]",
		"(y/n)",
		"[s/n]",
		"password:",
		"passphrase",
		"are you sure",
		"get-credential",
		"read-host",
	} {
		if strings.Contains(low, marker) {
			return true
		}
	}
	return false
}

// nonInteractiveEnv añade variables que evitan paginadores y preguntas.
func nonInteractiveEnv() []string {
	return append(os.Environ(),
		"TERM=dumb",
		"PAGER=cat",
		"GIT_PAGER=cat",
		"GIT_TERMINAL_PROMPT=0",
		"DEBIAN_FRONTEND=noninteractive",
		"SUDO_ASKPASS=/bin/false",
		"NO_COLOR=1",
	)
}

func bashText(out *limitedBuffer) string {
	text, truncated := out.Text()
	text = strings.TrimRight(text, "\n")
	if truncated {
		if text != "" {
			text += "\n"
		}
		text += "[salida recortada a 8 KB]"
	}
	return text
}

// exitCodeOf extrae el código de salida real del proceso.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var ee *exec.ExitError
	if ok := asExitError(err, &ee); ok {
		return ee.ExitCode()
	}
	return 1
}

func asExitError(err error, target **exec.ExitError) bool {
	for err != nil {
		if ee, ok := err.(*exec.ExitError); ok {
			*target = ee
			return true
		}
		type unwrapper interface{ Unwrap() error }
		u, ok := err.(unwrapper)
		if !ok {
			return false
		}
		err = u.Unwrap()
	}
	return false
}

type limitedBuffer struct {
	mu        sync.Mutex
	buf       bytes.Buffer
	max       int
	truncated bool
	secret    string // se oculta en todo lo que se lea (contraseña de sudo)
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
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

func (b *limitedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.redact(b.buf.String())
}

func (b *limitedBuffer) redact(s string) string {
	if b.secret == "" {
		return s
	}
	return strings.ReplaceAll(s, b.secret, "••••")
}

// Text devuelve el contenido y si se recortó, con un solo bloqueo.
func (b *limitedBuffer) Text() (string, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.redact(b.buf.String()), b.truncated
}
