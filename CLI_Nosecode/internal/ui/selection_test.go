package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

func viewWithMessages(t *testing.T, msgs ...Message) Model {
	t.Helper()
	m := newTestModel(t)
	m.msgs = append([]Message{}, msgs...)
	m.View()
	return m
}

func findLine(m Model, substr string) (int, uiLine, bool) {
	for y, ln := range m.buf.lines {
		if strings.Contains(ansi.Strip(ln.text), substr) {
			return y, ln, true
		}
	}
	return 0, uiLine{}, false
}

func TestSelectionByCharacters(t *testing.T) {
	m := viewWithMessages(t, Message{Role: "assistant", Text: "Hola mundo"})
	y, ln, ok := findLine(m, "Hola mundo")
	if !ok || !ln.selectable {
		t.Fatal("no encuentro la linea seleccionable")
	}
	if ln.selStart != 5 {
		t.Fatalf("selStart = %d, queria 5", ln.selStart)
	}
	m.sel = selectState{
		active: true, moved: true,
		startY: y, startX: ln.selStart,
		endY: y, endX: ln.selStart + 4,
		panel: "chat", panelTop: y, panelBottom: y,
		panelX0: ln.panelX0, panelX1: ln.panelX1,
	}
	if got := m.selectedText(); got != "Hola" {
		t.Fatalf("selectedText = %q, queria %q", got, "Hola")
	}
}

func TestSelectionClipsToPanelAndTrimsPadding(t *testing.T) {
	m := viewWithMessages(t, Message{Role: "assistant", Text: "Hola mundo"})
	y, ln, ok := findLine(m, "Hola mundo")
	if !ok {
		t.Fatal("no encuentro la linea")
	}
	m.sel = selectState{
		active: true, moved: true,
		startY: y, startX: 0,
		endY: y, endX: m.width,
		panel: "chat", panelTop: y, panelBottom: y,
		panelX0: ln.panelX0, panelX1: ln.panelX1,
	}
	got := m.selectedText()
	if got != "Hola mundo" {
		t.Fatalf("selectedText = %q, queria %q", got, "Hola mundo")
	}
	if strings.Contains(got, "Proveedor") || strings.Contains(got, "Sesiones") {
		t.Fatalf("la seleccion invadio la barra lateral: %q", got)
	}
}

func TestDoubleClickSelectsWord(t *testing.T) {
	m := viewWithMessages(t, Message{Role: "assistant", Text: "Hola mundo"})
	y, ln, ok := findLine(m, "Hola mundo")
	if !ok {
		t.Fatal("no encuentro la linea")
	}
	m = m.selectWordAt(ln.selStart+1, y)
	if got := m.selectedText(); got != "Hola" {
		t.Fatalf("palabra = %q, queria %q", got, "Hola")
	}
}

func TestTripleClickExcludesHeader(t *testing.T) {
	m := viewWithMessages(t, Message{Role: "assistant", Text: "Hola mundo"})
	y, _, ok := findLine(m, "nosecode ·")
	if !ok {
		t.Fatal("no encuentro la cabecera")
	}
	m = m.selectMessageAt(5, y)
	got := m.selectedText()
	if got != "Hola mundo" {
		t.Fatalf("cuerpo = %q, queria %q", got, "Hola mundo")
	}
	if strings.Contains(got, "nosecode") {
		t.Fatalf("la cabecera entro en la seleccion: %q", got)
	}
}

func TestExtractCodeBlocks(t *testing.T) {
	text := "antes\n```go\nfmt.Println(1)\n```\nmedio\n```python\nprint(2)\n```\n"
	blocks := extractCodeBlocks(text)
	if len(blocks) != 2 {
		t.Fatalf("bloques = %d, queria 2", len(blocks))
	}
	if blocks[0].Lang != "go" || blocks[0].Body != "fmt.Println(1)" {
		t.Fatalf("bloque 1 = %+v", blocks[0])
	}
	if blocks[1].Lang != "python" || blocks[1].Body != "print(2)" {
		t.Fatalf("bloque 2 = %+v", blocks[1])
	}
}

func TestTabsExposeSessions(t *testing.T) {
	m := newTestModel(t)
	m = m.newSession()
	m.sessions[0].Title = "Primera"
	m.sessions[len(m.sessions)-1].Title = "Segunda"
	m.msgs = append(m.msgs, Message{Role: "user", Text: "hola"})
	m.View()
	hasSwitch, hasClose, hasNew := false, false, false
	for _, b := range m.hit.boxes {
		switch b.action {
		case "sessionSwitch":
			hasSwitch = true
		case "sessionClose":
			hasClose = true
		case "new":
			hasNew = true
		}
	}
	if !hasSwitch || !hasClose || !hasNew {
		t.Fatal("faltan zonas clicables en las pestanas")
	}
}
