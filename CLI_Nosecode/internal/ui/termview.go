package ui

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-runewidth"

	"nosecode/internal/tools"
)

// Vista de una terminal (PTY) del agente: el usuario puede mirar y escribir en
// ella (por ejemplo, una contraseña que el modelo no debe ver). Salir: ctrl+].

type termViewState struct{ id int }

type termTickMsg struct{}

func termTick() tea.Cmd {
	return tea.Tick(120*time.Millisecond, func(time.Time) tea.Msg { return termTickMsg{} })
}

// termSlash: /terminal | /terminal <id> | /terminal new [cmd] | /terminal close <id>
func (m Model) termSlash(args []string) Model {
	say := func(t string) Model {
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: t})
		return m
	}
	if !tools.PTYAvailable() {
		return say("Las terminales interactivas (PTY) no están disponibles en Windows todavía.")
	}
	if len(args) == 0 {
		list := tools.TermList()
		var b strings.Builder
		b.WriteString("Terminales interactivas (las abre el agente con la herramienta `terminal`, o tú con /terminal new):\n\n")
		if len(list) == 0 {
			b.WriteString("(ninguna abierta)\n")
		}
		for _, t := range list {
			fmt.Fprintf(&b, "  %d  %s  %s\n", t.ID, t.Status, t.Command)
		}
		b.WriteString("\n/terminal <id> → entrar y escribir (salir con ctrl+])\n/terminal new [comando] → abrir una (vacío = shell)\n/terminal close <id> → cerrar")
		return say(b.String())
	}
	switch strings.ToLower(args[0]) {
	case "new", "nueva":
		id, err := tools.TermOpen(strings.Join(args[1:], " "))
		if err != nil {
			m.toast, m.toastErr = err.Error(), true
			return m
		}
		return m.openTermView(id)
	case "close", "cerrar":
		if len(args) < 2 {
			m.toast, m.toastErr = "Uso: /terminal close <id>", true
			return m
		}
		id, _ := strconv.Atoi(args[1])
		tools.TermClose(id)
		m.toast, m.toastErr = "Terminal cerrada", false
		return m
	}
	id, err := strconv.Atoi(args[0])
	if err != nil {
		m.toast, m.toastErr = "Uso: /terminal [<id>|new [cmd]|close <id>]", true
		return m
	}
	return m.openTermView(id)
}

func (m Model) openTermView(id int) Model {
	if _, _, _, _, err := tools.TermScreen(id); err != nil {
		m.toast, m.toastErr = err.Error(), true
		return m
	}
	m.termView = termViewState{id: id}
	m.screen = screenTerminal
	tools.TermResize(id, m.width-6, m.height-6)
	return m
}

func (m Model) termKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if k.Type == tea.KeyCtrlCloseBracket {
		m.screen = screenChat
		m.termView = termViewState{}
		return m, nil
	}
	if b := keyToBytes(k); len(b) > 0 {
		if err := tools.TermWrite(m.termView.id, b); err != nil {
			m.toast, m.toastErr = err.Error(), true
		}
	}
	return m, nil
}

// keyToBytes traduce una tecla de Bubble Tea a lo que espera un terminal.
func keyToBytes(k tea.KeyMsg) []byte {
	var out string
	switch k.Type {
	case tea.KeyRunes:
		out = string(k.Runes)
	case tea.KeySpace:
		out = " "
	case tea.KeyEnter:
		out = "\r"
	case tea.KeyTab:
		out = "\t"
	case tea.KeyShiftTab:
		out = "\x1b[Z"
	case tea.KeyBackspace:
		out = "\x7f"
	case tea.KeyEsc:
		out = "\x1b"
	case tea.KeyUp:
		out = "\x1b[A"
	case tea.KeyDown:
		out = "\x1b[B"
	case tea.KeyRight:
		out = "\x1b[C"
	case tea.KeyLeft:
		out = "\x1b[D"
	case tea.KeyHome:
		out = "\x1b[H"
	case tea.KeyEnd:
		out = "\x1b[F"
	case tea.KeyPgUp:
		out = "\x1b[5~"
	case tea.KeyPgDown:
		out = "\x1b[6~"
	case tea.KeyDelete:
		out = "\x1b[3~"
	case tea.KeyInsert:
		out = "\x1b[2~"
	default:
		if k.Type >= tea.KeyCtrlA && k.Type <= tea.KeyCtrlZ {
			out = string(rune(k.Type-tea.KeyCtrlA) + 1)
		} else if k.Type == tea.KeyNull {
			out = "\x00"
		} else {
			return nil
		}
	}
	if k.Alt {
		out = "\x1b" + out
	}
	return []byte(out)
}

func (m Model) termScreen() []uiLine {
	id := m.termView.id
	lines, row, col, exited, err := tools.TermScreen(id)
	title := fmt.Sprintf("Terminal %d", id)
	if err != nil {
		m.screen = screenChat
		return []uiLine{{text: warningStyle.Render(err.Error())}}
	}
	if exited {
		title += " · terminó"
	}
	boxW := clamp(m.width-2, 40, 210)
	innerW := boxW - 4
	head := []uiLine{{text: modalTitleStyle.Render(title)}}
	var body []uiLine
	maxRows := clamp(m.height-6, 5, 60)
	if len(lines) > maxRows {
		lines = lines[:maxRows]
	}
	for y := 0; y < maxRows && (y < len(lines) || y <= row-1); y++ {
		ln := ""
		if y < len(lines) {
			ln = lines[y]
		}
		if y == row-1 && !exited {
			ln = withCursor(ln, col-1)
		} else {
			ln = runewidth.Truncate(ln, innerW, "")
		}
		body = append(body, uiLine{text: ln})
	}
	footer := "teclas → programa · ctrl+] salir · ctrl+q cerrar Nosecode"
	return m.centerModal(m.modalFrame(boxW, head, body, footer), boxW)
}

// withCursor pinta el cursor (vídeo inverso) en la columna col (0-based).
func withCursor(line string, col int) string {
	var b strings.Builder
	cell, drawn := 0, false
	for _, r := range line {
		w := runewidth.RuneWidth(r)
		if !drawn && cell == col {
			b.WriteString(lipgloss.NewStyle().Reverse(true).Render(string(r)))
			drawn = true
		} else {
			b.WriteRune(r)
		}
		cell += w
	}
	if !drawn {
		b.WriteString(strings.Repeat(" ", max(col-cell, 0)))
		b.WriteString(lipgloss.NewStyle().Reverse(true).Render(" "))
	}
	return b.String()
}
