package ui

import (
	"context"
	"strings"
	"testing"
	"time"

	"nosecode/internal/api"
)

func TestToolFailureGuardBloqueaAlSegundoFallo(t *testing.T) {
	var g toolFailureGuard
	if g.fail("srv__tool", "conexion rechazada") {
		t.Fatal("el primer fallo no debe bloquear")
	}
	if !g.fail("srv__tool", "  conexion   rechazada ") {
		t.Fatal("el segundo fallo identico debe bloquear")
	}
	reason, blocked := g.blockedReason("srv__tool")
	if !blocked || reason != "conexion rechazada" {
		t.Fatalf("bloqueada=%v reason=%q", blocked, reason)
	}
}

func TestToolFailureGuardNoBloqueaConOtroError(t *testing.T) {
	var g toolFailureGuard
	g.fail("srv__tool", "uno")
	if g.fail("srv__tool", "dos") {
		t.Fatal("un error distinto no debe bloquear")
	}
	g.success("srv__tool")
	if _, blocked := g.blockedReason("srv__tool"); blocked {
		t.Fatal("success debe limpiar el contador")
	}
}

func TestBeginStepLimitaA12(t *testing.T) {
	var a agentRun
	for i := 0; i < maxAgentSteps; i++ {
		if !a.beginStep() {
			t.Fatalf("paso %d no deberia fallar", i+1)
		}
	}
	if a.beginStep() {
		t.Fatalf("no debe permitir mas de %d pasos", maxAgentSteps)
	}
}

func TestActivityTextMuestraSegundos(t *testing.T) {
	m := Model{activity: activityState{kind: "tool", label: "playwright__browser_navigate", started: time.Now()}}
	text := m.activityText()
	if !strings.Contains(text, "ejecutando playwright__browser_navigate…") || !strings.Contains(text, "0 s") {
		t.Fatalf("activityText = %q", text)
	}
	m.activity = activityState{kind: "model", started: time.Now()}
	if !strings.Contains(m.activityText(), "esperando al modelo…") {
		t.Fatalf("activityText = %q", m.activityText())
	}
}

func TestPruneOldImagesConservaLasDosUltimas(t *testing.T) {
	img := func(url string) []api.ContentPart {
		return []api.ContentPart{
			{Type: "text", Text: "imagen"},
			{Type: "image_url", ImageURL: &api.ImageURL{URL: url}},
		}
	}
	msgs := []api.ChatMessage{
		{Role: "user", Content: img("data:1")},
		{Role: "user", Content: img("data:2")},
		{Role: "user", Content: img("data:3")},
	}
	pruneOldImages(msgs, 2)
	first := msgs[0].Content.([]api.ContentPart)
	if len(first) != 2 || first[1].Type != "text" ||
		!strings.Contains(first[1].Text, "imagen anterior omitida") {
		t.Fatalf("primera imagen no podada: %+v", first)
	}
	for _, i := range []int{1, 2} {
		parts := msgs[i].Content.([]api.ContentPart)
		if parts[1].Type != "image_url" {
			t.Fatalf("mensaje %d perdio su imagen: %+v", i, parts)
		}
	}
}

func TestEscCancelaContextoDelAgente(t *testing.T) {
	m := newTestModel(t)
	ctx, cancel := context.WithCancel(context.Background())
	m.run.agent = &agentRun{ctx: ctx, cancel: cancel}
	m.activity = activityState{kind: "tool", label: "x", started: time.Now()}

	updated, _ := m.cancelBusy()
	got := updated.(Model)
	if got.run.agent != nil {
		t.Fatal("el agente deberia quedar cancelado")
	}
	if got.activity.kind != "" {
		t.Fatalf("la actividad deberia limpiarse: %+v", got.activity)
	}
	select {
	case <-ctx.Done():
	default:
		t.Fatal("el contexto del turno no se cancelo")
	}
}
