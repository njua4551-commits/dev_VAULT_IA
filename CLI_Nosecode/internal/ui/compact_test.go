package ui

import (
	"fmt"
	"strings"
	"testing"
)

func TestCompactTranscriptKeepsHeadAndTail(t *testing.T) {
	var msgs []Message
	for i := 0; i < 50; i++ {
		msgs = append(msgs, Message{Role: "user",
			Text: fmt.Sprintf("mensaje-%02d %s", i, strings.Repeat("x", 200))})
	}
	got := compactTranscript(msgs, 2000)
	if !strings.Contains(got, "mensaje-00") || !strings.Contains(got, "mensaje-49") {
		t.Fatalf("faltan extremos: %q", got)
	}
	if !strings.Contains(got, compactNote) {
		t.Fatal("falta la nota de omisión")
	}
	if strings.Contains(got, "mensaje-25") {
		t.Fatal("incluyó el medio cuando no cabía")
	}
}

func TestCompactDoneReemplazaConversacion(t *testing.T) {
	m := newTestModel(t)
	m.msgs = []Message{
		{Role: "user", Text: "hola"},
		{Role: "assistant", Text: "mundo"},
	}
	m.compacting = true
	m.usageValid = true
	m.usageTotal = 120000
	updated, _ := m.onCompactDone(compactDoneMsg{summary: "resumen útil", count: 2})
	got, ok := updated.(Model)
	if !ok {
		t.Fatal("tipo inesperado")
	}
	if got.compacting || got.usageValid || got.ctxWarned {
		t.Fatalf("estado = compacting=%v usageValid=%v ctxWarned=%v", got.compacting, got.usageValid, got.ctxWarned)
	}
	if len(got.msgs) != 1 || !strings.Contains(got.msgs[0].Text, compactTag) ||
		!strings.Contains(got.msgs[0].Text, "resumen útil") {
		t.Fatalf("mensajes = %+v", got.msgs)
	}
	history := apiMessages(got.msgs, false)
	if len(history) != 1 || !strings.Contains(history[0].Content.(string), "resumen útil") {
		t.Fatalf("historial = %+v", history)
	}
}

func TestContextLabelUsaUsageReal(t *testing.T) {
	m := newTestModel(t)
	m.contextWin = 100000
	m.usageValid = true
	m.usageTotal = 50000
	if label := m.contextLabel(); !strings.Contains(label, "50000 tok") ||
		!strings.Contains(label, "50%") || strings.Contains(label, "/compact") {
		t.Fatalf("label = %q", label)
	}
	m.usageTotal = 85000
	if label := m.contextLabel(); !strings.Contains(label, "85%") || !strings.Contains(label, "/compact") {
		t.Fatalf("label = %q", label)
	}
}

func TestCompactResumeConModelo(t *testing.T) {
	srv := sseTestServer([]string{"Resumen ", "final"}, 0)
	defer srv.Close()
	m := connected(t, srv.URL, "clave")
	m.msgs = []Message{
		{Role: "user", Text: "hola"},
		{Role: "assistant", Text: "mundo"},
	}
	m.input.SetValue("/compact")
	nm, cmd := m.submit()
	m = nm.(Model)
	if !m.compacting || cmd == nil {
		t.Fatalf("no arrancó: compacting=%v cmd=%v", m.compacting, cmd)
	}
	m = pump(t, m, cmd)
	if m.compacting {
		t.Fatal("sigue compactando")
	}
	if len(m.msgs) != 1 || !strings.Contains(m.msgs[0].Text, compactTag) ||
		!strings.Contains(m.msgs[0].Text, "Resumen final") {
		t.Fatalf("mensajes = %+v", m.msgs)
	}
	if !strings.Contains(m.toast, "resumida") {
		t.Fatalf("toast = %q", m.toast)
	}
}

func TestWarnContextUnaVez(t *testing.T) {
	m := newTestModel(t)
	m.contextWin = 100000
	m.usageValid = true
	m.usageTotal = 81000
	m = m.warnContext()
	if !m.ctxWarned || !strings.Contains(m.toast, "/compact") {
		t.Fatalf("no avisó: %+v", m.toast)
	}
	m.toast = ""
	m = m.warnContext()
	if m.toast != "" {
		t.Fatalf("avisó dos veces: %q", m.toast)
	}
}
