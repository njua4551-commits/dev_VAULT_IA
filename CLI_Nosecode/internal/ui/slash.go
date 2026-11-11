package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type slashState struct {
	hidden bool
	cursor int
}

type slashEntry struct {
	name string
	desc string
	args bool
}

var slashEntries = []slashEntry{
	{"/help", "ayuda y atajos", false},
	{"/settings", "abrir ajustes", false},
	{"/model", "ver o cambiar el modelo", true},
	{"/sessions", "cambiar de sesión", false},
	{"/copy", "última respuesta · code [n] · files", true},
	{"/mcp", "servidores MCP: import, tools, on/off", true},
	{"/tools", "herramientas activas; browser/mcp on|off", true},
	{"/compact", "resumir la conversación y ahorrar contexto", false},
	{"/agent", "activar o desactivar herramientas", false},
	{"/vault", "importar endpoint del Vault", false},
	{"/up", "encender la IA en Kaggle", false},
	{"/down", "apagar la IA y dejar de gastar GPU", false},
	{"/urls", "ver túneles vivos", false},
	{"/status", "estado del notebook", false},
	{"/tokens", "mostrar u ocultar el contador", false},
	{"/sound", "activar o silenciar sonidos", false},
	{"/theme", "cambiar tema", false},
	{"/plan", "modo Plan", false},
	{"/build", "modo Build", false},
	{"/new", "nueva sesión", false},
	{"/rename", "renombrar sesión", false},
	{"/delete", "eliminar sesión", false},
	{"/config", "ruta del config", false},
	{"/quit", "salir", false},
}

func (m Model) slashOpen() bool {
	if m.screen != screenChat || m.slashMenu.hidden {
		return false
	}
	v := m.input.Value()
	if !strings.HasPrefix(v, "/") {
		return false
	}
	return !strings.ContainsAny(v, " \n\t")
}

func (m Model) slashQuery() string {
	if !m.slashOpen() {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m.input.Value()))
}

func (m Model) slashMatches() []slashEntry {
	q := m.slashQuery()
	if q == "" {
		return nil
	}
	out := make([]slashEntry, 0, len(slashEntries))
	for _, e := range slashEntries {
		if q == "/" || strings.HasPrefix(e.name, q) {
			out = append(out, e)
		}
	}
	return out
}

func (m Model) runSlashEntry(e slashEntry) (Model, tea.Cmd) {
	m.input.SetValue(e.name)
	m.slashMenu.cursor = 0
	if e.args {
		m.input.SetValue(e.name + " ")
		m.slashMenu.hidden = true
		return m, nil
	}
	m.slashMenu.hidden = false
	nm, cmd := m.submit()
	return nm.(Model), cmd
}

func (m Model) slashKey(k tea.KeyMsg) (Model, tea.Cmd, bool) {
	if !m.slashOpen() {
		return m, nil, false
	}
	matches := m.slashMatches()
	key := k.String()
	switch key {
	case "up":
		if len(matches) > 0 {
			m.slashMenu.cursor = (m.slashMenu.cursor - 1 + len(matches)) % len(matches)
		}
		return m, nil, true
	case "down":
		if len(matches) > 0 {
			m.slashMenu.cursor = (m.slashMenu.cursor + 1) % len(matches)
		}
		return m, nil, true
	case "tab":
		if len(matches) > 0 {
			e := matches[clamp(m.slashMenu.cursor, 0, len(matches)-1)]
			m.input.SetValue(e.name + " ")
			m.slashMenu.hidden = true
			m.slashMenu.cursor = 0
		}
		return m, nil, true
	case "enter":
		if len(matches) == 0 {
			return m, nil, false
		}
		e := matches[clamp(m.slashMenu.cursor, 0, len(matches)-1)]
		nm, cmd := m.runSlashEntry(e)
		return nm, cmd, true
	case "esc":
		m.slashMenu.hidden = true
		return m, nil, true
	}
	return m, nil, false
}

func (m Model) slashMenuLines(width int) []uiLine {
	if !m.slashOpen() {
		return nil
	}
	matches := m.slashMatches()
	if len(matches) == 0 {
		return nil
	}
	maxRows := 7
	if m.height < 20 {
		maxRows = 4
	}
	start := 0
	if len(matches) > maxRows {
		start = m.slashMenu.cursor - maxRows + 1
		if start < 0 {
			start = 0
		}
		if start > len(matches)-maxRows {
			start = len(matches) - maxRows
		}
		matches = matches[start : start+maxRows]
	}
	cursor := m.slashMenu.cursor - start
	lines := make([]uiLine, 0, len(matches))
	for i, e := range matches {
		selected := i == cursor
		left := "  " + e.name
		right := e.desc
		space := width - lipgloss.Width(left) - lipgloss.Width(right) - 4
		if space < 1 {
			space = 1
		}
		text := left + strings.Repeat(" ", space) + right + "  "
		if selected {
			lines = append(lines, uiLine{
				text: selectedStyle.Render(pad(text, width)),
				hits: []relHit{{x: 0, w: width, action: "slashItem", arg: strconv.Itoa(start + i)}},
			})
		} else {
			line := uiLine{text: paint(colPanel, pad(text, width))}
			line.hits = []relHit{{x: 0, w: width, action: "slashItem", arg: strconv.Itoa(start + i)}}
			lines = append(lines, line)
		}
	}
	return lines
}
