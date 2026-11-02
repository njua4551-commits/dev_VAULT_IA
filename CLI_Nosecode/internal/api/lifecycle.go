package api

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"
)

// VaultLine es una linea de salida de `vault.py` (o el final del proceso).
type VaultLine struct {
	Text string
	Done bool
	Err  error // solo con Done: nil si el codigo de salida fue 0
}

// VaultRun ejecuta vault.py en segundo plano mostrando su salida linea a linea.
type VaultRun struct {
	ch     chan VaultLine
	cancel context.CancelFunc
}

// Next bloquea hasta la siguiente linea.
func (v *VaultRun) Next() VaultLine {
	l, ok := <-v.ch
	if !ok {
		return VaultLine{Done: true}
	}
	return l
}

// Cancel termina el proceso (no apaga la IA en Kaggle: para eso esta `down`).
func (v *VaultRun) Cancel() { v.cancel() }

// FindScript busca client/vault.py. Orden: ruta configurada, VAULT_SCRIPT,
// carpeta actual, una carpeta dev-vault al lado y ~/Desktop/IA/dev-vault.
func FindScript(configured string) (string, error) {
	var cands []string
	if configured != "" {
		cands = append(cands, configured)
	}
	if v := strings.TrimSpace(os.Getenv("VAULT_SCRIPT")); v != "" {
		cands = append(cands, v)
	}
	cands = append(cands,
		filepath.Join("client", "vault.py"),
		filepath.Join("dev-vault", "client", "vault.py"),
		filepath.Join("..", "dev-vault", "client", "vault.py"),
	)
	if home, err := os.UserHomeDir(); err == nil {
		cands = append(cands,
			filepath.Join(home, "Desktop", "IA", "dev-vault", "client", "vault.py"),
			filepath.Join(home, "dev-vault", "client", "vault.py"),
		)
	}
	for _, c := range cands {
		if st, err := os.Stat(c); err == nil && !st.IsDir() {
			abs, _ := filepath.Abs(c)
			return abs, nil
		}
	}
	return "", errors.New("no encuentro client/vault.py: define VAULT_SCRIPT con su ruta completa")
}

// FindPython devuelve un interprete de Python disponible.
func FindPython() (string, error) {
	names := []string{"python3", "python"}
	if runtime.GOOS == "windows" {
		names = []string{"python", "py", "python3"}
	}
	for _, n := range names {
		if p, err := exec.LookPath(n); err == nil {
			return p, nil
		}
	}
	return "", errors.New("no encuentro Python en el PATH")
}

// RunVault ejecuta `python vault.py <args...>`. La salida llega por Next().
func RunVault(parent context.Context, python, script string, args ...string) *VaultRun {
	ctx, cancel := context.WithCancel(parent)
	v := &VaultRun{ch: make(chan VaultLine, 128), cancel: cancel}
	go func() {
		defer close(v.ch)
		defer cancel()
		put := func(l VaultLine) {
			select {
			case v.ch <- l:
			case <-ctx.Done():
			}
		}
		cmd := exec.CommandContext(ctx, python, append([]string{"-u", script}, args...)...)
		cmd.Dir = filepath.Dir(filepath.Dir(script))
		cmd.Env = append(os.Environ(), "PYTHONIOENCODING=utf-8", "PYTHONUTF8=1")
		out, err := cmd.StdoutPipe()
		if err != nil {
			v.ch <- VaultLine{Done: true, Err: err}
			return
		}
		cmd.Stderr = cmd.Stdout
		if err := cmd.Start(); err != nil {
			v.ch <- VaultLine{Done: true, Err: err}
			return
		}
		sc := bufio.NewScanner(out)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		for sc.Scan() {
			put(VaultLine{Text: strings.TrimRight(sc.Text(), "\r")})
		}
		werr := cmd.Wait()
		switch {
		case ctx.Err() != nil:
			werr = ErrCanceled
		case werr != nil:
			var ee *exec.ExitError
			if errors.As(werr, &ee) {
				werr = fmt.Errorf("vault.py termino con codigo %d", ee.ExitCode())
			}
		}
		select {
		case v.ch <- VaultLine{Done: true, Err: werr}:
		case <-time.After(2 * time.Second):
		}
	}()
	return v
}
