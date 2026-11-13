package tools

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCheckReadPathNormal(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nota.txt")
	if err := os.WriteFile(path, []byte("hola"), 0o644); err != nil {
		t.Fatal(err)
	}
	check := CheckReadPath("nota.txt", dir)
	if check.Sensitive || check.Outside {
		t.Fatalf("ruta normal marcada: %+v", check)
	}
	if check.Path != filepath.Clean(path) {
		t.Fatalf("path = %q, quiero %q", check.Path, path)
	}
	if got := EvaluateRead(check, "ask"); got != ReadAllow {
		t.Fatalf("decisión = %q", got)
	}
}

func TestCheckReadPathDotDotFueraDelProyecto(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "proyecto", "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	check := CheckReadPath(filepath.Join("..", "..", "fuera.txt"), sub)
	if !check.Outside || check.Sensitive {
		t.Fatalf(".. no quedó fuera: %+v", check)
	}
	if check.Path != filepath.Join(root, "fuera.txt") {
		t.Fatalf("path = %q", check.Path)
	}
	if got := EvaluateRead(check, "ask"); got != ReadAsk {
		t.Fatalf("ask = %q", got)
	}
	if got := EvaluateRead(check, "allow"); got != ReadAllow {
		t.Fatalf("allow = %q", got)
	}
	if got := EvaluateRead(check, "deny"); got != ReadDeny {
		t.Fatalf("deny = %q", got)
	}
}

func TestCheckReadPathSymlinkASSH(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	key := filepath.Join(ssh, "id_ed25519")
	if err := os.WriteFile(key, []byte("privada"), 0o600); err != nil {
		t.Fatal(err)
	}
	project := t.TempDir()
	link := filepath.Join(project, "claves")
	if err := os.Symlink(ssh, link); err != nil {
		t.Skipf("symlink no disponible: %v", err)
	}
	check := CheckReadPath(link, project)
	if !check.Sensitive {
		t.Fatalf("el symlink a ~/.ssh no se marcó sensible: %+v", check)
	}
	if check.Path != filepath.Clean(ssh) {
		t.Fatalf("path canónico = %q, quiero %q", check.Path, ssh)
	}
	if got := EvaluateRead(check, "allow"); got != ReadAsk {
		t.Fatalf("las rutas sensibles deben pedir permiso, got %q", got)
	}
	if got := EvaluateRead(check, "deny"); got != ReadDeny {
		t.Fatalf("deny = %q", got)
	}
}

func TestCheckReadPathEnvYSensible(t *testing.T) {
	dir := t.TempDir()
	env := filepath.Join(dir, "sub", ".env.local")
	if err := os.MkdirAll(filepath.Dir(env), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(env, []byte("TOKEN=secreto"), 0o600); err != nil {
		t.Fatal(err)
	}
	check := CheckReadPath(".env.local", filepath.Join(dir, "sub"))
	if !check.Sensitive || check.Outside {
		t.Fatalf(".env.local no se marcó sensible: %+v", check)
	}
	for _, name := range []string{".env", ".env.production", "server.pem", "server.key"} {
		if ok, _ := SensitiveName(name); !ok {
			t.Fatalf("%s debería ser sensible", name)
		}
	}
	for _, name := range []string{"note.txt", "main.go", "env.example"} {
		if ok, _ := SensitiveName(name); ok {
			t.Fatalf("%s no debería ser sensible", name)
		}
	}
}

func TestReadDeniedMessageExplica(t *testing.T) {
	check := CheckReadPath(filepath.Join("..", "secreto.txt"), t.TempDir())
	msg := ReadDeniedMessage(check)
	if !strings.Contains(msg, "lectura bloqueada") || !strings.Contains(msg, check.Path) {
		t.Fatalf("mensaje = %q", msg)
	}
}
