package ui

import (
	"strings"
	"testing"
	"time"

	"nosecode/internal/session"
)

func TestExportMarkdown(t *testing.T) {
	msgs := []Message{
		{Role: "user", Text: "hola"},
		{Role: "assistant", Text: "respuesta", Think: "PENSAMIENTO-INTERNO"},
		{Role: "assistant", Tool: &session.ToolBlock{Name: "edit_file", Args: "a.go", Status: toolOK, Diff: "+1 −1\n- x\n+ y"}},
	}
	md := exportMarkdown(msgs, time.Date(2026, 11, 15, 10, 0, 0, 0, time.UTC))
	for _, want := range []string{"## Tú", "hola", "## Nosecode", "respuesta", "```diff", "+ y"} {
		if !strings.Contains(md, want) {
			t.Fatalf("falta %q:\n%s", want, md)
		}
	}
	if strings.Contains(md, "PENSAMIENTO-INTERNO") {
		t.Fatal("no debe exportar el razonamiento")
	}
}
