package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExpandMentions(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.go"), []byte("package a\n"), 0o600)
	os.WriteFile(filepath.Join(dir, "bin.dat"), []byte{0, 1, 2}, 0o600)
	os.MkdirAll(filepath.Join(dir, "sub"), 0o700)
	os.WriteFile(filepath.Join(dir, "sub", "x.txt"), nil, 0o600)
	out := expandMentions("revisa @a.go, @sub y @bin.dat o @no-existe", dir)
	if !strings.Contains(out, "[archivo @a.go]") || !strings.Contains(out, "package a") {
		t.Fatal(out)
	}
	if !strings.Contains(out, "[carpeta @sub]") || !strings.Contains(out, "x.txt") {
		t.Fatal(out)
	}
	if strings.Contains(out, "bin.dat]") || strings.Contains(out, "no-existe]") {
		t.Fatalf("no debe incluir binarios ni inexistentes: %s", out)
	}
	if got := expandMentions("correo a@b.com", dir); got != "correo a@b.com" {
		t.Fatalf("un email no es mención: %q", got)
	}
}

func TestMentionizePaths(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "main.go")
	os.WriteFile(f, []byte("x"), 0o600)
	if got := mentionizePaths(f+" explica esto", dir); got != "@main.go explica esto" {
		t.Fatalf("%q", got)
	}
	if got := mentionizePaths("usa /usr/noexiste/x ok", dir); got != "usa /usr/noexiste/x ok" {
		t.Fatalf("%q", got)
	}
}

func TestDetectChecksYComandosPropios(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x"), 0o600)
	if c := detectChecks(dir); len(c) != 3 || c[0] != "go build ./..." {
		t.Fatal(c)
	}
	d2 := t.TempDir()
	os.WriteFile(filepath.Join(d2, "package.json"), []byte(`{"scripts":{"test":"x","build":"y"}}`), 0o600)
	if c := detectChecks(d2); len(c) != 2 || c[0] != "npm run build" {
		t.Fatal(c)
	}

	cmdDir := filepath.Join(dir, ".nosecode", "commands")
	os.MkdirAll(cmdDir, 0o700)
	os.WriteFile(filepath.Join(cmdDir, "revisar.md"), []byte("---\ndescription: Revisa un archivo\n---\nRevisa $1 buscando bugs. $ARGUMENTS"), 0o600)
	list := loadCustomCommands(dir, t.TempDir())
	if len(list) != 1 || list[0].desc != "Revisa un archivo" {
		t.Fatalf("%+v", list)
	}
	if got := list[0].expand("main.go ahora"); got != "Revisa main.go buscando bugs. main.go ahora" {
		t.Fatalf("%q", got)
	}
}
