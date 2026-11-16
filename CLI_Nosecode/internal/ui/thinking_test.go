package ui

import (
	"strings"
	"testing"
)

func TestThinkingSetting(t *testing.T) {
	m := newTestModel(t)
	if m.thinkingSetting() != nil {
		t.Fatal("auto debe ser nil")
	}
	m = m.thinkingSlash("on")
	if v := m.thinkingSetting(); v == nil || !*v {
		t.Fatal("on")
	}
	m = m.thinkingSlash("off")
	if v := m.thinkingSetting(); v == nil || *v {
		t.Fatal("off")
	}
	m = m.thinkingSlash("auto")
	if m.thinkingSetting() != nil {
		t.Fatal("auto")
	}
}

func TestThinkLinesPlegadoYDesplegado(t *testing.T) {
	m := newTestModel(t)
	join := func(ls []uiLine) string {
		var b strings.Builder
		for _, l := range ls {
			b.WriteString(l.text + "\n")
		}
		return b.String()
	}
	folded := join(m.thinkLines(0, "uno dos tres", 80, 60))
	if !strings.Contains(folded, "3 palabras") || strings.Contains(folded, "uno dos") {
		t.Fatalf("plegado: %q", folded)
	}
	m.showThink = true
	open := join(m.thinkLines(0, "uno dos tres", 80, 60))
	if !strings.Contains(open, "uno dos tres") {
		t.Fatalf("desplegado: %q", open)
	}
	if len(m.thinkLines(0, "  ", 80, 60)) != 0 {
		t.Fatal("sin razonamiento no dibuja nada")
	}
}

func TestPensamientoNoSeReenvia(t *testing.T) {
	msgs := []Message{{Role: "user", Text: "hola"}, {Role: "assistant", Text: "respuesta", Think: "SECRETO-INTERNO"}}
	for _, am := range apiMessages(msgs, false) {
		if s, _ := am.Content.(string); strings.Contains(s, "SECRETO-INTERNO") {
			t.Fatal("el razonamiento no debe volver al modelo")
		}
	}
}
