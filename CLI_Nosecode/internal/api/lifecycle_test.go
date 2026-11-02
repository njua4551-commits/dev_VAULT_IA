package api

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func fakeVault(t *testing.T, body string) (python, script string) {
	t.Helper()
	py, err := FindPython()
	if err != nil {
		t.Skip("no hay Python para probar RunVault")
	}
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "client"), 0o755); err != nil {
		t.Fatal(err)
	}
	script = filepath.Join(dir, "client", "vault.py")
	if err := os.WriteFile(script, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return py, script
}

func drain(v *VaultRun) ([]string, VaultLine) {
	var lines []string
	for {
		l := v.Next()
		if l.Done {
			return lines, l
		}
		lines = append(lines, l.Text)
	}
}

func TestRunVaultLineaALinea(t *testing.T) {
	py, script := fakeVault(t, "import sys\nprint('fase uno', flush=True)\nprint('args', ' '.join(sys.argv[1:]))\n")
	lines, last := drain(RunVault(context.Background(), py, script, "up", "--no-wait"))
	if last.Err != nil || len(lines) != 2 || lines[0] != "fase uno" || lines[1] != "args up --no-wait" {
		t.Fatalf("lines=%v last=%+v", lines, last)
	}
}

func TestRunVaultCodigoDeSalida(t *testing.T) {
	py, script := fakeVault(t, "import sys\nprint('ERROR: algo fallo', file=sys.stderr)\nsys.exit(3)\n")
	lines, last := drain(RunVault(context.Background(), py, script, "down"))
	if last.Err == nil || !strings.Contains(last.Err.Error(), "codigo 3") || len(lines) != 1 || !strings.Contains(lines[0], "algo fallo") {
		t.Fatalf("lines=%v last=%+v", lines, last)
	}
}

func TestRunVaultCancelar(t *testing.T) {
	py, script := fakeVault(t, "import time\nprint('empieza', flush=True)\ntime.sleep(30)\n")
	v := RunVault(context.Background(), py, script, "up")
	if l := v.Next(); l.Text != "empieza" {
		t.Fatalf("primera linea = %+v", l)
	}
	start := time.Now()
	v.Cancel()
	_, last := drain(v)
	if last.Err != ErrCanceled || time.Since(start) > 5*time.Second {
		t.Fatalf("last=%+v tardo=%v", last, time.Since(start))
	}
}

func TestFindScript(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "vault.py")
	_ = os.WriteFile(p, []byte("x"), 0o644)
	got, err := FindScript(p)
	if err != nil || got != p {
		t.Fatalf("got=%q err=%v", got, err)
	}
	t.Setenv("VAULT_SCRIPT", p)
	if got, err := FindScript(""); err != nil || got != p {
		t.Fatalf("env: got=%q err=%v", got, err)
	}
}
