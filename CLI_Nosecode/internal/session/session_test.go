package session

import (
	"os"
	"strings"
	"testing"
)

func TestSaveLoadListDelete(t *testing.T) {
	t.Setenv("NOSECODE_HOME", t.TempDir())

	s := New()
	s.Title = "Prueba"
	s.Messages = []Message{{Role: "user", Text: "hola"}}
	s.Updated = s.Updated.Add(1)
	if err := s.Save(); err != nil {
		t.Fatalf("Save: %v", err)
	}

	list, err := List()
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list) != 1 || list[0].ID != s.ID || list[0].Title != "Prueba" {
		t.Fatalf("List inesperada: %+v", list)
	}

	got, err := Load(s.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(got.Messages) != 1 || got.Messages[0].Text != "hola" {
		t.Fatalf("mensajes inesperados: %+v", got.Messages)
	}

	if err := Delete(s.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := os.Stat(Dir() + "/" + s.ID + ".json"); !os.IsNotExist(err) {
		t.Fatal("el archivo de sesion sigue en disco")
	}
}

func TestTitleFrom(t *testing.T) {
	if got := TitleFrom("  hola\nmundo  "); got != "hola" {
		t.Fatalf("TitleFrom = %q", got)
	}
	if got := TitleFrom("   "); got != "Nueva sesión" {
		t.Fatalf("TitleFrom vacio = %q", got)
	}
	long := TitleFrom(strings.Repeat("a", 60))
	if len([]rune(long)) > 32 {
		t.Fatalf("titulo demasiado largo: %q", long)
	}
}

func TestRejectsBadIDs(t *testing.T) {
	t.Setenv("NOSECODE_HOME", t.TempDir())
	if err := Delete("../config"); err == nil {
		t.Fatal("Delete acepto un id invalido")
	}
	bad := New()
	bad.ID = "abc"
	if err := bad.Save(); err == nil {
		t.Fatal("Save acepto un id invalido")
	}
}
