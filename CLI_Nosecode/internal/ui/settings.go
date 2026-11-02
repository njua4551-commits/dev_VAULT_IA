package ui

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"nosecode/internal/config"
	"nosecode/internal/version"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// settings es una lista plana de ajustes agrupada por categorias.
type settings struct {
	cursor  int
	query   string
	editing bool
	isNew   bool
	oldName string
	fields  []textinput.Model
	field   int
}

// settingRow es una fila del panel. kind "header" es un titulo de grupo.
type settingRow struct {
	kind  string
	label string
	value string
	arg   string
}

var fieldLabels = []string{"Nombre", "Base URL", "API key", "Modelo", "Contexto"}

func (m Model) openSettings(_ int) Model {
	if m.screen != screenSettings {
		m.settings.query = ""
		m.settings.cursor = 0
		m.settings.editing = false
		m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), m.settings.cursor)
	}
	m.screen = screenSettings
	m.layoutSettings()
	return m
}

func (m *Model) layoutSettings() {
	if !m.settings.editing {
		return
	}
	w := m.modalInnerWidth() - 16
	if w < 20 {
		w = 20
	}
	for i := range m.settings.fields {
		m.settings.fields[i].Width = w
	}
}

func (m Model) modalInnerWidth() int {
	boxW := clamp(m.width-8, 42, 92)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	return boxW - 4
}

// ------------------------------------------------------------ filas ---

func (m Model) settingRows() []settingRow {
	rows := []settingRow{}
	header := func(name string) {
		rows = append(rows, settingRow{kind: "header", label: name})
	}
	row := func(kind, label, value, arg string) {
		rows = append(rows, settingRow{kind: kind, label: label, value: value, arg: arg})
	}

	header("Apariencia")
	row("mode", "Modo", modeLabel(m.mode), "")
	row("theme", "Tema", m.cfg.UI.Theme, "")
	row("toggle", "Sonidos", onOff(!m.cfg.UI.Mute), "sound")
	row("toggle", "Contador de tokens", onOff(m.cfg.UI.Tokens), "tokens")
	row("toggle", "Barra lateral", onOff(m.cfg.UI.Sidebar), "sidebar")
	row("toggle", "Animaciones", onOff(m.cfg.UI.Animations), "animations")

	header("Proveedores")
	for _, name := range m.providerNames() {
		p := m.cfg.Providers[name]
		value := valueOr(p.Model, "sin modelo")
		if name == m.cfg.DefaultProvider {
			value = "predeterminado · " + value
		}
		row("provider", name, value, name)
	}
	row("new", "Nuevo proveedor", "", "")
	row("import", "Importar del Vault", "", "")

	header("Modelo")
	name, p := m.cfg.Active()
	row("editmodel", "Modelo", valueOr(p.Model, "sin modelo"), "")
	row("provider", "Proveedor", name, name)
	ctx := "—"
	if p.ContextWindow > 0 {
		ctx = fmt.Sprintf("%d tokens", p.ContextWindow)
	}
	row("info", "Contexto", ctx, "")

	header("Permisos")
	row("perm", "Bash", m.cfg.Permissions.Bash, "bash")
	row("perm", "Edición", m.cfg.Permissions.Edit, "edit")
	row("perm", "Web fetch", m.cfg.Permissions.Webfetch, "web")
	row("perm", "Herramientas MCP", m.cfg.Permissions.MCP, "mcp")

	header("Sistema")
	row("info", "Configuración", m.cfg.Path, "")
	row("info", "Sesiones", config.SessionsDir(), "")
	row("rename", "Renombrar sesión", fit(m.activeTitle(), 28), "")
	row("sessiondelete", "Eliminar sesión", "borra el historial", "")
	row("newsession", "Nueva sesión", "ctrl+n", "")
	row("info", "Versión", version.Version, "")

	return rows
}

// filteredSettingRows aplica la busqueda y conserva las cabeceras necesarias.
func (m Model) filteredSettingRows() []settingRow {
	rows := m.settingRows()
	q := strings.ToLower(strings.TrimSpace(m.settings.query))
	if q == "" {
		return rows
	}

	out := []settingRow{}
	category := ""
	lastCategory := ""
	for _, r := range rows {
		if r.kind == "header" {
			category = r.label
			continue
		}
		haystack := strings.ToLower(r.label + " " + r.value + " " + category)
		if !strings.Contains(haystack, q) {
			continue
		}
		if lastCategory != category {
			out = append(out, settingRow{kind: "header", label: category})
			lastCategory = category
		}
		out = append(out, r)
	}
	return out
}

func normalizeSettingCursor(rows []settingRow, cursor int) int {
	if len(rows) == 0 {
		return 0
	}
	if cursor >= 0 && cursor < len(rows) && rows[cursor].kind != "header" {
		return cursor
	}
	for i, r := range rows {
		if r.kind != "header" {
			return i
		}
	}
	return 0
}

func (m Model) moveSetting(delta int) Model {
	rows := m.filteredSettingRows()
	if len(rows) == 0 {
		return m
	}
	i := normalizeSettingCursor(rows, m.settings.cursor)
	for n := 0; n < len(rows); n++ {
		i = (i + delta + len(rows)) % len(rows)
		if rows[i].kind != "header" {
			m.settings.cursor = i
			return m
		}
	}
	return m
}

func (m Model) activateSetting() (tea.Model, tea.Cmd) {
	rows := m.filteredSettingRows()
	if len(rows) == 0 {
		return m, nil
	}
	i := normalizeSettingCursor(rows, m.settings.cursor)
	m.settings.cursor = i
	row := rows[i]

	switch row.kind {
	case "mode":
		return m.toggleMode(), nil
	case "rename":
		return m.openRename(m.activeID), nil
	case "sessiondelete":
		return m.openConfirmDelete(m.activeID), nil
	case "toggle":
		return m.toggleOption(row.arg), nil
	case "theme":
		return m.cycleTheme(), nil
	case "perm":
		return m.cyclePermission(row.arg), nil
	case "provider":
		return m.startEdit(row.arg, false), nil
	case "new":
		return m.startEdit("", true), nil
	case "import":
		m = m.importVault()
		m.sound(soundSuccess)
		return m, nil
	case "editmodel":
		name, _ := m.cfg.Active()
		m = m.startEdit(name, false)
		if len(m.settings.fields) > 3 {
			m.settings.fields[m.settings.field].Blur()
			m.settings.field = 3
			m.settings.fields[3].Focus()
		}
		return m, nil
	case "newsession":
		return m.newSession(), nil
	}
	return m, nil
}

// ------------------------------------------------------------ teclas ---

func (m Model) settingsKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.settings.editing {
		return m.editKey(k)
	}

	switch k.String() {
	case "esc":
		if m.settings.query != "" {
			m.settings.query = ""
			m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), 0)
			return m, nil
		}
		m.screen = screenChat
		return m, nil
	case "up":
		return m.moveSetting(-1), nil
	case "down", "tab":
		return m.moveSetting(1), nil
	case "shift+tab":
		return m.moveSetting(-1), nil
	case "enter":
		return m.activateSetting()
	case "backspace":
		if m.settings.query != "" {
			r := []rune(m.settings.query)
			m.settings.query = string(r[:len(r)-1])
			m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), 0)
		}
		return m, nil
	case "ctrl+u":
		m.settings.query = ""
		m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), 0)
		return m, nil
	}

	if k.Type == tea.KeyRunes {
		m.settings.query += string(k.Runes)
		m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), 0)
	}
	return m, nil
}

// --------------------------------------------------------- proveedores ---

func (m Model) providerNames() []string {
	names := make([]string, 0, len(m.cfg.Providers))
	for name := range m.cfg.Providers {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func (m Model) activateProviderNamed(name string) (tea.Model, tea.Cmd) {
	if name == "" {
		name = m.cfg.DefaultProvider
	}
	if _, ok := m.cfg.Providers[name]; !ok {
		return m, nil
	}
	m.cfg.DefaultProvider = name
	m.settings.editing = false
	m = m.saveCfg()
	m.toast = "Proveedor activo: " + name
	return m, nil
}

func (m Model) deleteProvider() (tea.Model, tea.Cmd) {
	names := m.providerNames()
	if len(names) <= 1 {
		m.toast = "Debe quedar al menos un proveedor"
		m.toastErr = true
		return m, nil
	}

	name := m.settings.oldName
	if name == "" {
		name = m.cfg.DefaultProvider
	}
	if _, ok := m.cfg.Providers[name]; !ok {
		return m, nil
	}

	delete(m.cfg.Providers, name)
	if m.cfg.DefaultProvider == name {
		m.cfg.DefaultProvider = m.providerNames()[0]
	}
	m.settings.editing = false
	m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), m.settings.cursor)
	m = m.saveCfg()
	m.toast = "Proveedor eliminado: " + name
	return m, nil
}

func (m Model) startEdit(name string, isNew bool) Model {
	p := config.Provider{Type: "openai"}
	if !isNew {
		if got, ok := m.cfg.Provider(name); ok {
			p = got
		}
	} else {
		p.BaseURL = "https://"
		p.ContextWindow = 32768
	}

	contextValue := ""
	if p.ContextWindow > 0 {
		contextValue = strconv.Itoa(p.ContextWindow)
	}
	values := []string{name, p.BaseURL, p.APIKey, p.Model, contextValue}

	fields := make([]textinput.Model, len(values))
	for i, v := range values {
		ti := textinput.New()
		ti.Prompt = ""
		ti.CharLimit = 0
		ti.Width = m.modalInnerWidth() - 16
		ti.SetValue(v)
		fields[i] = ti
	}
	fields[0].Focus()
	m.settings.fields = fields
	m.settings.field = 0
	m.settings.editing = true
	m.settings.isNew = isNew
	m.settings.oldName = name
	m.layoutSettings()
	return m
}

func (m Model) editKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.settings.editing = false
		m.toast = "Edición cancelada"
		return m, nil
	case "ctrl+s":
		return m.saveEdit()
	case "tab", "down":
		m.focusField(1)
		return m, nil
	case "shift+tab", "up":
		m.focusField(-1)
		return m, nil
	case "enter":
		if m.settings.field == len(m.settings.fields)-1 {
			return m.saveEdit()
		}
		m.focusField(1)
		return m, nil
	}

	var cmd tea.Cmd
	i := m.settings.field
	if i < 0 || i >= len(m.settings.fields) {
		return m, nil
	}
	m.settings.fields[i], cmd = m.settings.fields[i].Update(k)
	return m, cmd
}

func (m Model) setField(arg string) Model {
	idx, err := strconv.Atoi(arg)
	if err != nil || idx < 0 || idx >= len(m.settings.fields) {
		return m
	}
	if m.settings.field < len(m.settings.fields) {
		m.settings.fields[m.settings.field].Blur()
	}
	m.settings.field = idx
	m.settings.fields[idx].Focus()
	return m
}

func (m *Model) focusField(delta int) {
	if len(m.settings.fields) == 0 {
		return
	}
	m.settings.fields[m.settings.field].Blur()
	m.settings.field = (m.settings.field + delta + len(m.settings.fields)) % len(m.settings.fields)
	m.settings.fields[m.settings.field].Focus()
}

func (m Model) saveEdit() (tea.Model, tea.Cmd) {
	get := func(i int) string {
		if i < len(m.settings.fields) {
			return strings.TrimSpace(m.settings.fields[i].Value())
		}
		return ""
	}

	name := get(0)
	if name == "" {
		m.toast = "El nombre no puede estar vacío"
		m.toastErr = true
		return m, nil
	}
	if len(strings.Fields(name)) > 1 {
		m.toast = "El nombre no puede tener espacios"
		m.toastErr = true
		return m, nil
	}

	baseURL := config.NormalizeBaseURL(get(1))
	if baseURL == "https:" {
		baseURL = ""
	}
	apiKey := get(2)
	model := get(3)
	ctx, _ := strconv.Atoi(get(4))
	if ctx < 0 {
		ctx = 0
	}

	wasDefault := m.settings.oldName != "" && m.cfg.DefaultProvider == m.settings.oldName
	if m.settings.oldName != "" && m.settings.oldName != name {
		delete(m.cfg.Providers, m.settings.oldName)
	}
	m.cfg.Providers[name] = config.Provider{
		Type:          "openai",
		BaseURL:       baseURL,
		APIKey:        apiKey,
		Model:         model,
		ContextWindow: ctx,
	}
	if wasDefault {
		m.cfg.DefaultProvider = name
	}
	if m.settings.isNew && len(m.cfg.Providers) == 1 {
		m.cfg.DefaultProvider = name
	}

	m = m.saveCfg()
	m.settings.editing = false
	m.toastErr = false
	m.toast = "Guardado: " + name
	m.settings.cursor = normalizeSettingCursor(m.filteredSettingRows(), m.settings.cursor)
	return m, nil
}

// ------------------------------------------------------------- render ---

func (m Model) settingsScreen() []uiLine {
	boxW := clamp(m.width-8, 42, 92)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4

	title := modalTitleStyle.Render("Configuración")
	right := faintStyle.Render("esc cerrar")
	sub := m.settingsSearchLine()
	if m.settings.editing {
		name := m.settings.oldName
		if name == "" {
			name = "nuevo proveedor"
		}
		sub = uiLine{text: faintStyle.Render("Proveedor · " + name)}
	}
	head := []uiLine{
		{text: title + spaces(maxInt(0, innerW-lipgloss.Width(title)-lipgloss.Width(right))) + right},
		sub,
	}

	if m.settings.editing {
		body := m.editFormLines(innerW)
		box := m.modalFrame(boxW, head, body, m.settingsHint())
		return m.centerModal(box, boxW)
	}

	rows := m.filteredSettingRows()
	cursor := normalizeSettingCursor(rows, m.settings.cursor)

	list := []uiLine{}
	cursorLine := 0
	if len(rows) == 0 {
		list = append(list, uiLine{text: faintStyle.Render("Sin resultados")})
	}
	for i, r := range rows {
		if r.kind == "header" {
			if len(list) > 0 {
				list = append(list, uiLine{})
			}
			list = append(list, m.settingRowLine(innerW, r, i, false))
			continue
		}
		if i == cursor {
			cursorLine = len(list)
		}
		list = append(list, m.settingRowLine(innerW, r, i, i == cursor))
	}

	maxBody := m.height - 9
	if maxBody < 5 {
		maxBody = 5
	}
	maxList := maxBody - 2
	if maxList < 1 {
		maxList = 1
	}
	if len(list) > maxList {
		start := cursorLine - maxList + 2
		if start < 0 {
			start = 0
		}
		if start > len(list)-maxList {
			start = len(list) - maxList
		}
		list = list[start : start+maxList]
	}

	body := append([]uiLine{}, list...)
	body = append(body, uiLine{})
	body = append(body, m.buttonRow(
		uiButton{"＋ Nuevo", "providerNew", "", true},
		uiButton{"Importar", "providerImport", "", false},
		uiButton{"Nueva sesión", "new", "", false},
	))

	box := m.modalFrame(boxW, head, body, m.settingsHint())
	return m.centerModal(box, boxW)
}

func (m Model) settingsSearchLine() uiLine {
	if m.settings.query == "" {
		return uiLine{text: primaryStyle.Render("❯ ") + faintStyle.Render("Buscar ajustes…")}
	}
	return uiLine{text: primaryStyle.Render("❯ ") + textStyle.Render(m.settings.query) + faintStyle.Render("▌")}
}

func (m Model) settingRowLine(innerW int, r settingRow, idx int, selected bool) uiLine {
	if r.kind == "header" {
		return uiLine{text: " " + accentStyle.Bold(true).Render(r.label)}
	}

	value := r.value
	prefix := "   " + textStyle.Render(r.label)
	maxValue := innerW - lipgloss.Width(prefix) - 3
	if maxValue < 4 {
		value = ""
		maxValue = 0
	}
	if value != "" {
		value = fit(value, maxValue)
	}

	var text string
	if value == "" {
		text = pad(prefix, innerW)
	} else {
		styled := m.settingValueStyle(r.kind, value).Render(value)
		gap := innerW - lipgloss.Width(prefix) - lipgloss.Width(value) - 1
		if gap < 1 {
			gap = 1
		}
		text = prefix + spaces(gap) + styled + " "
	}

	if selected {
		text = selectRow(text, innerW)
	} else {
		text = pad(text, innerW)
	}

	return uiLine{
		text: text,
		hits: []relHit{{x: 0, w: innerW, action: "settingRow", arg: strconv.Itoa(idx)}},
	}
}

func (m Model) settingValueStyle(kind, value string) lipgloss.Style {
	switch kind {
	case "toggle":
		switch value {
		case "on":
			return successStyle
		case "off":
			return faintStyle
		}
	case "perm":
		switch value {
		case "allow":
			return successStyle
		case "deny":
			return dangerStyle
		default:
			return warningStyle
		}
	case "theme":
		return secondaryStyle
	case "mode":
		if value == "Plan" {
			return warningStyle
		}
		return successStyle
	case "rename":
		return secondaryStyle
	case "sessiondelete":
		return dangerStyle
	case "provider":
		if strings.HasPrefix(value, "predeterminado") {
			return successStyle
		}
		return mutedStyle
	case "new", "newsession":
		return primaryStyle
	case "import":
		return secondaryStyle
	}
	return faintStyle
}

func (m Model) editFormLines(innerW int) []uiLine {
	title := "Editar proveedor"
	if m.settings.isNew {
		title = "Nuevo proveedor"
	}
	lines := []uiLine{
		{text: warningStyle.Render(title) + faintStyle.Render("   ctrl+s guardar · esc cancelar")},
	}

	for i, label := range fieldLabels {
		labelStyle := mutedStyle
		if i == m.settings.field {
			labelStyle = primaryStyle
		}
		labelText := labelStyle.Render(fmt.Sprintf("%-11s", label))
		value := ""
		if i < len(m.settings.fields) {
			value = m.settings.fields[i].View()
		}
		ln := uiLine{text: " " + labelText + " " + value}
		ln.hits = []relHit{{x: 0, w: innerW, action: "field", arg: strconv.Itoa(i)}}
		lines = append(lines, ln)
	}

	lines = append(lines, uiLine{})
	lines = append(lines, m.buttonRow(
		uiButton{"Guardar", "saveEdit", "", true},
		uiButton{"Cancelar", "cancelEdit", "", false},
	))
	if !m.settings.isNew && m.settings.oldName != "" {
		lines = append(lines, m.buttonRow(
			uiButton{"Usar este proveedor", "providerActivate", m.settings.oldName, false},
			uiButton{"Borrar", "providerDelete", "", false},
		))
	}
	lines = append(lines, uiLine{text: faintStyle.Render("Base URL: https://abc.ngrok.app/v1 · la key queda en tu config local")})
	return lines
}

func (m Model) settingsHint() string {
	if m.settings.editing {
		return "tab / ↑↓ mover · enter siguiente · ctrl+s guardar · esc cancelar"
	}
	return "↑↓ mover · enter cambiar · escribe para filtrar · esc cerrar"
}

// ------------------------------------------------------------- modal ---

type uiButton struct {
	label   string
	action  string
	arg     string
	primary bool
}

func (m Model) buttonRow(buttons ...uiButton) uiLine {
	line := uiLine{text: " "}
	x := 1
	for i, b := range buttons {
		style := chipStyle
		switch {
		case b.action == "confirmDelete":
			style = chipDangerStyle
		case b.primary:
			style = chipPrimaryStyle
		}
		label := style.Render(b.label)
		if i > 0 {
			line.text += " "
			x++
		}
		line.hits = append(line.hits, relHit{x: x, w: lipgloss.Width(label), action: b.action, arg: b.arg})
		line.text += label
		x += lipgloss.Width(label)
	}
	return line
}

// modalFrame dibuja un diálogo como opencode: panel con fondo propio y sin
// borde, título arriba, lista y una línea de ayuda al pie.
func (m Model) modalFrame(boxW int, head, body []uiLine, footer string) []uiLine {
	innerW := boxW - 4
	out := []uiLine{}

	add := func(ln uiLine) {
		row := uiLine{text: paint(colPanel, "  "+pad(ln.text, innerW)+"  ")}
		for _, h := range ln.hits {
			row.hits = append(row.hits, relHit{x: h.x + 2, w: h.w, action: h.action, arg: h.arg})
		}
		out = append(out, row)
	}

	add(uiLine{})
	for _, ln := range head {
		add(ln)
	}
	add(uiLine{})
	for _, ln := range body {
		add(ln)
	}
	add(uiLine{})
	add(uiLine{text: faintStyle.Render(footer)})
	add(uiLine{})
	return out
}

func (m Model) centerModal(box []uiLine, boxW int) []uiLine {
	top := (m.height - len(box)) / 3
	if top < 0 {
		top = 0
	}
	x := (m.width - boxW) / 2
	if x < 0 {
		x = 0
	}

	lines := make([]uiLine, 0, m.height)
	for i := 0; i < top; i++ {
		lines = append(lines, blankLine(m.width))
	}
	for _, ln := range box {
		ln.text = spaces(x) + ln.text
		for i := range ln.hits {
			ln.hits[i].x += x
		}
		lines = append(lines, ln)
	}
	return lines
}

func valueOr(v, fallback string) string {
	if strings.TrimSpace(v) == "" {
		return fallback
	}
	return v
}
