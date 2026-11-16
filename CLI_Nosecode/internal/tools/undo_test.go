package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func run(t *testing.T, name string, v any) Result {
	t.Helper()
	raw, _ := json.Marshal(v)
	tl, _ := Lookup(name)
	return tl.Run(context.Background(), raw)
}

func TestUndoRedoEditYCrear(t *testing.T) {
	resetUndoForTest()
	d := t.TempDir()
	f := filepath.Join(d, "a.txt")
	os.WriteFile(f, []byte("uno"), 0o644)
	if r := run(t, "edit_file", map[string]string{"path": f, "old": "uno", "new": "dos"}); r.Error {
		t.Fatal(r.Text)
	}
	g := filepath.Join(d, "nuevo.txt")
	if r := run(t, "write_file", map[string]string{"path": g, "content": "hola"}); r.Error {
		t.Fatal(r.Text)
	}
	if len(UndoList()) != 2 {
		t.Fatalf("lista: %v", UndoList())
	}
	if _, err := Undo(false); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(g); !os.IsNotExist(err) {
		t.Fatal("el archivo creado debía borrarse")
	}
	if _, err := Undo(false); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "uno" {
		t.Fatalf("contenido: %q", b)
	}
	if _, err := Undo(false); err == nil {
		t.Fatal("ya no hay nada")
	}
	if _, err := Redo(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "dos" {
		t.Fatalf("rehacer: %q", b)
	}
}

func TestUndoSeNiegaSiCambioAMano(t *testing.T) {
	resetUndoForTest()
	f := filepath.Join(t.TempDir(), "a.txt")
	os.WriteFile(f, []byte("uno"), 0o644)
	run(t, "edit_file", map[string]string{"path": f, "old": "uno", "new": "dos"})
	os.WriteFile(f, []byte("tres a mano"), 0o644)
	if _, err := Undo(false); err == nil {
		t.Fatal("debe negarse")
	}
	if _, err := Undo(true); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(f); string(b) != "uno" {
		t.Fatalf("force: %q", b)
	}
}
