package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func bash(t *testing.T, cmd string) Result {
	t.Helper()
	raw, _ := json.Marshal(map[string]any{"command": cmd, "timeout_s": 20})
	return runBash(context.Background(), raw)
}

func TestSandboxArgsYPerfil(t *testing.T) {
	a := bwrapArgs("/p", "/h", SandboxConfig{NoNetwork: true, Writable: []string{"/x"}}, []string{"/h/.ssh"}, []string{"/h/.nosecode/config.json"})
	j := strings.Join(a, " ")
	for _, want := range []string{"--ro-bind / /", "--unshare-net", "--tmpfs /h/.ssh", "--ro-bind /dev/null /h/.nosecode/config.json", "--chdir /p --"} {
		if !strings.Contains(j, want) {
			t.Fatalf("falta %q en %s", want, j)
		}
	}
	p := seatbeltProfile("/p", "/h", SandboxConfig{NoNetwork: true}, []string{"/h/.ssh"}, nil)
	for _, want := range []string{"(deny file-write*)", `(subpath "/p")`, `(deny file-read* (subpath "/h/.ssh"))`, "(deny network*)"} {
		if !strings.Contains(p, want) {
			t.Fatalf("perfil sin %q:\n%s", want, p)
		}
	}
}

func TestSandboxRealConBubblewrap(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip()
	}
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("sin bwrap")
	}
	proj, outside, home := t.TempDir(), t.TempDir(), t.TempDir()
	t.Setenv("HOME", home)
	os.MkdirAll(filepath.Join(home, ".ssh"), 0o700)
	os.WriteFile(filepath.Join(home, ".ssh", "id_x"), []byte("SECRETO"), 0o600)
	old, _ := os.Getwd()
	os.Chdir(proj)
	defer os.Chdir(old)
	defer SetSandbox(SandboxConfig{})
	ResetSandboxProbe()

	SetSandbox(SandboxConfig{Mode: "on", NoNetwork: true})
	if mode, active, note := SandboxState(); mode != "on" || !active {
		t.Skipf("bwrap no puede aislar en este entorno: %s", note)
	}
	if r := bash(t, "echo ok > dentro.txt && cat dentro.txt"); r.Error || !strings.Contains(r.Text, "ok") {
		t.Fatalf("debe poder escribir en el proyecto: %+v", r)
	}
	if r := bash(t, "echo mal > "+filepath.Join(outside, "fuera.txt")); !r.Error {
		t.Fatalf("no debe poder escribir fuera del proyecto: %+v", r)
	}
	if _, err := os.Stat(filepath.Join(outside, "fuera.txt")); err == nil {
		t.Fatal("el archivo de fuera no debía crearse")
	}
	if r := bash(t, "cat "+filepath.Join(home, ".ssh", "id_x")+" 2>&1; true"); strings.Contains(r.Text, "SECRETO") {
		t.Fatalf("los secretos deben estar ocultos: %s", r.Text)
	}
	if r := bash(t, "cat /proc/net/dev | grep -c ':'"); strings.TrimSpace(r.Text) != "1" { // solo «lo»
		t.Fatalf("sin red debe verse solo la interfaz lo: %q", r.Text)
	}
	if r := bash(t, "sudo true"); !r.Error || !strings.Contains(r.Text, "sudo no funciona dentro del sandbox") {
		t.Fatalf("%+v", r)
	}

	// modo on sin herramienta → se niega; auto → ejecuta sin aislar
	sbMu.Lock()
	sbProbe.done, sbProbe.ok, sbProbe.why = true, false, "simulado"
	sbMu.Unlock()
	if r := bash(t, "echo x"); !r.Error || !strings.Contains(r.Text, "modo «on»") {
		t.Fatalf("on sin herramienta debe negarse: %+v", r)
	}
	SetSandbox(SandboxConfig{Mode: "auto"})
	if r := bash(t, "echo x"); r.Error || !strings.Contains(r.Text, "x") {
		t.Fatalf("auto sin herramienta ejecuta normal: %+v", r)
	}
	ResetSandboxProbe()
}
