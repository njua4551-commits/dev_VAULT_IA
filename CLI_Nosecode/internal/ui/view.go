package ui

import (
	"fmt"
	"strings"
	"time"

	"nosecode/internal/version"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// tips se muestran bajo el prompt en la pantalla de inicio, como en opencode.
var tips = []string{
	"Pulsa ctrl+p para abrir la paleta de comandos",
	"Usa shift+tab para alternar entre los agentes Plan y Build",
	"Escribe /model para ver o cambiar el modelo activo",
	"ctrl+n crea una sesión nueva; /sessions cambia entre ellas",
	"Escribe /theme para alternar entre vercel y midnight",
	"alt+enter inserta una nueva línea en el prompt",
	"f2 renombra la sesión y ctrl+w la elimina",
	"ctrl+b muestra u oculta la barra lateral",
}

// View implementa tea.Model.
func (m Model) View() string {
	if m.width <= 0 || m.height <= 0 {
		return ""
	}

	var lines []uiLine
	if m.width < 44 || m.height < 14 {
		lines = m.smallWindowScreen()
	} else {
		switch m.screen {
		case screenSettings:
			lines = m.settingsScreen()
		case screenPalette:
			lines = m.paletteScreen()
		case screenRename:
			lines = m.renameScreen()
		case screenConfirm:
			lines = m.confirmScreen()
		case screenTool:
			lines = m.toolScreen()
		default:
			lines = m.mainScreen()
		}
	}

	lines = normalizeScreen(lines, m.width, m.height)
	lines = m.selectionView(lines)
	m.buf.lines = lines
	m.buf.width = m.width
	m.buf.height = m.height
	m.register(lines)

	texts := make([]string, len(lines))
	for i, ln := range lines {
		texts[i] = ln.text
	}
	return strings.Join(texts, "\n")
}

// normalizeScreen ajusta cada línea al ancho y pinta el fondo global (opencode
// pinta todo el lienzo, no solo las zonas con texto).
func normalizeScreen(lines []uiLine, w, h int) []uiLine {
	out := make([]uiLine, 0, h)
	blank := uiLine{text: paintWith(appStyle, spaces(w))}
	for i := 0; i < h; i++ {
		if i < len(lines) {
			ln := lines[i]
			ln.text = paintWith(appStyle, pad(ln.text, w))
			out = append(out, ln)
		} else {
			out = append(out, blank)
		}
	}
	return out
}

func (m Model) smallWindowScreen() []uiLine {
	lines := make([]uiLine, 0, m.height)
	top := m.height/2 - 2
	if top < 0 {
		top = 0
	}
	for i := 0; i < top; i++ {
		lines = append(lines, blankLine(m.width))
	}
	add := func(text string) {
		lines = append(lines, uiLine{text: appStyle.Width(m.width).Render(center(text, m.width))})
	}
	add(titleStyle.Render("Ventana demasiado pequeña"))
	add("")
	add(mutedStyle.Render("Amplía el terminal para continuar"))
	add(faintStyle.Render("ctrl+0 restablece el zoom del terminal"))
	for len(lines) < m.height {
		lines = append(lines, blankLine(m.width))
	}
	return lines
}

func (m Model) mainScreen() []uiLine {
	if len(m.msgs) == 0 {
		return m.homeScreen()
	}
	return m.sessionScreen()
}

// shiftLine desplaza una línea x columnas (texto y zonas clicables).
func shiftLine(ln uiLine, x int) uiLine {
	if x <= 0 {
		return ln
	}
	out := ln
	out.text = spaces(x) + ln.text
	if ln.selectable {
		out.selStart += x
		out.selEnd += x
	}
	out.panelX0 += x
	out.panelX1 += x
	out.hits = nil
	for _, h := range ln.hits {
		h.x += x
		out.hits = append(out.hits, h)
	}
	return out
}

// keyHint pinta «tecla etiqueta» como los atajos de opencode.
func keyHint(key, label string) string {
	return textStyle.Render(key) + " " + mutedStyle.Render(label)
}

// ---------------------------------------------------------------- home ---

func (m Model) homeScreen() []uiLine {
	tabs := m.tabBarLines(m.width)
	usable := m.height - statusHeight - len(tabs)
	content := []uiLine{}

	for _, l := range logoRows() {
		content = append(content, uiLine{text: center(l, m.width)})
	}
	content = append(content, blankLine(m.width))

	boxW := m.inputBoxWidth()
	boxX := (m.width - boxW) / 2
	for _, ln := range m.slashMenuLines(boxW) {
		content = append(content, shiftLine(ln, boxX))
	}
	for _, ln := range m.promptBox(boxW) {
		content = append(content, shiftLine(ln, boxX))
	}

	hints := keyHint("shift+tab", "agentes") + "   " + keyHint("ctrl+p", "comandos")
	hx := boxX + boxW - lipgloss.Width(hints)
	if hx < 0 {
		hx = 0
	}
	content = append(content, uiLine{text: spaces(hx) + hints})

	if usable >= 20 {
		content = append(content, blankLine(m.width))
		tip := warningStyle.Render("● Tip ") + mutedStyle.Render(fit(tips[m.tip%len(tips)], m.width-12))
		content = append(content, uiLine{text: center(tip, m.width)})
	}

	top := (usable - len(content)) / 2
	if top < 0 {
		top = 0
	}

	lines := make([]uiLine, 0, m.height)
	lines = append(lines, tabs...)
	for i := 0; i < top; i++ {
		lines = append(lines, blankLine(m.width))
	}
	lines = append(lines, content...)
	for len(lines) < m.height-statusHeight {
		lines = append(lines, blankLine(m.width))
	}
	lines = append(lines, m.statusLine())
	return lines
}

// promptBox dibuja el prompt como opencode: barra izquierda «┃» con el color
// del agente, fondo de elemento, línea de agente/modelo/proveedor y un borde
// inferior de medio bloque. Alto fijo: inputHeight líneas.
func (m Model) promptBox(width int) []uiLine {
	agent := m.agentColor()
	bar := lipgloss.NewStyle().Foreground(agent).Render("┃")
	inner := width - 1
	row := func(content string) string {
		return paint(colElement, bar+pad(content, inner))
	}

	lines := []uiLine{{text: row("")}}

	text := strings.Split(stripBg(m.input.View()), "\n")
	for i := 0; i < 2; i++ {
		r := ""
		if i < len(text) {
			r = text[i]
		}
		lines = append(lines, uiLine{text: row("  " + pad(r, inner-4))})
	}
	lines = append(lines, uiLine{text: row("")})

	// agente · modelo · proveedor
	name, p := m.cfg.Active()
	label := lipgloss.NewStyle().Foreground(agent).Bold(true).Render(modeLabel(m.mode))
	lw := lipgloss.Width(label)
	nameW := lipgloss.Width(name)
	if nameW > 16 {
		nameW = 16
	}
	modelW := inner - 4 - lw - 4 - nameW
	if modelW < 6 {
		modelW = 6
	}
	modelTxt := fit(valueOr(p.Model, "sin modelo"), modelW)
	nameTxt := fit(name, nameW)
	meta := uiLine{text: row("  " + label + "  " + textStyle.Render(modelTxt) + "  " + mutedStyle.Render(nameTxt))}
	x := 3
	meta.hits = append(meta.hits, relHit{x: x, w: lw, action: "mode"})
	x += lw + 2
	mw := lipgloss.Width(modelTxt) + 2 + lipgloss.Width(nameTxt)
	meta.hits = append(meta.hits, relHit{x: x, w: mw, action: "settings", arg: "0"})
	lines = append(lines, meta)

	edge := lipgloss.NewStyle().Foreground(agent).Render("╹") +
		lipgloss.NewStyle().Foreground(colElement).Render(strings.Repeat("▀", inner))
	lines = append(lines, uiLine{text: edge})

	// el clic en cualquier parte del prompt enfoca la entrada
	for i := range lines {
		if i == len(lines)-1 {
			break
		}
		lines[i].hits = append(lines[i].hits, relHit{x: 0, w: width, action: "focusInput"})
	}
	return lines
}

// ------------------------------------------------------------- sesion ---

func (m Model) sessionScreen() []uiLine {
	bodyRows := m.height - statusHeight

	mainW := m.width
	if m.sidebarVisible() {
		mainW = m.width - railWidth
	}

	head := m.headerLines(mainW)
	tabs := m.tabBarLines(mainW)
	boxW := m.inputBoxWidth()
	boxX := (mainW - boxW) / 2
	box := m.promptBox(boxW)
	menu := m.slashMenuLines(boxW)
	if maxMenu := bodyRows - len(tabs) - len(head) - len(box) - 4; len(menu) > maxMenu {
		if maxMenu < 0 {
			maxMenu = 0
		}
		menu = menu[:maxMenu]
	}

	chatH := bodyRows - len(tabs) - len(head) - len(box) - len(menu)
	if chatH < 4 {
		chatH = 4
	}

	main := make([]uiLine, 0, bodyRows)
	main = append(main, tabs...)
	main = append(main, head...)
	main = append(main, m.chatLines(chatH, mainW)...)
	for _, ln := range menu {
		main = append(main, shiftLine(ln, boxX))
	}
	for _, ln := range box {
		main = append(main, shiftLine(ln, boxX))
	}

	lines := make([]uiLine, 0, m.height)
	if m.sidebarVisible() {
		rail := m.railLines(len(main))
		for i, ln := range main {
			row := ln
			row.text = pad(ln.text, mainW) + rail[i].text
			for _, h := range rail[i].hits {
				h.x += mainW
				row.hits = append(row.hits, h)
			}
			lines = append(lines, row)
		}
	} else {
		lines = append(lines, main...)
	}

	lines = append(lines, m.statusLine())
	return lines
}

// tabBarLines dibuja las sesiones como pestañas de navegador.
func (m Model) tabBarLines(width int) []uiLine {
	if len(m.sessions) == 0 || width < 24 {
		return nil
	}
	const tabW = 18
	maxTabs := (width - 4) / tabW
	if maxTabs < 1 {
		maxTabs = 1
	}
	start, end := 0, len(m.sessions)
	if len(m.sessions) > maxTabs {
		active := m.activeIndex()
		if active < 0 {
			active = 0
		}
		start = clamp(active-maxTabs/2, 0, len(m.sessions)-maxTabs)
		end = start + maxTabs
	}

	line := uiLine{}
	x := 0
	activeStyle := lipgloss.NewStyle().Background(colElement).Foreground(colPrimary).Bold(true)
	idleStyle := lipgloss.NewStyle().Foreground(colMuted)
	for _, s := range m.sessions[start:end] {
		active := s.ID == m.activeID
		title := fit(s.Title, tabW-3)
		label := " " + title + " "
		style := idleStyle
		if active {
			style = activeStyle
		} else if m.isHover("sessionSwitch", s.ID) {
			style = rowSelectedStyle
		}
		mark := " "
		if active {
			mark = "×"
		}
		line.text += style.Render(label) + faintStyle.Render(mark)
		if active {
			line.hits = append(line.hits,
				relHit{x: x, w: tabW - 1, action: "sessionSwitch", arg: s.ID},
				relHit{x: x + tabW - 1, w: 1, action: "sessionClose", arg: s.ID},
			)
		} else {
			line.hits = append(line.hits,
				relHit{x: x, w: tabW, action: "sessionSwitch", arg: s.ID},
			)
		}
		x += tabW
	}
	if start > 0 {
		line.text = faintStyle.Render("…") + line.text
		for i := range line.hits {
			line.hits[i].x++
		}
		x++
	}
	if end < len(m.sessions) {
		line.text += faintStyle.Render("…")
		x++
	}
	if x+3 <= width {
		line.text += lipgloss.NewStyle().Background(colElement).Foreground(colMuted).Render(" + ")
		line.hits = append(line.hits, relHit{x: x, w: 3, action: "new"})
	}
	return []uiLine{line}
}

// headerLines es la cabecera de sesión de opencode: panel con «# título» y,
// sin barra lateral, el contador de contexto a la derecha.
func (m Model) headerLines(mainW int) []uiLine {
	w := mainW - 4
	if w < 20 {
		w = 20
	}
	cw := w - 5
	bar := lipgloss.NewStyle().Foreground(colBorderActive).Render("┃")
	panelRow := func(content string) string {
		return spaces(2) + paint(colPanel, bar+"  "+pad(content, cw)+"  ")
	}

	right := ""
	if !m.sidebarVisible() && m.cfg.UI.Tokens {
		right = mutedStyle.Render(m.contextLabel())
	}
	titleW := cw - lipgloss.Width(right) - 2
	if titleW < 8 {
		titleW = 8
	}
	left := titleStyle.Render("# " + fit(m.activeTitle(), titleW))
	gap := cw - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	content := uiLine{
		text: panelRow(left + spaces(gap) + right),
		hits: []relHit{{x: 2, w: w, action: "sessionRename", arg: m.activeID}},
	}

	lines := []uiLine{}
	if m.height >= 30 {
		lines = append(lines, uiLine{text: panelRow("")})
	}
	lines = append(lines, content)
	if m.height >= 30 {
		lines = append(lines, uiLine{text: panelRow("")})
	}
	return lines
}

// contextLabel resume el uso de contexto: «~33 tok · 1%».
func (m Model) contextLabel() string {
	_, p := m.cfg.Active()
	tokens := m.tokensValue()
	if p.ContextWindow > 0 {
		return fmt.Sprintf("~%d tok · %d%%", tokens, tokens*100/p.ContextWindow)
	}
	return fmt.Sprintf("~%d tok", tokens)
}

// railLines es la barra lateral derecha de opencode: panel con el título,
// el contexto, el proveedor y la lista de sesiones.
func (m Model) railLines(height int) []uiLine {
	name, p := m.cfg.Active()
	cw := railWidth - 4

	rows := []uiLine{}
	add := func(content string, hits ...relHit) {
		rows = append(rows, uiLine{
			text: paint(colPanel, "  "+pad(content, cw)+"  "),
			hits: hits,
		})
	}
	whole := func(action, arg string) relHit { return relHit{x: 2, w: cw, action: action, arg: arg} }

	add("")
	add(titleStyle.Render(fit(m.activeTitle(), cw)), whole("sessionRename", m.activeID))
	add("")

	if m.cfg.UI.Tokens {
		add(titleStyle.Render("Contexto"))
		add(mutedStyle.Render(fmt.Sprintf("%d tokens", m.tokensValue())))
		if p.ContextWindow > 0 {
			add(mutedStyle.Render(fmt.Sprintf("%d%% usado", m.tokensValue()*100/p.ContextWindow)))
		}
		add("")
	}

	dot := warningStyle.Render("●")
	state := "sin endpoint"
	if p.BaseURL != "" && p.APIKey != "" {
		dot = successStyle.Render("●")
		state = "configurado"
	}
	add(titleStyle.Render("Proveedor"), whole("settings", "0"))
	add(dot+" "+textStyle.Render(fit(name, cw-4-lipgloss.Width(state)))+" "+mutedStyle.Render(state), whole("settings", "0"))
	add(mutedStyle.Render(fit(valueOr(p.Model, "sin modelo"), cw)), whole("settings", "0"))
	add("")

	if name == "vault" {
		add(titleStyle.Render("Kaggle GPU"))
		add(m.quotaLabel())
		if m.quota.ok {
			add(mutedStyle.Render(fmt.Sprintf("de %s h · reset semanal", formatQuotaHours(m.quota.q.TotalHours))))
		}
		add("")
	}

	plusStyle := mutedStyle
	if m.isHover("new", "") {
		plusStyle = primaryStyle
	}
	heading := titleStyle.Render("Sesiones")
	add(heading+spaces(cw-lipgloss.Width(heading)-1)+plusStyle.Render("+"),
		relHit{x: 2 + cw - 1, w: 1, action: "new"})

	// reserva: separador + cwd + versión + margen
	bottom := 4
	room := height - len(rows) - bottom
	if room < 1 {
		room = 1
	}
	start, end := 0, len(m.sessions)
	more := 0
	if len(m.sessions) > room {
		room--
		if room < 1 {
			room = 1
		}
		active := m.activeIndex()
		if active < 0 {
			active = 0
		}
		start = clamp(active-room/2, 0, len(m.sessions)-room)
		end = start + room
		more = len(m.sessions) - room
	}
	for _, s := range m.sessions[start:end] {
		active := s.ID == m.activeID
		marker := "  "
		style := mutedStyle
		if active {
			marker = primaryStyle.Render("● ")
			style = textStyle
		} else if m.isHover("sessionSwitch", s.ID) {
			style = textStyle
		}
		titleW := cw - 2
		if active {
			titleW -= 2
		}
		line := marker + style.Render(fit(s.Title, titleW))
		hits := []relHit{{x: 2, w: cw, action: "sessionSwitch", arg: s.ID}}
		if active {
			closeStyle := faintStyle
			if m.isHover("sessionClose", s.ID) {
				closeStyle = dangerStyle
			}
			gap := cw - lipgloss.Width(line) - 1
			line += spaces(gap) + closeStyle.Render("×")
			hits = append([]relHit{{x: 2 + cw - 1, w: 1, action: "sessionClose", arg: s.ID}}, hits...)
		}
		add(line, hits...)
	}
	if more > 0 {
		add(faintStyle.Render(fmt.Sprintf("… %d más · /sessions", more)))
	}

	cwd, err := osGetwd()
	if err != nil {
		cwd = "."
	}
	footer := []string{
		mutedStyle.Render(fit(shortPath(cwd), cw)),
		successStyle.Render("●") + " " + mutedStyle.Render("nose") + titleStyle.Render("code") + " " + mutedStyle.Render(version.Version),
		"",
	}

	out := make([]uiLine, 0, height)
	fill := height - len(footer)
	for i := 0; i < height; i++ {
		switch {
		case i < len(rows) && i < fill:
			out = append(out, rows[i])
		case i >= fill:
			ln := uiLine{text: paint(colPanel, "  "+pad(footer[i-fill], cw)+"  ")}
			out = append(out, ln)
		default:
			out = append(out, uiLine{text: paint(colPanel, spaces(railWidth))})
		}
	}
	return out
}

func (m Model) chatLines(height, width int) []uiLine {
	content := m.chatContent(width)
	if len(content) == 0 {
		content = []uiLine{blankLine(width)}
	}

	maxScroll := len(content) - height
	if maxScroll < 0 {
		maxScroll = 0
	}
	scroll := clamp(m.scroll, 0, maxScroll)

	end := len(content) - scroll
	start := end - height
	if start < 0 {
		start = 0
	}

	out := make([]uiLine, 0, height)
	for _, ln := range content[start:end] {
		ln.text = pad(ln.text, width)
		out = append(out, ln)
	}
	for len(out) < height {
		out = append(out, uiLine{panel: "chat", panelX0: 0, panelX1: width})
	}
	return out
}

// chatContent compone los mensajes como opencode: el usuario en un panel con
// barra del color del agente; la respuesta, texto plano con sangría y una
// línea «▣ agente · modelo» debajo.
func (m Model) chatContent(width int) []uiLine {
	blockW := width - 4
	if blockW < 24 {
		blockW = 24
	}
	userW := blockW - 5
	botW := width - 7
	if botW < 16 {
		botW = 16
	}

	agent := lipgloss.NewStyle().Foreground(m.agentColor())
	bar := agent.Render("┃")
	blankChat := func() uiLine {
		return uiLine{panel: "chat", panelX0: 0, panelX1: width}
	}
	userRow := func(txt string, msgIdx int, body bool) uiLine {
		ln := uiLine{
			text:     spaces(2) + paint(colPanel, bar+pad("  "+txt, blockW-1)),
			panel:    "chat",
			panelX0:  0,
			panelX1:  width,
			hasMsg:   true,
			msgIndex: msgIdx,
			msgBody:  body,
		}
		if body && lipgloss.Width(txt) > 0 {
			ln.selectable = true
			ln.selStart = 5
			ln.selEnd = 5 + lipgloss.Width(txt)
		}
		return ln
	}
	_, p := m.cfg.Active()
	model := valueOr(p.Model, "sin modelo")

	lines := []uiLine{blankChat()}
	for i, msg := range m.msgs {
		if msg.Role == "assistant" {
			for _, ln := range wrapText(sanitizeText(msg.Text), botW) {
				row := uiLine{
					text:     spaces(5) + textStyle.Render(ln),
					panel:    "chat",
					panelX0:  0,
					panelX1:  width,
					hasMsg:   true,
					msgIndex: i,
					msgBody:  true,
				}
				if lipgloss.Width(ln) > 0 {
					row.selectable = true
					row.selStart = 5
					row.selEnd = 5 + lipgloss.Width(ln)
				}
				lines = append(lines, row)
			}
			lines = append(lines, blankChat())
			head := "▣  nosecode · " + fit(model, 30)
			headLine := uiLine{
				text:       spaces(5) + agent.Render("▣") + "  " + mutedStyle.Render("nosecode · "+fit(model, 30)),
				panel:      "chat",
				panelX0:    0,
				panelX1:    width,
				selectable: true,
				selStart:   5,
				selEnd:     5 + lipgloss.Width(head),
				hasMsg:     true,
				msgIndex:   i,
				msgHead:    true,
			}
			lines = append(lines, headLine)
			lines = append(lines, blankChat())
			continue
		}
		lines = append(lines, userRow("", i, false))
		for _, ln := range wrapText(sanitizeText(msg.Text), userW) {
			lines = append(lines, userRow(textStyle.Render(ln), i, true))
		}
		lines = append(lines, userRow("", i, false))
		lines = append(lines, blankChat())
	}
	return lines
}

// --------------------------------------------------------- diálogos ---

func (m Model) renameScreen() []uiLine {
	boxW := clamp(m.width-12, 40, 68)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4

	title := modalTitleStyle.Render("Renombrar sesión")
	right := faintStyle.Render("esc cancelar")
	head := []uiLine{
		{text: title + spaces(maxInt(0, innerW-lipgloss.Width(title)-lipgloss.Width(right))) + right},
	}
	body := []uiLine{
		{text: faintStyle.Render("Nombre de la sesión")},
		{text: " " + m.rename.input.View()},
		uiLine{},
		m.buttonRow(
			uiButton{"Guardar", "renameSave", "", true},
			uiButton{"Cancelar", "renameCancel", "", false},
		),
	}
	box := m.modalFrame(boxW, head, body, "enter guardar · esc cancelar")
	return m.centerModal(box, boxW)
}

func (m Model) confirmScreen() []uiLine {
	boxW := clamp(m.width-14, 40, 64)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4

	isDown := m.confirm.action == "vaultDown"
	titleText := "Eliminar sesión"
	body := []uiLine{
		{text: textStyle.Render("Se borrará «" + fit(m.confirm.title, 34) + "»")},
		{text: faintStyle.Render("con todo su historial. No se puede deshacer.")},
		uiLine{},
		m.buttonRow(
			uiButton{"Eliminar", "confirmGo", "", false},
			uiButton{"Cancelar", "confirmCancel", "", true},
		),
	}
	footer := "enter eliminar · esc cancelar"
	if isDown {
		titleText = "Apagar la IA"
		body = []uiLine{
			{text: textStyle.Render("Se detendrá la sesión de Kaggle y dejará de gastar GPU.")},
			{text: faintStyle.Render("Podrás volver a encenderla con /up o F8.")},
			uiLine{},
			m.buttonRow(
				uiButton{"Apagar", "confirmGo", "", false},
				uiButton{"Cancelar", "confirmCancel", "", true},
			),
		}
		footer = "enter apagar · esc cancelar"
	}
	title := modalTitleStyle.Render(titleText)
	right := faintStyle.Render("esc cancelar")
	head := []uiLine{
		{text: title + spaces(maxInt(0, innerW-lipgloss.Width(title)-lipgloss.Width(right))) + right},
	}
	box := m.modalFrame(boxW, head, body, footer)
	return m.centerModal(box, boxW)
}

func (m Model) statusLine() uiLine {
	cwd, err := osGetwd()
	if err != nil {
		cwd = "."
	}
	name, p := m.cfg.Active()
	dot := warningStyle.Render("●")
	if p.BaseURL != "" && p.APIKey != "" {
		dot = successStyle.Render("●")
	}

	left := ""
	if !m.sidebarVisible() {
		left = " " + mutedStyle.Render(fit(shortPath(cwd), 44))
	}

	parts := []string{}
	if m.cfg.UI.Tokens && !m.sidebarVisible() && len(m.msgs) > 0 {
		parts = append(parts, mutedStyle.Render(m.contextLabel()))
	}
	if name == "vault" {
		parts = append(parts, m.quotaLabel())
	}
	parts = append(parts,
		keyHint("ctrl+p", "comandos"),
		dot+" "+mutedStyle.Render(fit(name, 14)),
		faintStyle.Render("v"+version.Version),
	)
	right := strings.Join(parts, "  ")
	for lipgloss.Width(right) > m.width-12 && len(parts) > 1 {
		parts = parts[1:]
		right = strings.Join(parts, "  ")
	}

	avail := m.width - lipgloss.Width(left) - lipgloss.Width(right) - 6
	mid := ""
	if text := m.activityText(); text != "" {
		mid = primaryStyle.Render(fit(text, clamp(avail, 8, 46)))
	} else if m.toast != "" {
		style := successStyle
		if m.toastErr {
			style = dangerStyle
		}
		mid = style.Render(fit(m.toast, clamp(avail, 8, 46)))
	}

	content := compose(left, mid, right, m.width-2)
	return uiLine{text: " " + content + " "}
}

func (m Model) activityText() string {
	a := m.activity
	if a.kind == "" {
		return ""
	}
	secs := int(time.Since(a.started).Seconds())
	if secs < 0 {
		secs = 0
	}
	switch a.kind {
	case "model":
		return fmt.Sprintf("esperando al modelo… %d s", secs)
	case "stream":
		return fmt.Sprintf("recibiendo respuesta… %d s", secs)
	case "tool":
		return fmt.Sprintf("ejecutando %s… %d s", a.label, secs)
	case "image":
		return fmt.Sprintf("procesando imagen… %d s", secs)
	default:
		return a.label
	}
}

// -------------------------------------------------------------- utils ---

func blankLine(width int) uiLine {
	return uiLine{text: appStyle.Width(width).Render("")}
}

func compose(left, mid, right string, width int) string {
	if width < 1 {
		return ""
	}
	if lipgloss.Width(right) > width {
		right = ansi.Truncate(right, width, "…")
	}
	for lipgloss.Width(left)+lipgloss.Width(mid)+lipgloss.Width(right)+2 > width && mid != "" {
		mid = ""
	}
	for lipgloss.Width(left)+lipgloss.Width(right)+1 > width && lipgloss.Width(left) > 0 {
		left = ansi.Truncate(left, lipgloss.Width(left)-1, "")
	}
	gap := width - lipgloss.Width(left) - lipgloss.Width(mid) - lipgloss.Width(right)
	if gap < 0 {
		gap = 0
	}
	if mid != "" {
		half := gap / 2
		return left + spaces(half) + mid + spaces(gap-half) + right
	}
	if gap < 1 && lipgloss.Width(left)+lipgloss.Width(right) < width {
		gap = 1
	}
	return left + spaces(gap) + right
}

func fit(s string, w int) string {
	if w <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w <= 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}
