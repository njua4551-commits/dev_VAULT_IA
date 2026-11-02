package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type paletteItem struct {
	label    string
	category string
	shortcut string
	keywords string
	action   func(Model) Model
}

type palette struct {
	items    []paletteItem
	filtered []paletteItem
	cursor   int
	query    string
}

func (p *palette) setItems(items []paletteItem) {
	p.items = items
	p.filtered = items
	p.cursor = 0
	p.query = ""
}

func (p *palette) filter() {
	q := strings.ToLower(strings.TrimSpace(p.query))
	if q == "" {
		p.filtered = p.items
		p.cursor = 0
		return
	}
	out := make([]paletteItem, 0, len(p.items))
	for _, it := range p.items {
		haystack := strings.ToLower(strings.Join([]string{
			it.label, it.category, it.keywords, it.shortcut,
		}, " "))
		if strings.Contains(haystack, q) {
			out = append(out, it)
		}
	}
	p.filtered = out
	p.cursor = 0
}

func (m Model) openPalette() Model {
	items := []paletteItem{
		{"Nueva sesión", "Sesión", "ctrl+n", "crear abrir", func(m Model) Model { return m.newSession() }},
		{"Renombrar sesión", "Sesión", "f2", "título nombre", func(m Model) Model { return m.openRename(m.activeID) }},
		{"Eliminar sesión", "Sesión", "ctrl+w", "borrar historial pestaña", func(m Model) Model { return m.openConfirmDelete(m.activeID) }},
	}
	items = append(items, m.sessionItems()...)
	items = append(items, []paletteItem{
		{"Modo Plan", "Agente", "shift+tab", "describir plan sin ejecutar", func(m Model) Model { return m.setMode("plan") }},
		{"Modo Build", "Agente", "shift+tab", "ejecutar cambios", func(m Model) Model { return m.setMode("build") }},
		{"Herramientas MCP", "Agente", "/mcp", "tools servidores estado", func(m Model) Model { return m.mcpSlash("") }},
		{"Conectar MCP", "Agente", "/mcp import", "importar opencode servidores", func(m Model) Model { return m.mcpSlash("import") }},

		{"Configuración", "Ajustes", "f1", "ajustes preferencias", func(m Model) Model { return m.openSettings(0) }},
		{"Proveedores", "Ajustes", "", "endpoint api key", func(m Model) Model { return m.openSettings(0) }},
		{"Modelo", "Ajustes", "", "modelo contexto", func(m Model) Model { return m.openSettings(0) }},
		{"Permisos", "Ajustes", "", "bash edición web ask allow deny", func(m Model) Model { return m.openSettings(0) }},
		{"Apariencia", "Ajustes", "", "tema sonidos tokens barra", func(m Model) Model { return m.openSettings(0) }},
		{"Cambiar tema", "Ajustes", "", "vercel midnight color", func(m Model) Model { return m.cycleTheme() }},
		{"Sonidos", "Ajustes", "", "mute silenciar audio", func(m Model) Model {
			m.cfg.UI.Mute = !m.cfg.UI.Mute
			m.saveCfg()
			m.toast = "Sonidos: " + onOff(!m.cfg.UI.Mute)
			return m
		}},
		{"Barra lateral", "Ajustes", "ctrl+b", "sidebar panel", func(m Model) Model {
			m.cfg.UI.Sidebar = !m.cfg.UI.Sidebar
			m = m.saveCfg()
			m.toast = "Barra lateral: " + onOff(m.cfg.UI.Sidebar)
			return m
		}},

		{"Importar del Vault", "Sistema", "", "vault ngrok endpoint", func(m Model) Model { return m.importVault() }},
		{"Apagar la IA", "Sistema", "f8", "down detener kaggle gpu", func(m Model) Model { return m.openConfirmDown() }},
		{"Ayuda y atajos", "Sistema", "/help", "comandos teclas", func(m Model) Model {
			m.msgs = append(m.msgs, Message{Role: "assistant", Text: helpText})
			return m.saveActive()
		}},
		{"Salir", "Sistema", "ctrl+c", "cerrar quit", func(m Model) Model {
			m.quit = true
			return m
		}},
	}...)
	if m.plan != "" {
		items = append(items, paletteItem{
			label: "Aprobar plan y pasar a Build", category: "Plan pendiente", shortcut: "ctrl+e",
			keywords: "ejecutar aprobar", action: func(m Model) Model { return m.approvePlan() },
		})
	}
	m.palette.setItems(items)
	m.screen = screenPalette
	m.toast = ""
	return m
}

func (m Model) paletteKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.screen = screenChat
		return m, nil
	case "up", "shift+tab":
		if len(m.palette.filtered) > 0 {
			m.palette.cursor = (m.palette.cursor - 1 + len(m.palette.filtered)) % len(m.palette.filtered)
		}
		return m, nil
	case "down", "tab":
		if len(m.palette.filtered) > 0 {
			m.palette.cursor = (m.palette.cursor + 1) % len(m.palette.filtered)
		}
		return m, nil
	case "enter":
		if len(m.palette.filtered) > 0 {
			item := m.palette.filtered[m.palette.cursor]
			m.screen = screenChat
			m = item.action(m)
		}
		return m, nil
	case "backspace":
		if m.palette.query != "" {
			r := []rune(m.palette.query)
			m.palette.query = string(r[:len(r)-1])
			m.palette.filter()
		}
		return m, nil
	case "ctrl+u":
		m.palette.query = ""
		m.palette.filter()
		return m, nil
	}

	if k.Type == tea.KeyRunes {
		m.palette.query += string(k.Runes)
		m.palette.filter()
	}
	return m, nil
}

func (m Model) paletteScreen() []uiLine {
	boxW := clamp(m.width-10, 44, 76)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4

	title := modalTitleStyle.Render("Comandos")
	right := faintStyle.Render("esc cerrar")
	head := []uiLine{
		{text: title + spaces(maxInt(0, innerW-lipgloss.Width(title)-lipgloss.Width(right))) + right},
		m.paletteSearchLine(),
	}

	list := []uiLine{}
	cursorLine := 0
	if len(m.palette.filtered) == 0 {
		list = append(list, uiLine{text: faintStyle.Render("Sin resultados")})
	}

	category := ""
	for i, it := range m.palette.filtered {
		if it.category != category {
			category = it.category
			if len(list) > 0 {
				list = append(list, uiLine{})
			}
			list = append(list, uiLine{text: " " + accentStyle.Bold(true).Render(category)})
		}
		if i == m.palette.cursor {
			cursorLine = len(list)
		}
		list = append(list, m.paletteRowLine(innerW, it, i, i == m.palette.cursor))
	}

	maxBody := m.height - 9
	if maxBody < 4 {
		maxBody = 4
	}
	if len(list) > maxBody {
		start := cursorLine - maxBody + 2
		if start < 0 {
			start = 0
		}
		if start > len(list)-maxBody {
			start = len(list) - maxBody
		}
		list = list[start : start+maxBody]
	}

	box := m.modalFrame(boxW, head, list, "↑↓ navegar · enter ejecutar · esc cerrar")
	return m.centerModal(box, boxW)
}

func (m Model) paletteSearchLine() uiLine {
	if m.palette.query == "" {
		return uiLine{text: primaryStyle.Render("❯ ") + faintStyle.Render("Escribe un comando…")}
	}
	return uiLine{text: primaryStyle.Render("❯ ") + textStyle.Render(m.palette.query) + faintStyle.Render("▌")}
}

func (m Model) paletteRowLine(innerW int, it paletteItem, idx int, selected bool) uiLine {
	prefix := "   " + textStyle.Render(it.label)
	shortcut := it.shortcut
	if shortcut != "" {
		shortcut = fit(shortcut, innerW-lipgloss.Width(prefix)-3)
		if lipgloss.Width(prefix)+lipgloss.Width(shortcut)+2 > innerW {
			shortcut = ""
		}
	}

	var text string
	if shortcut == "" {
		text = pad(prefix, innerW)
	} else {
		gap := innerW - lipgloss.Width(prefix) - lipgloss.Width(shortcut) - 1
		if gap < 1 {
			gap = 1
		}
		text = prefix + spaces(gap) + mutedStyle.Render(shortcut) + " "
	}

	if selected {
		text = selectRow(text, innerW)
	} else {
		text = pad(text, innerW)
	}

	return uiLine{
		text: text,
		hits: []relHit{{x: 0, w: innerW, action: "paletteItem", arg: strconv.Itoa(idx)}},
	}
}

// sessionItems lista las sesiones abiertas para cambiar entre ellas
// (sustituye a las pestañas: opencode no tiene barra de pestañas).
func (m Model) sessionItems() []paletteItem {
	out := make([]paletteItem, 0, len(m.sessions))
	for _, s := range m.sessions {
		id := s.ID
		label := "  " + fit(s.Title, 44)
		if id == m.activeID {
			label = "● " + fit(s.Title, 44)
		}
		out = append(out, paletteItem{
			label:    label,
			category: "Sesiones",
			keywords: "cambiar abrir ir sesión " + s.Title,
			action:   func(m Model) Model { return m.loadSession(id) },
		})
	}
	return out
}
