package tools

import (
	"strings"
	"testing"
)

func TestMiniDiff(t *testing.T) {
	old := "a\nb\nc\nd\ne\nf\n"
	new := "a\nb\nX\nY\nd\ne\nf\n"
	d := MiniDiff(old, new)
	for _, want := range []string{"+2 −1", "@@ línea 3 @@", "- c", "+ X", "+ Y", "  b", "  d"} {
		if !strings.Contains(d, want) {
			t.Errorf("falta %q en:\n%s", want, d)
		}
	}
	if strings.Contains(d, "  a\n") && strings.Count(d, "\n") > 9 {
		t.Errorf("demasiado contexto:\n%s", d)
	}
	if MiniDiff("x", "x") != "" {
		t.Fatal("sin cambios no hay diff")
	}
	if d := MiniDiff("", "uno\ndos\n"); !strings.Contains(d, "+2 −0") || !strings.Contains(d, "+ uno") {
		t.Fatalf("archivo nuevo: %s", d)
	}
}
