package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestUsesSudoYRewrite(t *testing.T) {
	for _, c := range []string{"sudo ls", "echo a && sudo apt update", "x; sudo -n id", "(sudo id)", "sudo"} {
		if !UsesSudo(c) {
			t.Errorf("%q debe detectarse", c)
		}
	}
	for _, c := range []string{"echo pseudo", "ls sudoers.d", "cat sudo.txt", "sudoedit x"} {
		if UsesSudo(c) {
			t.Errorf("%q NO debe detectarse", c)
		}
	}
	if got := rewriteSudo("a && sudo apt update | sudo tee x"); got != "a && sudo -S -p '' apt update | sudo -S -p '' tee x" {
		t.Fatalf("rewrite: %q", got)
	}
}

// fakeSudo imita sudo -S -p ” leyendo la contraseña de stdin.
func fakeSudo(t *testing.T) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("sudo no existe en Windows")
	}
	dir := t.TempDir()
	script := "#!/bin/sh\n" +
		"[ \"$1\" = \"-S\" ] || { echo 'sudo: a password is required' >&2; exit 1; }\n" +
		"shift 3\nread pw\n" +
		"if [ \"$pw\" != \"secreto\" ]; then echo 'Sorry, try again.' >&2; echo 'sudo: 1 incorrect password attempt' >&2; exit 1; fi\n" +
		"echo \"ejecutado como root: $* (pw=$pw)\"\n"
	if err := os.WriteFile(filepath.Join(dir, "sudo"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir+string(os.PathListSeparator)+os.Getenv("PATH"))
}

func bashRun(ctx context.Context, cmd string) Result {
	raw, _ := json.Marshal(map[string]any{"command": cmd, "timeout_s": 10})
	return runBash(ctx, raw)
}

func TestSudoConContrasenaCorrectaYOculta(t *testing.T) {
	fakeSudo(t)
	res := bashRun(WithSudoPassword(context.Background(), "secreto"), "sudo id -u")
	if res.Error || res.SudoBad {
		t.Fatalf("falló: %+v", res)
	}
	if !strings.Contains(res.Text, "ejecutado como root: id -u") {
		t.Fatalf("salida: %q", res.Text)
	}
	if strings.Contains(res.Text, "secreto") || !strings.Contains(res.Text, "••••") {
		t.Fatalf("la contraseña no debe aparecer: %q", res.Text)
	}
}

func TestSudoContrasenaIncorrecta(t *testing.T) {
	fakeSudo(t)
	res := bashRun(WithSudoPassword(context.Background(), "mala"), "sudo id")
	if !res.Error || !res.SudoBad {
		t.Fatalf("debe marcar SudoBad: %+v", res)
	}
	if strings.Contains(res.Text, "mala") && strings.Contains(res.Text, "pw=mala") {
		t.Fatal("fuga de contraseña")
	}
}

func TestSudoSinContrasenaSigueFallandoSinColgarse(t *testing.T) {
	fakeSudo(t)
	res := bashRun(context.Background(), "sudo id")
	if !res.Error {
		t.Fatalf("sin contraseña debe fallar: %+v", res)
	}
}
