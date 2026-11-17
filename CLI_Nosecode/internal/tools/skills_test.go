package tools

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSkillsYHooks(t *testing.T) {
	dir := t.TempDir()
	os.MkdirAll(filepath.Join(dir, "demo"), 0o700)
	os.WriteFile(filepath.Join(dir, "demo", "SKILL.md"), []byte("---\nname: demo\ndescription: Guía de prueba\n---\n# Demo\npasos"), 0o600)
	SkillDirs = []string{dir}
	defer func() { SkillDirs = nil }()
	if l := ListSkills(); len(l) != 1 || l[0].Name != "demo" || l[0].Description != "Guía de prueba" {
		t.Fatalf("%+v", l)
	}
	if !strings.Contains(SkillsPrompt(), "demo: Guía de prueba") {
		t.Fatal(SkillsPrompt())
	}
	if r := runSkill(context.Background(), []byte(`{"name":"demo"}`)); r.Error || !strings.Contains(r.Text, "pasos") {
		t.Fatalf("%+v", r)
	}
	if r := runSkill(context.Background(), []byte(`{"name":"nada"}`)); !r.Error || !strings.Contains(r.Text, "demo") {
		t.Fatalf("%+v", r)
	}

	if out := RunHooks([]string{"true"}, "/x"); out != "" {
		t.Fatalf("un hook silencioso no debe añadir texto: %q", out)
	}
	if out := RunHooks([]string{"echo mal {file}; exit 2"}, "a b.go"); !strings.Contains(out, "falló") || !strings.Contains(out, "a b.go") {
		t.Fatalf("%q", out)
	}
}
