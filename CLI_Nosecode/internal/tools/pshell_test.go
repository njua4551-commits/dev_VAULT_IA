package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func psh1(t *testing.T, cmd string, timeout int) Result {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"command": cmd, "timeout": timeout})
	return runBash(context.Background(), raw)
}

func pshSetup(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sin shell persistente en Windows")
	}
	ResetShell()
	psh.mu.Lock()
	psh.lastDir = t.TempDir()
	psh.mu.Unlock()
	SetPersistentShell(true)
	t.Cleanup(func() { ResetShell(); KillJobs() })
}

func TestShellPersisteCdYVariables(t *testing.T) {
	pshSetup(t)
	d := t.TempDir()
	if r := psh1(t, "cd "+d+" && export FOO=bar", 20); r.Error {
		t.Fatal(r.Text)
	}
	r := psh1(t, "echo $FOO; pwd", 20)
	if !strings.Contains(r.Text, "bar") || !strings.Contains(r.Text, d) {
		t.Fatalf("no persistió: %q", r.Text)
	}
}

func TestShellCodigoDeSalida(t *testing.T) {
	pshSetup(t)
	r := psh1(t, "echo hola; false", 20)
	if !r.Error || r.ExitCode != 1 || !strings.Contains(r.Text, "hola") {
		t.Fatalf("%+v", r)
	}
	if r := psh1(t, "echo ok", 20); r.Error {
		t.Fatal(r.Text)
	}
}

func TestShellExitReinicia(t *testing.T) {
	pshSetup(t)
	d := t.TempDir()
	psh1(t, "cd "+d, 20)
	r := psh1(t, "exit 3", 20)
	if !r.Error {
		t.Fatalf("esperaba error: %+v", r)
	}
	r = psh1(t, "pwd", 20)
	if !strings.Contains(r.Text, d) || !strings.Contains(r.Text, "reinició") {
		t.Fatalf("no se reinició con el directorio: %q", r.Text)
	}
}

func TestShellCancelarMataElShell(t *testing.T) {
	pshSetup(t)
	ctx, cancel := context.WithCancel(context.Background())
	raw, _ := json.Marshal(map[string]any{"command": "sleep 30", "timeout": 60})
	go func() { time.Sleep(500 * time.Millisecond); cancel() }()
	runBash(ctx, raw)
	if r := psh1(t, "echo vivo", 20); r.Error || !strings.Contains(r.Text, "vivo") {
		t.Fatalf("%+v", r)
	}
}

func TestShellSegundoPlanoNoBloquea(t *testing.T) {
	pshSetup(t)
	r := psh1(t, "sleep 1; echo fin", 1) // timeout suave
	if r.Process == nil {
		t.Skipf("terminó antes del timeout: %+v", r)
	}
	r2 := psh1(t, "echo nuevo", 20)
	if r2.Error || !strings.Contains(r2.Text, "nuevo") {
		t.Fatalf("%+v", r2)
	}
}

func TestKillJobsMataElShellPersistente(t *testing.T) {
	pshSetup(t)
	psh1(t, "echo x", 20)
	if alive, _ := ShellStatus(); !alive {
		t.Fatal("debería estar vivo")
	}
	KillJobs()
	for i := 0; i < 100 && LiveCount() != 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if LiveCount() != 0 {
		t.Fatalf("quedan %d vivos", LiveCount())
	}
}
