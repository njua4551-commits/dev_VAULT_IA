package ui

import (
	"strings"
	"testing"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/session"

	"github.com/charmbracelet/lipgloss"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"
)

func TestToolBlockVistaPreviaYExpansion(t *testing.T) {
	b := &session.ToolBlock{
		CallID:     "c1",
		Name:       "bash",
		Args:       "$ seq 1 10",
		Status:     toolOK,
		Output:     "l1\nl2\nl3\nl4\nl5\nl6\nl7\nl8\nl9\nl10",
		DurationMS: 800,
	}
	m := newTestModel(t)
	lines := m.toolBlockLines(0, b, 100)
	var out strings.Builder
	for _, ln := range lines {
		out.WriteString(ansi.Strip(ln.text))
		out.WriteString("\n")
	}
	text := out.String()
	for _, want := range []string{"⚙ bash", "$ seq 1 10", "✓ ok 0,8 s · 10 líneas", "l1", "l5", "+5 líneas · ctrl+o"} {
		if !strings.Contains(text, want) {
			t.Fatalf("falta %q en:\n%s", want, text)
		}
	}
	if strings.Contains(text, "l6") {
		t.Fatalf("la vista previa no debe mostrar l6:\n%s", text)
	}
	b.Expanded = true
	lines = m.toolBlockLines(0, b, 100)
	out.Reset()
	for _, ln := range lines {
		out.WriteString(ansi.Strip(ln.text))
		out.WriteString("\n")
	}
	if !strings.Contains(out.String(), "l10") || !strings.Contains(out.String(), "ctrl+o colapsa") {
		t.Fatalf("la expansión no muestra la salida completa:\n%s", out.String())
	}
}

func TestToolBlockEstados(t *testing.T) {
	now := time.Now()
	cases := []struct {
		block *session.ToolBlock
		want  string
	}{
		{&session.ToolBlock{Status: toolApproval}, "esperando tu aprobación"},
		{&session.ToolBlock{Status: toolRunning, Started: now.Add(-12 * time.Second)}, "ejecutando 12 s"},
		{&session.ToolBlock{Status: toolError, ExitCode: 1}, "error (código 1)"},
		{&session.ToolBlock{Status: toolDenied}, "denegado"},
		{&session.ToolBlock{Status: toolCancelled}, "cancelado"},
		{&session.ToolBlock{Status: toolTimeout, TimeoutS: 60}, "timeout 60 s"},
	}
	for _, tc := range cases {
		got := ansi.Strip(toolStatusLine(tc.block, now))
		if !strings.Contains(got, tc.want) {
			t.Fatalf("estado %q = %q, quería %q", tc.block.Status, got, tc.want)
		}
	}
}

func TestAddToolBlocksSinBloqueVacio(t *testing.T) {
	m := newTestModel(t)
	m.run.agent = &agentRun{}
	call := api.ToolCall{ID: "c1", Type: "function"}
	call.Function.Name = "bash"
	call.Function.Arguments = `{"command":"pwd"}`
	m = m.addToolBlocks([]api.ToolCall{call})
	if len(m.msgs) != 1 {
		t.Fatalf("mensajes = %d, quería 1 bloque", len(m.msgs))
	}
	b := m.msgs[0].Tool
	if b == nil || b.Name != "bash" || b.Args != "$ pwd" || b.CallID != "c1" {
		t.Fatalf("bloque inesperado: %+v", b)
	}
	if b.Status == "" {
		t.Fatal("el bloque debe tener estado")
	}
}

func TestMensajeEnColaMientrasAgenteOcupado(t *testing.T) {
	m := newTestModel(t)
	m.run.agent = &agentRun{pending: []api.ToolCall{{}}}
	m.input.SetValue("segundo mensaje")
	updated, _ := m.key(tea.KeyMsg{Type: tea.KeyEnter})
	nm := updated.(Model)
	if len(nm.queue) != 1 || nm.queue[0] != "segundo mensaje" {
		t.Fatalf("cola = %v", nm.queue)
	}
	if strings.TrimSpace(nm.input.Value()) != "" {
		t.Fatalf("el input debía vaciarse: %q", nm.input.Value())
	}
	if len(nm.msgs) != 0 {
		t.Fatalf("no debe iniciarse un turno paralelo: %d mensajes", len(nm.msgs))
	}
}

func TestLineaActividadFijaYVigilante(t *testing.T) {
	m := newTestModel(t)
	m.activity = activityState{kind: "tool", label: "bash", started: time.Now().Add(-2 * time.Second)}
	if got := m.activityLineText(); !strings.Contains(got, "ejecutando bash") || !strings.Contains(got, "esc cancela") {
		t.Fatalf("actividad = %q", got)
	}
	m = m.stopActivity()
	if got := m.activityLineText(); got != "listo" {
		t.Fatalf("en reposo = %q", got)
	}

	m.activity = activityState{kind: "model", started: time.Now().Add(-50 * time.Second)}
	m.tunnelState = "ok"
	m.run.agent = &agentRun{stream: &api.Streamer{}, lastToken: time.Now().Add(-50 * time.Second)}
	got := m.activityLineText()
	if !strings.Contains(got, "el modelo no responde hace") || !strings.Contains(got, "túnel OK") {
		t.Fatalf("vigilante = %q", got)
	}
}

func TestDiffRowsVerdeYRojo(t *testing.T) {
	m := newTestModel(t)
	b := &session.ToolBlock{Name: "edit_file", Status: toolOK, Diff: "+1 −1\n@@ línea 2 @@\n  a\n- viejo\n+ nuevo\n  z"}
	var rowS func(string, lipgloss.Style) uiLine
	rowS = func(s string, st lipgloss.Style) uiLine { return uiLine{text: s} }
	rows := m.diffRows(b, 60, rowS, func(s string, f bool) uiLine { return uiLine{text: s} })
	var got []string
	for _, r := range rows {
		got = append(got, r.text)
	}
	joined := strings.Join(got, "\n")
	for _, want := range []string{"cambios +1 −1", "- viejo", "+ nuevo"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("falta %q en %q", want, joined)
		}
	}
}

func TestQuestionSeVeComoProsa(t *testing.T) {
	m := newTestModel(t)
	m.width = 100
	b := &session.ToolBlock{
		Name:   "question",
		Args:   "{\"question\":\"¿Qué enfoque?\\n```go\\nfunc a() {}\\n```\",\"options\":[\"Opción uno\",\"Opción dos\"]}",
		Status: toolOK,
		Output: "El usuario respondió: Opción dos",
	}
	var sb strings.Builder
	for _, l := range m.toolBlockLines(0, b, 100) {
		sb.WriteString(l.text + "\n")
	}
	out := sb.String()
	for _, want := range []string{"Pregunta", "¿Qué enfoque?", "│ func a() {}", "1) Opción uno", "2) Opción dos", "→ Opción dos"} {
		if !strings.Contains(out, want) {
			t.Fatalf("falta %q en:\n%s", want, out)
		}
	}
	for _, bad := range []string{`"options"`, "líneas", "```"} {
		if strings.Contains(out, bad) {
			t.Fatalf("no debía contener %q:\n%s", bad, out)
		}
	}
}
