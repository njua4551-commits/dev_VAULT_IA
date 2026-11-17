package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func patchArgs(p string) json.RawMessage {
	b, _ := json.Marshal(map[string]string{"patch": p})
	return b
}

func TestApplyPatchMultiarchivoYNuevo(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("uno\ndos\ntres\ncuatro\n"), 0o644)
	os.WriteFile(b, []byte("x\ny\n"), 0o644)
	n := filepath.Join(dir, "nuevo.txt")
	patch := "--- " + a + "\n+++ " + a + "\n@@ -1,4 +1,4 @@\n uno\n-dos\n+DOS\n tres\n cuatro\n" +
		"--- " + b + "\n+++ " + b + "\n@@ -9,2 +9,3 @@\n x\n y\n+z\n" + // número de línea desplazado a propósito
		"--- /dev/null\n+++ " + n + "\n@@ -0,0 +1,2 @@\n+hola\n+mundo\n"
	r := runApplyPatch(context.Background(), patchArgs(patch))
	if r.Error || len(r.Edited) != 3 {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(a); string(got) != "uno\nDOS\ntres\ncuatro\n" {
		t.Fatalf("a: %q", got)
	}
	if got, _ := os.ReadFile(b); string(got) != "x\ny\nz\n" {
		t.Fatalf("b: %q", got)
	}
	if got, _ := os.ReadFile(n); string(got) != "hola\nmundo\n" {
		t.Fatalf("nuevo: %q", got)
	}
	if _, err := Undo(false); err != nil {
		t.Fatal(err)
	}
}

func TestApplyPatchTodoONadaYRepoMap(t *testing.T) {
	dir := t.TempDir()
	a, b := filepath.Join(dir, "a.txt"), filepath.Join(dir, "b.txt")
	os.WriteFile(a, []byte("uno\n"), 0o644)
	os.WriteFile(b, []byte("x\n"), 0o644)
	bad := "--- " + a + "\n+++ " + a + "\n@@ -1 +1 @@\n-uno\n+UNO\n" +
		"--- " + b + "\n+++ " + b + "\n@@ -1 +1 @@\n-no-existe\n+y\n"
	r := runApplyPatch(context.Background(), patchArgs(bad))
	if !r.Error || !strings.Contains(r.Text, "No se cambió ningún archivo") {
		t.Fatalf("%+v", r)
	}
	if got, _ := os.ReadFile(a); string(got) != "uno\n" {
		t.Fatal("el primer archivo no debía cambiar si el segundo falla")
	}
	os.WriteFile(filepath.Join(dir, "main.go"), []byte("package x\nfunc Hola() {}\ntype T struct{}\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, "node_modules", "p"), 0o755)
	os.WriteFile(filepath.Join(dir, "node_modules", "p", "i.js"), []byte("function no(){}"), 0o644)
	m := RepoMap(dir, 4000)
	if !strings.Contains(m, "main.go: Hola, T") || strings.Contains(m, "node_modules") {
		t.Fatal(m)
	}
}
