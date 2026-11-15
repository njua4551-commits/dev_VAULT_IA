//go:build !windows

package tools

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestBashTimeoutSuaveDevuelveProceso(t *testing.T) {
	start := time.Now()
	res := runBash(context.Background(), args(map[string]any{
		"command":   "sleep 5",
		"timeout_s": 1,
	}))
	if !res.Soft || res.Process == nil {
		t.Fatalf("esperaba timeout suave con proceso vivo: %+v", res)
	}
	if time.Since(start) > 3*time.Second {
		t.Fatalf("el timeout suave tardó demasiado: %s", time.Since(start))
	}
	res.Process.Kill()
	select {
	case <-res.Process.Done():
	case <-time.After(3 * time.Second):
		t.Fatal("el proceso no terminó tras Kill")
	}
}

func TestBashDetectaPromptYNoCuelga(t *testing.T) {
	start := time.Now()
	res := runBash(context.Background(), args(map[string]any{
		"command":   "echo '[sudo] password for test:'; sleep 30",
		"timeout_s": 30,
	}))
	if !res.Error || !strings.Contains(res.Text, "necesita contraseña") {
		t.Fatalf("resultado = %q", res.Text)
	}
	if time.Since(start) > 5*time.Second {
		t.Fatalf("no cortó el prompt a tiempo: %s", time.Since(start))
	}
}

func TestBashTimeoutMaximoSeRecorta(t *testing.T) {
	res := runBash(context.Background(), args(map[string]any{
		"command":   "true",
		"timeout_s": 9999,
	}))
	if res.TimeoutS != bashMaxTimeout {
		t.Fatalf("timeout_s = %d, quería %d", res.TimeoutS, bashMaxTimeout)
	}
	res = runBash(context.Background(), args(map[string]any{"command": "true"}))
	if res.TimeoutS != bashDefaultTimeout {
		t.Fatalf("timeout por defecto = %d, quería %d", res.TimeoutS, bashDefaultTimeout)
	}
}

func TestBashExitCodeYBytes(t *testing.T) {
	res := runBash(context.Background(), args(map[string]any{"command": "echo hola; exit 3"}))
	if !res.Error || res.ExitCode != 3 {
		t.Fatalf("exit=%d err=%v text=%q", res.ExitCode, res.Error, res.Text)
	}
	if res.Bytes == 0 || !strings.Contains(res.Text, "hola") {
		t.Fatalf("bytes=%d text=%q", res.Bytes, res.Text)
	}
}
