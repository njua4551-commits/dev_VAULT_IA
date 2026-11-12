package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/config"
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

func TestBeginStepRespetaLimite(t *testing.T) {
	var a agentRun
	const limit = 5
	for i := 0; i < limit; i++ {
		if !a.beginStep(limit) {
			t.Fatalf("paso %d no deberia fallar", i+1)
		}
	}
	if a.beginStep(limit) {
		t.Fatalf("no debe permitir mas de %d pasos", limit)
	}
	if a.beginStep(0) {
		t.Fatal("un limite no valido no debe permitir pasos")
	}
}

func TestSkipPendingToolsRellenaResultados(t *testing.T) {
	var a agentRun
	a.pending = []api.ToolCall{
		{ID: "c1"},
		{ID: ""},
	}
	a.pendingIdx = 1
	m := Model{cfg: config.Defaults()}
	m.run.agent = &a
	m = m.skipPendingTools()
	if len(a.pending) != 0 || a.pendingIdx != 0 {
		t.Fatalf("pending = %d idx = %d", len(a.pending), a.pendingIdx)
	}
	last := a.msgs[len(a.msgs)-1]
	if last.Role != "tool" || last.ToolCallID == "" {
		t.Fatalf("resultado sintetico = %+v", last)
	}
}

func TestWrapUpPausaYContinua(t *testing.T) {
	srv := sseTestServer([]string{"Resumen ", "final"}, 0)
	defer srv.Close()
	m := connected(t, srv.URL, "clave")
	m.cfg.Tools.MaxSteps = 30
	m.msgs = []Message{{Role: "user", Text: "investiga"}}
	m.run.agent = &agentRun{
		msgs: []api.ChatMessage{
			{Role: "system", Content: "sistema"},
			{Role: "user", Content: "investiga"},
			{Role: "assistant", Content: "resultado parcial"},
		},
		steps: 30,
	}
	nm, cmd := m.beginWrapUp(30)
	m = nm
	if a := m.run.agent; a == nil || !a.wrapup || a.stream == nil {
		t.Fatalf("wrapup no arranco: %+v", m.run.agent)
	}
	m = pump(t, m, cmd)
	a := m.run.agent
	if a == nil || !a.paused || a.wrapup {
		t.Fatalf("agente no pausado: %+v", a)
	}
	if last := a.msgs[len(a.msgs)-1]; last.Role != "assistant" ||
		!strings.Contains(last.Content.(string), "Resumen final") {
		t.Fatalf("resumen no guardado: %+v", last)
	}
	if !strings.Contains(m.toast, "Límite de 30 pasos") || !strings.Contains(m.toast, "continúa") {
		t.Fatalf("toast = %q", m.toast)
	}

	// «continúa» debe reutilizar el mismo agente y su historial.
	m.input.SetValue("continúa")
	updated, next := m.submit()
	m = updated.(Model)
	if m.run.agent != a {
		t.Fatal("continúa creó un agente nuevo en vez de reanudar")
	}
	m = pump(t, m, next)
	if m.run.agent != nil {
		t.Fatal("el turno reanudado debia terminar")
	}
}

func TestAgentePausaAlLimiteYResume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer clave" {
			w.WriteHeader(401)
			return
		}
		var body struct {
			Tools []json.RawMessage `json:"tools"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		if len(body.Tools) == 0 {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]string{"content": "Resumen tras el límite"}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		} else {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{
				"delta": map[string]any{"tool_calls": []any{map[string]any{
					"index": 0, "id": "c1", "type": "function",
					"function": map[string]string{"name": "bash", "arguments": `{"command":"echo ok"}`},
				}}}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	m := connected(t, srv.URL, "clave")
	m.cfg.Tools.MaxSteps = 5
	m.allowTools = map[string]bool{"bash": true}
	m.input.SetValue("investiga")
	nm, cmd := m.submit()
	m = pump(t, nm.(Model), cmd)

	a := m.run.agent
	if a == nil || !a.paused || a.wrapup {
		t.Fatalf("el agente debía quedar pausado: %+v", a)
	}
	if !strings.Contains(m.toast, "Límite de 5 pasos") {
		t.Fatalf("toast = %q", m.toast)
	}
	before := len(a.msgs)
	if before < 5 {
		t.Fatalf("historial demasiado corto: %d mensajes", before)
	}

	m.input.SetValue("continúa")
	nm, cmd = m.submit()
	m = pump(t, nm.(Model), cmd)
	a = m.run.agent
	if a == nil || !a.paused {
		t.Fatalf("el turno reanudado debía volver a pausarse: %+v", a)
	}
	if len(a.msgs) <= before {
		t.Fatalf("continuar no añadió historial: %d → %d", before, len(a.msgs))
	}
}

func TestAgentStepLabel(t *testing.T) {
	m := Model{cfg: config.Defaults()}
	if label := m.agentStepLabel(); label != "" {
		t.Fatalf("sin agente = %q", label)
	}
	m.run.agent = &agentRun{steps: 5}
	if label := m.agentStepLabel(); label != "paso 5/30" {
		t.Fatalf("label = %q", label)
	}
	m.run.agent.paused = true
	if label := m.agentStepLabel(); label != "pausado 5/30" {
		t.Fatalf("label pausado = %q", label)
	}
}

func TestResumeAgentConservaHistorial(t *testing.T) {
	m := Model{cfg: config.Defaults()}
	m.msgs = []Message{
		{Role: "user", Text: "investiga WordCraft"},
		{Role: "assistant", Text: "Herramienta · bash\nresultado"},
		{Role: "user", Text: "continúa"},
	}
	history := []api.ChatMessage{
		{Role: "system", Content: "sistema"},
		{Role: "user", Content: "investiga WordCraft"},
		{Role: "assistant", Content: "", ToolCalls: []api.ToolCall{{ID: "c1"}}},
		{Role: "tool", Content: "resultado completo", ToolCallID: "c1"},
		{Role: "assistant", Content: "resumen"},
	}
	m.run.agent = &agentRun{msgs: history, steps: 30, paused: true, wrapup: false}
	nm, _ := m.resumeAgent()
	m = nm
	defer m.dropAgent()

	a := m.run.agent
	if a == nil {
		t.Fatal("el agente debe seguir vivo")
	}
	if a.paused || a.wrapup {
		t.Fatalf("paused=%v wrapup=%v", a.paused, a.wrapup)
	}
	if a.steps != 1 {
		t.Fatalf("steps = %d; debe reiniciarse y contar el primer paso", a.steps)
	}
	found := false
	for _, msg := range a.msgs {
		if msg.Role == "tool" && msg.Content == "resultado completo" {
			found = true
		}
	}
	if !found {
		t.Fatal("se perdio el resultado completo de la herramienta")
	}
	last := a.msgs[len(a.msgs)-1]
	if last.Role != "user" || last.Content != "continúa" {
		t.Fatalf("ultimo mensaje = %+v", last)
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
