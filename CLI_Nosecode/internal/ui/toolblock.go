package ui

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/session"
	"nosecode/internal/tools"

	"github.com/charmbracelet/lipgloss"
)

// Estados visibles de un bloque de herramienta.
const (
	toolWaiting     = "waiting"
	toolApproval    = "approval"
	toolRunning     = "running"
	toolOK          = "ok"
	toolError       = "error"
	toolDenied      = "denied"
	toolCancelled   = "cancelled"
	toolTimeout     = "timeout"
	toolSoftTimeout = "soft_timeout"
)

// addToolBlocks crea un bloque visible por cada llamada antes de ejecutarla.
func (m Model) addToolBlocks(calls []api.ToolCall) Model {
	a := m.run.agent
	if a == nil {
		return m
	}
	if a.toolIdx == nil {
		a.toolIdx = map[string]int{}
	}
	for _, call := range calls {
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_block_%d", len(m.msgs))
		}
		if _, ok := a.toolIdx[call.ID]; ok {
			continue
		}
		status := toolWaiting
		if m.callNeedsApproval(call) {
			status = toolApproval
		}
		block := &session.ToolBlock{
			CallID: call.ID,
			Name:   shortTool(call.Function.Name),
			Args:   toolArgsDisplay(call),
			Status: status,
		}
		if _, ok := bashCommand(call); ok {
			block.TimeoutS = bashTimeoutS(call)
		}
		m.msgs = append(m.msgs, Message{Role: "assistant", Tool: block})
		a.toolIdx[call.ID] = len(m.msgs) - 1
	}
	m.scroll = 0
	m.layout()
	return m
}

// setToolBlock actualiza el bloque de una llamada. Si ya no está en memoria,
// busca por CallID para no perder sesiones restauradas.
func (m Model) setToolBlock(callID string, fn func(*session.ToolBlock)) Model {
	if callID == "" {
		return m
	}
	idx := -1
	if a := m.run.agent; a != nil && a.toolIdx != nil {
		idx = a.toolIdx[callID]
	}
	if idx < 0 || idx >= len(m.msgs) || m.msgs[idx].Tool == nil {
		idx = -1
		for i := len(m.msgs) - 1; i >= 0; i-- {
			if m.msgs[i].Tool != nil && m.msgs[i].Tool.CallID == callID {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		return m
	}
	fn(m.msgs[idx].Tool)
	return m
}

// hasToolBlock indica si una llamada tiene bloque visible en el chat.
func (m Model) hasToolBlock(callID string) bool {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		if m.msgs[i].Tool != nil && m.msgs[i].Tool.CallID == callID {
			return true
		}
	}
	return false
}

// activeToolBlock devuelve el bloque que se está ejecutando.
func (m Model) activeToolBlock() *session.ToolBlock {
	a := m.run.agent
	if a == nil || a.activeCallID == "" {
		return nil
	}
	for i := len(m.msgs) - 1; i >= 0; i-- {
		if m.msgs[i].Tool != nil && m.msgs[i].Tool.CallID == a.activeCallID {
			return m.msgs[i].Tool
		}
	}
	return nil
}

// callNeedsApproval decide si una sola llamada debe pasar por el modal.
func (m Model) callNeedsApproval(call api.ToolCall) bool {
	if cmd, ok := bashCommand(call); ok && tools.UsesSudo(cmd) && !m.planDenied(call.Function.Name) {
		if d, _ := m.bashDecision(call); d != "deny" {
			return true // sudo siempre pide permiso, aunque bash esté en "permitir"
		}
	}
	if m.planDenied(call.Function.Name) {
		return false // se denegará al ejecutar
	}
	if shortTool(call.Function.Name) == "question" {
		return false
	}
	if call.Function.Name == "terminal" && terminalAction(call) != "open" {
		return false // la sesión ya se aprobó al abrirla
	}
	if _, isBash := bashCommand(call); isBash {
		switch d, _ := m.bashDecision(call); d {
		case "allow", "deny":
			return false
		}
	}
	if cat, ok := approvalCategory(call.Function.Name); ok && cat == "mcp" {
		switch m.mcpServerPerm(call.Function.Name) {
		case "allow":
			return false
		case "ask":
			return true
		}
	}
	if cat, ok := approvalCategory(call.Function.Name); ok && !m.approvalAllowed(cat) {
		return true
	}
	return m.readNeedsApproval(call)
}

// toolArgsDisplay resume el argumento principal de la herramienta.
func toolArgsDisplay(call api.ToolCall) string {
	var a map[string]any
	_ = json.Unmarshal([]byte(call.Function.Arguments), &a)
	str := func(key string) string {
		v, _ := a[key].(string)
		return strings.TrimSpace(v)
	}
	switch shortTool(call.Function.Name) {
	case "bash":
		if cmd := str("command"); cmd != "" {
			return "$ " + cmd
		}
	case "terminal":
		act := str("action")
		if act == "" {
			act = "open"
		}
		switch act {
		case "open":
			if c := str("command"); c != "" {
				return "terminal ▸ " + c
			}
			return "terminal ▸ shell"
		case "send":
			return fmt.Sprintf("terminal %v ◂ %s %s", a["id"], strings.TrimSpace(str("text")), strings.Join(anyStrings(a["keys"]), " "))
		}
		return fmt.Sprintf("terminal %s %v", act, a["id"])
	case "read_file", "write_file", "edit_file", "list_dir", "view_image":
		if path := str("path"); path != "" {
			return path
		}
	case "grep":
		pattern, path := str("pattern"), str("path")
		if path != "" {
			pattern += "  en " + path
		}
		return pattern
	case "web_fetch":
		return str("url")
	case "screenshot":
		return ""
	}
	for _, key := range []string{"path", "url", "command", "query", "pattern", "name"} {
		if v := str(key); v != "" {
			return v
		}
	}
	return oneLine(prettyArgs(call.Function.Arguments), 160)
}

func formatToolDuration(ms int64) string {
	s := float64(ms) / 1000
	text := fmt.Sprintf("%.1f s", s)
	if s < 0.05 {
		text = "0,0 s"
	}
	return strings.Replace(text, ".", ",", 1)
}

func countTextLines(s string) int {
	s = strings.TrimRight(s, "\n")
	if strings.TrimSpace(s) == "" {
		return 0
	}
	return strings.Count(s, "\n") + 1
}

// toolStatusLine pinta el estado actual del bloque.
func toolStatusLine(b *session.ToolBlock, now time.Time) string {
	switch b.Status {
	case toolWaiting:
		return mutedStyle.Render("· en espera")
	case toolApproval:
		return warningStyle.Render("⏸ esperando tu aprobación")
	case toolRunning:
		secs := int(now.Sub(b.Started).Seconds())
		if secs < 0 {
			secs = 0
		}
		text := fmt.Sprintf("⏳ ejecutando %d s", secs)
		if b.TimeoutS > 0 {
			text = fmt.Sprintf("⏳ ejecutando %d s / %d s", secs, b.TimeoutS)
		}
		return primaryStyle.Render(text)
	case toolOK:
		text := "✓ ok " + formatToolDuration(b.DurationMS)
		if lines := countTextLines(b.Output); lines > 0 {
			text += fmt.Sprintf(" · %d líneas", lines)
		}
		return successStyle.Render(text)
	case toolError:
		if b.ExitCode > 0 {
			return dangerStyle.Render(fmt.Sprintf("✗ error (código %d)", b.ExitCode))
		}
		return dangerStyle.Render("✗ error")
	case toolDenied:
		return dangerStyle.Render("⛔ denegado")
	case toolCancelled:
		return mutedStyle.Render("⏹ cancelado")
	case toolTimeout:
		return warningStyle.Render(fmt.Sprintf("⌛ timeout %d s", b.TimeoutS))
	case toolSoftTimeout:
		return warningStyle.Render(fmt.Sprintf("⌛ %d s · esperando tu decisión", b.TimeoutS))
	default:
		return mutedStyle.Render(b.Status)
	}
}

// toolBlockLines convierte un bloque en filas de chat con vista previa.
func (m Model) toolBlockLines(msgIdx int, b *session.ToolBlock, width int) []uiLine {
	indent := 5
	avail := width - indent - 4
	if avail < 16 {
		avail = 16
	}
	blank := uiLine{panel: "chat", panelX0: 0, panelX1: width}
	var rowS func(text string, style lipgloss.Style) uiLine
	row := func(text string, faint bool) uiLine {
		style := textStyle
		if faint {
			style = faintStyle
		}
		return rowS(text, style)
	}
	rowS = func(text string, style lipgloss.Style) uiLine {
		ln := uiLine{
			text:     spaces(indent) + style.Render(text),
			panel:    "chat",
			panelX0:  0,
			panelX1:  m.width,
			hasMsg:   true,
			msgIndex: msgIdx,
			msgBody:  true,
		}
		if lipgloss.Width(text) > 0 {
			ln.selectable = true
			ln.selStart = indent
			ln.selEnd = indent + lipgloss.Width(text)
		}
		return ln
	}

	if shortTool(b.Name) == "question" {
		return m.questionBlockLines(b, avail, blank, rowS)
	}
	lines := []uiLine{blank}
	icon := "⚙ "
	name := b.Name
	args := strings.TrimSpace(sanitizeText(b.Args))
	argLines := []string{}
	if args != "" {
		argLines = wrapText(args, avail-lipgloss.Width(name)-4)
		if len(argLines) > 2 {
			argLines = argLines[:2]
			argLines[1] = fit(argLines[1], maxInt(4, lipgloss.Width(argLines[1])-1)) + "…"
		}
	}
	head := icon + name
	if len(argLines) > 0 {
		head += "  " + argLines[0]
	}
	headLine := uiLine{
		text: spaces(indent) + primaryStyle.Render(icon+name) + func() string {
			if len(argLines) == 0 {
				return ""
			}
			return "  " + textStyle.Render(argLines[0])
		}(),
		panel:    "chat",
		panelX0:  0,
		panelX1:  m.width,
		hasMsg:   true,
		msgIndex: msgIdx,
		msgBody:  true,
		hits:     []relHit{{x: indent, w: width - indent, action: "toolToggle", arg: b.CallID}},
	}
	if len(argLines) > 0 {
		headLine.selectable = true
		headLine.selStart = indent
		headLine.selEnd = indent + lipgloss.Width(head)
	}
	lines = append(lines, headLine)
	for _, extra := range argLines[1:] {
		lines = append(lines, row("        "+extra, false))
	}
	lines = append(lines, row("  "+toolStatusLine(b, time.Now()), true))

	if b.Diff != "" && b.Status == toolOK {
		return append(lines, append(m.diffRows(b, avail, rowS, row), blank)...)
	}
	out := strings.TrimRight(sanitizeText(b.Output), "\n")
	if out != "" {
		outLines := strings.Split(out, "\n")
		limit := 5
		if b.Expanded {
			limit = len(outLines)
			if limit > 200 {
				limit = 200
			}
		}
		if limit > len(outLines) {
			limit = len(outLines)
		}
		for _, ln := range outLines[:limit] {
			for _, wrapped := range wrapText(ln, avail-2) {
				lines = append(lines, row("  "+wrapped, true))
			}
		}
		if len(outLines) > limit {
			hint := fmt.Sprintf("… +%d líneas · ctrl+o", len(outLines)-limit)
			if b.Expanded {
				hint = "ctrl+o colapsa"
			}
			lines = append(lines, row("  "+hint, true))
		} else if b.Expanded && len(outLines) > 5 {
			lines = append(lines, row("  ctrl+o colapsa", true))
		}
	}
	lines = append(lines, blank)
	return lines
}

// toggleToolBlock alterna la salida de un bloque; sin CallID usa el último.
func (m Model) toggleToolBlock(callID string) Model {
	idx := -1
	if callID != "" {
		for i := len(m.msgs) - 1; i >= 0; i-- {
			if m.msgs[i].Tool != nil && m.msgs[i].Tool.CallID == callID {
				idx = i
				break
			}
		}
	} else {
		for i := len(m.msgs) - 1; i >= 0; i-- {
			if m.msgs[i].Tool != nil {
				idx = i
				break
			}
		}
	}
	if idx < 0 {
		m.toast, m.toastErr = "No hay bloques de herramienta", false
		return m
	}
	m.msgs[idx].Tool.Expanded = !m.msgs[idx].Tool.Expanded
	m.scroll = 0
	m.layout()
	return m
}

// diffRows dibuja el diff de write_file/edit_file: verde añadido, rojo quitado.
func (m Model) diffRows(b *session.ToolBlock, avail int, rowS func(string, lipgloss.Style) uiLine, row func(string, bool) uiLine) []uiLine {
	all := strings.Split(strings.TrimRight(sanitizeText(b.Diff), "\n"), "\n")
	if len(all) == 0 {
		return nil
	}
	var out []uiLine
	summary := all[0]
	out = append(out, rowS("  cambios "+summary, successStyle))
	body := all[1:]
	limit := 12
	if b.Expanded {
		limit = 200
	}
	if limit > len(body) {
		limit = len(body)
	}
	for _, ln := range body[:limit] {
		style := faintStyle
		switch {
		case strings.HasPrefix(ln, "+ "), ln == "+":
			style = successStyle
		case strings.HasPrefix(ln, "- "), ln == "-":
			style = dangerStyle
		case strings.HasPrefix(ln, "@@"):
			style = faintStyle
		default:
			style = textStyle
		}
		for _, w := range wrapText(ln, avail-2) {
			out = append(out, rowS("  "+w, style))
		}
	}
	if len(body) > limit {
		out = append(out, row(fmt.Sprintf("  … +%d líneas · ctrl+o", len(body)-limit), true))
	} else if b.Expanded && len(body) > 12 {
		out = append(out, row("  ctrl+o colapsa", true))
	}
	return out
}

// questionBlockLines dibuja la herramienta question como texto legible
// (pregunta, opciones numeradas y respuesta) en lugar de JSON crudo.
// Los bloques ``` del texto se muestran con una barra lateral.
func (m Model) questionBlockLines(b *session.ToolBlock, avail int, blank uiLine, rowS func(string, lipgloss.Style) uiLine) []uiLine {
	text, opts, err := tools.ParseQuestionArgs(json.RawMessage(b.Args))
	lines := []uiLine{blank}
	if err != nil {
		lines = append(lines, rowS("❓ pregunta no válida", mutedStyle))
		return append(lines, blank)
	}
	lines = append(lines, rowS("❓ Pregunta", primaryStyle))
	inCode := false
	for _, ln := range strings.Split(strings.TrimRight(sanitizeText(text), "\n"), "\n") {
		if strings.HasPrefix(strings.TrimSpace(ln), "```") {
			inCode = !inCode
			continue
		}
		if inCode {
			for _, w := range wrapText(ln, avail-4) {
				lines = append(lines, rowS("  │ "+w, faintStyle))
			}
			continue
		}
		for _, w := range wrapText(ln, avail-2) {
			lines = append(lines, rowS("  "+w, textStyle))
		}
	}
	for i, o := range opts {
		for j, w := range wrapText(o, avail-8) {
			pre := "     "
			if j == 0 {
				pre = fmt.Sprintf("  %d) ", i+1)
			}
			lines = append(lines, rowS(pre+w, textStyle))
		}
	}
	switch b.Status {
	case toolOK:
		ans := strings.TrimPrefix(strings.TrimSpace(sanitizeText(b.Output)), "El usuario respondió:")
		for j, w := range wrapText(strings.TrimSpace(ans), avail-6) {
			pre := "     "
			if j == 0 {
				pre = "  → "
			}
			lines = append(lines, rowS(pre+w, primaryStyle))
		}
	case toolApproval:
		lines = append(lines, rowS("  esperando tu respuesta…", faintStyle))
	case toolDenied:
		lines = append(lines, rowS("  sin respuesta", faintStyle))
	}
	return append(lines, blank)
}

func terminalAction(call api.ToolCall) string {
	var a struct {
		Action  string `json:"action"`
		Command string `json:"command"`
	}
	_ = json.Unmarshal([]byte(call.Function.Arguments), &a)
	if a.Action == "" && a.Command != "" {
		return "open"
	}
	return strings.ToLower(a.Action)
}

func anyStrings(v any) []string {
	var out []string
	if l, ok := v.([]any); ok {
		for _, x := range l {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
	}
	return out
}
