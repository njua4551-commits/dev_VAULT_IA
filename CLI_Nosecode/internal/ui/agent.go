package ui

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/browser"
	"nosecode/internal/config"
	"nosecode/internal/debuglog"
	"nosecode/internal/images"
	"nosecode/internal/mcp"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	toolTag       = "Herramienta · "
	maxAgentSteps = 12
)

// toolFailureGuard corta las llamadas repetidas a una herramienta que falla
// dos veces seguidas con el mismo error.
type toolFailureGuard struct {
	lastWire string
	lastErr  string
	count    int
	blocked  map[string]string
}

func (g *toolFailureGuard) fail(wire, errText string) bool {
	key := errorKey(errText)
	if g.lastWire == wire && g.lastErr == key {
		g.count++
	} else {
		g.lastWire, g.lastErr, g.count = wire, key, 1
	}
	if g.count < 2 {
		return false
	}
	if g.blocked == nil {
		g.blocked = map[string]string{}
	}
	g.blocked[wire] = key
	return true
}

func (g *toolFailureGuard) success(wire string) {
	if g.lastWire == wire {
		g.lastWire, g.lastErr, g.count = "", "", 0
	}
}

func (g *toolFailureGuard) blockedReason(wire string) (string, bool) {
	if g == nil {
		return "", false
	}
	reason, ok := g.blocked[wire]
	return reason, ok
}

func errorKey(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 180 {
		s = s[:180] + "…"
	}
	return s
}

func (a *agentRun) beginStep() bool {
	if a.steps >= maxAgentSteps {
		return false
	}
	a.steps++
	return true
}

const mcpConnectingText = "⏳ Conectando herramientas MCP…"

const agentSystem = "Eres Nosecode, un agente de terminal con herramientas MCP para observar y " +
	"controlar la máquina del usuario. Nunca digas que no tienes acceso a la pantalla: usa las " +
	"herramientas. Reglas:\n" +
	"1. Para saber qué ventanas hay, llama a una herramienta list_windows. No inventes nombres ni " +
	"afirmes que una aplicación está abierta sin el resultado de la herramienta.\n" +
	"2. Para abrir una aplicación en Windows: press_keys(keys=\"win+r\"), type_text(text=\"calc\"), " +
	"press_keys(keys=\"enter\") y después list_windows para verificar. Si el usuario pide otra app, " +
	"usa su nombre o ejecutable (notepad, mspaint, explorer...).\n" +
	"3. Para ver la pantalla usa take_screenshot; para hacer clic, antes localiza el punto con una " +
	"captura y usa coordenadas relativas a la ventana.\n" +
	"4. Usa una herramienta solo cuando haga falta, con argumentos válidos. Espera el resultado real " +
	"antes de continuar. Si falla, explica el error y prueba otra acción.\n" +
	"5. Responde en español, breve y claro. No inventes resultados."

type mcpReadyMsg struct {
	manager *mcp.Manager
	tools   []mcp.Tool
	errs    []string
}

type toolDoneMsg struct {
	call     api.ToolCall
	result   mcp.Result
	err      error
	duration time.Duration
}

func (m Model) useAgent() bool {
	return m.cfg.MCP.Enabled && len(m.cfg.MCP.Servers) > 0 &&
		m.cfg.Permissions.MCP != "deny" && m.mode != "plan"
}

func (m Model) startAgent() (Model, tea.Cmd) {
	if m.mcp == nil {
		if m.mcpLoading {
			m.toast, m.toastErr = "Conectando herramientas MCP… espera un momento", false
			return m, nil
		}
		m.mcpLoading = true
		m.run.agent = &agentRun{connecting: true}
		n := len(m.cfg.MCP.Servers)
		m.msgs = append(m.msgs, Message{Role: "assistant",
			Text: fmt.Sprintf("%s (%d servidores)", mcpConnectingText, n)})
		m.scroll = 0
		m.toast, m.toastErr = "Conectando herramientas MCP… (máx. 8 s por servidor)", false
		debuglog.Logf("mcp connect start servers=%d", n)
		m.layout()
		return m, connectMCPCmd(m.cfg.MCP.Servers)
	}
	if len(m.mcpTools) == 0 {
		return m.startChat()
	}
	return m.beginAgent()
}

func connectMCPCmd(servers map[string]config.MCPServer) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 70*time.Second)
		defer cancel()
		mgr, errs := mcp.ConnectAll(ctx, servers)
		return mcpReadyMsg{manager: mgr, tools: mgr.Tools(), errs: errs}
	}
}

func (m Model) onMCPReady(msg mcpReadyMsg) (tea.Model, tea.Cmd) {
	m.mcpLoading = false
	m = m.clearConnecting()
	if m.mcp != nil && m.mcp != msg.manager {
		m.mcp.Close()
	}
	m.mcp = msg.manager
	m.mcpTools = msg.tools
	m.mcpErrs = msg.errs
	debuglog.Logf("mcp ready tools=%d servers=%d errors=%d", len(msg.tools), mcpServerCount(msg.manager), len(msg.errs))
	if m.run.agent == nil {
		if len(m.mcpTools) > 0 {
			m.toast = mcpReadyToast(msg.tools, msg.errs, msg.manager)
		}
		return m, nil
	}
	m.run.agent = nil
	if len(m.mcpTools) == 0 {
		m.toast, m.toastErr = "MCP sin herramientas; sigo en chat normal", true
		return m.startChat()
	}
	m.run.agent = &agentRun{}
	m.toast = mcpReadyToast(msg.tools, msg.errs, msg.manager)
	return m.beginAgent()
}

func (m Model) clearConnecting() Model {
	if n := len(m.msgs); n > 0 && strings.HasPrefix(m.msgs[n-1].Text, mcpConnectingText) {
		m.msgs = m.msgs[:n-1]
	}
	return m
}

func (m Model) beginAgent() (Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		return m, nil
	}
	a.connecting = false
	if a.ctx == nil {
		a.ctx, a.cancel = context.WithCancel(context.Background())
	}
	if !a.beginStep() {
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: fmt.Sprintf(
			"⚠ El agente alcanzó el límite de %d pasos por mensaje. Revisa el resultado y continúa con otro mensaje.", maxAgentSteps)})
		return m.finishAgent(), nil
	}
	if len(a.msgs) == 0 {
		history := apiMessages(m.msgs, false)
		a.msgs = append([]api.ChatMessage{{Role: "system", Content: agentSystem}}, history...)
	}
	_, p := m.cfg.Active()
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "default"
	}
	s := api.Stream(a.ctx, api.Request{
		Bases:       api.Bases(p.BaseURL, config.VaultHome()),
		APIKey:      p.APIKey,
		Model:       model,
		Messages:    a.msgs,
		Tools:       m.apiTools(),
		IdleTimeout: 120 * time.Second,
	})
	a.stream = s
	a.assistant = ""
	a.calls = nil
	debuglog.Logf("model request step=%d msgs=%d tools=%d", a.steps, len(a.msgs), len(m.apiTools()))
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: ""})
	var tick tea.Cmd
	m, tick = m.startActivity("model", "")
	m.toast, m.toastErr = "Agente trabajando… (esc cancela)", false
	m.scroll = 0
	m.layout()
	return m, tea.Batch(waitChunk(s), tick)
}

func (m Model) apiTools() []api.ToolDef {
	out := make([]api.ToolDef, 0, len(m.mcpTools))
	for _, t := range m.mcpTools {
		if a := m.run.agent; a != nil {
			if _, blocked := a.failures.blockedReason(t.Wire); blocked {
				continue
			}
		}
		schema := t.Schema
		if len(schema) == 0 {
			schema = json.RawMessage(`{"type":"object","properties":{}}`)
		}
		out = append(out, api.ToolDef{
			Type: "function",
			Function: api.FunctionDef{
				Name:        t.Wire,
				Description: t.Server + ": " + t.Description,
				Parameters:  schema,
			},
		})
	}
	return out
}

func (m Model) onAgentChunk(msg chunkMsg) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if a == nil || msg.s != a.stream || m.lastIdx() < 0 {
		return m, nil
	}
	c := msg.c
	i := m.lastIdx()
	if c.Text != "" {
		a.assistant += c.Text
		m.msgs[i].Text += c.Text
		if m.activity.kind == "model" {
			m.activity.kind = "stream"
		}
		m.scroll = 0
		m.layout()
	}
	if len(c.ToolCalls) > 0 {
		mergeToolCalls(a, c.ToolCalls)
		m.toast = "El agente está preparando una herramienta…"
	}
	if !c.Done {
		return m, waitChunk(msg.s)
	}

	m = m.stopActivity()
	a.stream = nil
	a.msgs = append(a.msgs, api.ChatMessage{
		Role:      "assistant",
		Content:   a.assistant,
		ToolCalls: a.calls,
	})
	assistantText := a.assistant
	a.assistant = ""
	calls := a.calls
	a.calls = nil

	if c.Err != nil {
		debuglog.Logf("model error: %s", oneLine(c.Err.Error(), 200))
		switch {
		case c.Err == api.ErrCanceled:
			m.toast, m.toastErr = "Agente cancelado", false
		case errors.Is(c.Err, api.ErrModelIdle):
			m.toast, m.toastErr = "El modelo no respondió a tiempo", true
			m.msgs[i].Text += "\n\n⚠ " + c.Err.Error()
		default:
			m.toast, m.toastErr = "Error del agente: "+c.Err.Error(), true
			m.msgs[i].Text += "\n\n⚠ " + c.Err.Error()
		}
		return m.finishAgent(), nil
	}

	m = m.adoptBase(c.Base)

	if len(calls) == 0 {
		if strings.TrimSpace(assistantText) == "" {
			m.msgs[i].Text = "(respuesta vacía)"
		}
		return m.finishAgent(), nil
	}
	if strings.TrimSpace(assistantText) == "" {
		m.msgs[i].Text = "⚙ " + toolNames(calls)
	}
	if a.allowSession || m.cfg.Permissions.MCP == "allow" {
		a.pending = calls
		a.pendingIdx = 0
		return m.executeNextTool()
	}
	m.tool = toolConfirmState{calls: calls}
	m.screen = screenTool
	return m, nil
}

func mergeToolCalls(a *agentRun, deltas []api.ToolCallDelta) {
	for _, d := range deltas {
		for len(a.calls) <= d.Index {
			a.calls = append(a.calls, api.ToolCall{Type: "function"})
		}
		call := &a.calls[d.Index]
		if call.Type == "" {
			call.Type = "function"
		}
		if d.ID != "" {
			call.ID = d.ID
		}
		if d.Name != "" {
			switch {
			case call.Function.Name == "":
				call.Function.Name = d.Name
			case call.Function.Name == d.Name:
				// algunos servidores repiten el nombre completo en cada delta
			case strings.HasPrefix(d.Name, call.Function.Name):
				call.Function.Name = d.Name
			default:
				call.Function.Name += d.Name
			}
		}
		call.Function.Arguments += d.Arguments
	}
}

func toolNames(calls []api.ToolCall) string {
	names := make([]string, 0, len(calls))
	for _, c := range calls {
		names = append(names, shortTool(c.Function.Name))
	}
	return strings.Join(names, ", ")
}

func shortTool(wire string) string {
	if i := strings.Index(wire, "__"); i >= 0 && i+2 < len(wire) {
		return wire[i+2:]
	}
	return wire
}

func (m Model) executeNextTool() (Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		return m, nil
	}
	if a.pendingIdx >= len(a.pending) {
		a.pending = nil
		a.pendingIdx = 0
		return m.continueAgent()
	}
	call := a.pending[a.pendingIdx]
	if call.ID == "" {
		call.ID = fmt.Sprintf("call_%d_%d", a.rounds, a.pendingIdx)
		a.pending[a.pendingIdx] = call
	}
	if !a.beginStep() {
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: fmt.Sprintf(
			"⚠ El agente alcanzó el límite de %d pasos por mensaje. Revisa el resultado y continúa con otro mensaje.", maxAgentSteps)})
		return m.finishAgent(), nil
	}
	if reason, blocked := a.failures.blockedReason(call.Function.Name); blocked {
		m = m.appendToolResult(call, "herramienta no disponible, no la reintentes: "+reason, nil, true)
		a.pendingIdx++
		return m.executeNextTool()
	}

	var tick tea.Cmd
	m, tick = m.startActivity("tool", shortTool(call.Function.Name))
	m.toast = "Ejecutando " + shortTool(call.Function.Name) + "…"
	ctx := a.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	mgr := m.mcp
	servers := m.cfg.MCP.Servers
	browserTool := isBrowserTool(call.Function.Name)
	debuglog.Logf("tool start name=%s browser=%v", call.Function.Name, browserTool)
	cmd := func() tea.Msg {
		start := time.Now()
		if mgr == nil {
			return toolDoneMsg{call: call, err: errors.New("MCP no conectado"), duration: time.Since(start)}
		}
		if browserTool {
			if err := browser.EnsureDefault(servers); err != nil {
				return toolDoneMsg{call: call, err: fmt.Errorf("navegador: %w", err), duration: time.Since(start)}
			}
		}
		args := json.RawMessage(call.Function.Arguments)
		res, err := mgr.Call(ctx, call.Function.Name, args)
		return toolDoneMsg{call: call, result: res, err: err, duration: time.Since(start)}
	}
	return m, tea.Batch(cmd, tick)
}

func (m Model) appendToolResult(call api.ToolCall, text string, imgs []api.ContentPart, isErr bool) Model {
	a := m.run.agent
	if a == nil {
		return m
	}
	a.msgs = append(a.msgs, api.ChatMessage{
		Role:       "tool",
		Content:    text,
		ToolCallID: call.ID,
	})
	if len(imgs) > 0 {
		parts := make([]api.ContentPart, 0, len(imgs)+1)
		parts = append(parts, api.ContentPart{
			Type: "text",
			Text: "Imagen devuelta por la herramienta " + shortTool(call.Function.Name) + ".",
		})
		parts = append(parts, imgs...)
		a.msgs = append(a.msgs, api.ChatMessage{Role: "user", Content: parts})
		pruneOldImages(a.msgs, 2)
	}
	label := shortTool(call.Function.Name)
	preview := oneLine(text, 360)
	if isErr {
		preview = "⚠ " + preview
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: toolTag + label + "\n" + preview})
	m.scroll = 0
	m.layout()
	return m
}

// pruneOldImages deja solo las ultimas `keep` imagenes en el historial. Las
// anteriores conservan su texto y un aviso, para no reenviar capturas viejas.
func pruneOldImages(msgs []api.ChatMessage, keep int) {
	seen := 0
	for i := len(msgs) - 1; i >= 0; i-- {
		parts, ok := msgs[i].Content.([]api.ContentPart)
		if !ok {
			continue
		}
		hasImage := false
		for _, p := range parts {
			if p.Type == "image_url" && p.ImageURL != nil {
				hasImage = true
				break
			}
		}
		if !hasImage {
			continue
		}
		seen++
		if seen <= keep {
			continue
		}
		out := make([]api.ContentPart, 0, len(parts))
		for _, p := range parts {
			if p.Type == "image_url" {
				continue
			}
			out = append(out, p)
		}
		out = append(out, api.ContentPart{Type: "text", Text: "[imagen anterior omitida para ahorrar contexto]"})
		msgs[i].Content = out
	}
}

func mcpServerCount(mgr *mcp.Manager) int {
	if mgr == nil {
		return 0
	}
	return len(mgr.Servers())
}

func mcpReadyToast(tools []mcp.Tool, errs []string, mgr *mcp.Manager) string {
	text := fmt.Sprintf("%d herramientas MCP listas · %d servidores", len(tools), mcpServerCount(mgr))
	if len(errs) > 0 {
		text += fmt.Sprintf(" · %d con error", len(errs))
	}
	return text
}

func (m Model) onToolDone(msg toolDoneMsg) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		return m, nil
	}
	m = m.stopActivity()
	text := msg.result.Text
	errText := ""
	if msg.err != nil {
		errText = msg.err.Error()
		text = "Error: " + errText
		msg.result.Error = true
	} else if msg.result.Error {
		errText = text
	}
	var parts []api.ContentPart
	for _, img := range msg.result.Images {
		path, part, err := prepareToolImage(img)
		if err != nil {
			debuglog.Logf("image error name=%s err=%s", msg.call.Function.Name, oneLine(err.Error(), 160))
			text += "\n[imagen no procesada: " + oneLine(err.Error(), 160) + "]"
			continue
		}
		text += "\n[imagen guardada: " + path + "]"
		parts = append(parts, part)
	}
	if strings.TrimSpace(text) == "" {
		text = "(sin contenido)"
	}
	if errText != "" {
		if a.failures.fail(msg.call.Function.Name, errText) {
			text += "\n[herramienta no disponible, no la reintentes]"
			msg.result.Error = true
		}
	} else {
		a.failures.success(msg.call.Function.Name)
	}
	debuglog.Logf("tool done name=%s duration=%s err=%s images=%d",
		msg.call.Function.Name, msg.duration.Round(time.Millisecond), oneLine(errText, 160), len(parts))
	m = m.appendToolResult(msg.call, text, parts, msg.result.Error)
	a.pendingIdx++
	return m.executeNextTool()
}

func isBrowserTool(wire string) bool {
	server, tool := splitWire(wire)
	return strings.Contains(strings.ToLower(server), "playwright") ||
		strings.HasPrefix(strings.ToLower(tool), "browser_")
}

func (m Model) continueAgent() (Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		return m, nil
	}
	a.rounds++
	return m.beginAgent()
}

func (m Model) finishAgent() Model {
	if a := m.run.agent; a != nil {
		if a.cancel != nil {
			a.cancel()
		}
		debuglog.Logf("turn done steps=%d", a.steps)
	}
	m.run.agent = nil
	m.screen = screenChat
	m = m.stopActivity()
	m.toast, m.toastErr = "", false
	m.sound(soundDone)
	m = m.saveActive()
	m.layout()
	return m
}

func (m Model) toolKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		m.screen = screenChat
		return m, nil
	}
	switch k.String() {
	case "esc":
		return m.cancelBusy()
	case "n":
		return m.denyTools()
	case "enter", "y":
		m.screen = screenChat
		a.pending = m.tool.calls
		a.pendingIdx = 0
		return m.executeNextTool()
	case "a":
		m.screen = screenChat
		a.allowSession = true
		a.pending = m.tool.calls
		a.pendingIdx = 0
		return m.executeNextTool()
	}
	return m, nil
}

func (m Model) denyTools() (Model, tea.Cmd) {
	a := m.run.agent
	m.screen = screenChat
	if a == nil {
		return m, nil
	}
	for _, call := range m.tool.calls {
		if call.ID == "" {
			call.ID = fmt.Sprintf("call_deny_%d", len(a.msgs))
		}
		a.msgs = append(a.msgs, api.ChatMessage{
			Role:       "tool",
			Content:    "El usuario denegó esta acción.",
			ToolCallID: call.ID,
		})
		m.msgs = append(m.msgs, Message{Role: "assistant",
			Text: toolTag + shortTool(call.Function.Name) + "\n(denegado por el usuario)"})
	}
	m.tool = toolConfirmState{}
	m.toast, m.toastErr = "Herramienta denegada", false
	return m.continueAgent()
}

func (m Model) toolScreen() []uiLine {
	boxW := clamp(m.width-12, 52, 88)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4
	if len(m.tool.calls) == 0 {
		return m.centerModal(nil, boxW)
	}
	call := m.tool.calls[0]
	server, tool := splitWire(call.Function.Name)
	head := []uiLine{{
		text: modalTitleStyle.Render("Permiso de herramienta") +
			spaces(maxInt(0, innerW-lipgloss.Width("Permiso de herramienta")-lipgloss.Width("esc cancelar turno"))) +
			faintStyle.Render("esc cancelar turno"),
	}}
	body := []uiLine{
		{text: faintStyle.Render("Servidor   ") + textStyle.Render(server)},
		{text: faintStyle.Render("Herramienta ") + titleStyle.Render(tool)},
		{},
		{text: faintStyle.Render("Argumentos")},
	}
	args := prettyArgs(call.Function.Arguments)
	for _, ln := range wrapText(args, innerW-2) {
		body = append(body, uiLine{text: textStyle.Render(" " + ln)})
	}
	body = append(body,
		uiLine{},
		uiLine{text: primaryStyle.Render("[y] Permitir una vez")},
		uiLine{text: successStyle.Render("[a] Permitir en esta sesión")},
		uiLine{text: dangerStyle.Render("[n] Denegar")},
	)
	// hitboxes de los tres botones
	n := len(body)
	if n >= 3 {
		body[n-3].hits = []relHit{{x: 0, w: innerW, action: "toolAllow"}}
		body[n-2].hits = []relHit{{x: 0, w: innerW, action: "toolAllowSession"}}
		body[n-1].hits = []relHit{{x: 0, w: innerW, action: "toolDeny"}}
	}
	footer := "y permitir · a sesión · n denegar · esc cancelar turno"
	return m.centerModal(m.modalFrame(boxW, head, body, footer), boxW)
}

func splitWire(wire string) (string, string) {
	if i := strings.Index(wire, "__"); i >= 0 {
		return wire[:i], wire[i+2:]
	}
	return "", wire
}

func prettyArgs(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	var v any
	if json.Unmarshal([]byte(raw), &v) != nil {
		return raw
	}
	b, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return raw
	}
	return string(b)
}

// prepareToolImage convierte la captura a JPEG reducido, la guarda en disco y
// devuelve la parte image_url que se enviara al modelo.
func prepareToolImage(img mcp.Image) (string, api.ContentPart, error) {
	raw, err := base64.StdEncoding.DecodeString(img.Data)
	if err != nil {
		return "", api.ContentPart{}, err
	}
	mime, data, err := images.Prepare(raw, img.MIME)
	if err != nil {
		return "", api.ContentPart{}, err
	}
	path, err := saveToolImage(data, mime)
	if err != nil {
		return "", api.ContentPart{}, err
	}
	part := api.ContentPart{
		Type: "image_url",
		ImageURL: &api.ImageURL{
			URL: "data:" + mime + ";base64," + base64.StdEncoding.EncodeToString(data),
		},
	}
	return path, part, nil
}

func saveToolImage(data []byte, mime string) (string, error) {
	dir := filepath.Join(config.Home(), "tool-images")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	ext := ".png"
	switch strings.ToLower(mime) {
	case "image/jpeg", "image/jpg":
		ext = ".jpg"
	case "image/webp":
		ext = ".webp"
	}
	path := filepath.Join(dir, fmt.Sprintf("tool-%d%s", time.Now().UnixNano(), ext))
	return path, os.WriteFile(path, data, 0o600)
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}

// ------------------------------------------------------------- comandos ---

func (m Model) mcpSlash(rest string) Model {
	arg := strings.ToLower(strings.TrimSpace(rest))
	switch arg {
	case "import", "importar":
		n, path, err := m.cfg.ImportMCP()
		if err != nil {
			m.toast, m.toastErr = err.Error(), true
			m.msgs = append(m.msgs, Message{Role: "assistant", Text: "MCP: " + err.Error()})
			break
		}
		m.closeMCP()
		m.toast, m.toastErr = fmt.Sprintf("%d servidores MCP importados", n), false
		m.msgs = append(m.msgs, Message{Role: "assistant",
			Text: fmt.Sprintf("Importados %d servidores MCP desde %s.\nEscribe /agent on o simplemente un mensaje.", n, path)})
	case "on", "off":
		m.cfg.MCP.Enabled = arg == "on"
		m.saveCfg()
		if !m.cfg.MCP.Enabled {
			m.closeMCP()
		}
		m.toast = "Agente MCP: " + onOff(m.cfg.MCP.Enabled)
	case "reload", "recargar":
		m.closeMCP()
		m.toast = "MCP se reconectará en el próximo mensaje"
	case "tools", "herramientas":
		if m.mcp == nil {
			m.msgs = append(m.msgs, Message{Role: "assistant", Text: "MCP aún no conectado. Envía un mensaje o usa /mcp import."})
			break
		}
		var b strings.Builder
		b.WriteString("Herramientas MCP conectadas:\n")
		for _, t := range m.mcpTools {
			b.WriteString("\n• " + t.Wire + "\n  " + oneLine(t.Description, 160))
		}
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: b.String()})
	case "allow", "ask", "deny":
		m.cfg.Permissions.MCP = arg
		m.saveCfg()
		m.toast = "Permiso MCP: " + arg
	case "":
		var b strings.Builder
		b.WriteString("MCP · agente " + onOff(m.cfg.MCP.Enabled) + "\n")
		if m.mcp != nil {
			b.WriteString("Conectados: " + strings.Join(m.mcp.Servers(), ", ") + "\n")
			b.WriteString(fmt.Sprintf("Herramientas: %d\n", len(m.mcpTools)))
		} else {
			b.WriteString("Conectados: (ninguno todavía)\n")
		}
		b.WriteString(fmt.Sprintf("Configurados: %d · permiso: %s\n", len(m.cfg.MCP.Servers), m.cfg.Permissions.MCP))
		if len(m.mcpErrs) > 0 {
			b.WriteString("\nErrores:\n")
			for _, e := range m.mcpErrs {
				b.WriteString("• " + oneLine(e, 180) + "\n")
			}
		}
		b.WriteString("\n/mcp import · /mcp tools · /mcp on|off · /mcp ask|allow|deny")
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: b.String()})
	default:
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Uso: /mcp [import|tools|on|off|reload|ask|allow|deny]"})
	}
	m = m.saveActive()
	m.layout()
	return m
}

func (m Model) toggleAgent() Model {
	m.cfg.MCP.Enabled = !m.cfg.MCP.Enabled
	m.saveCfg()
	if !m.cfg.MCP.Enabled {
		m.closeMCP()
	}
	m.toast = "Agente MCP: " + onOff(m.cfg.MCP.Enabled)
	return m
}

func (m Model) closeMCP() Model {
	if m.mcp != nil {
		m.mcp.Close()
		m.mcp = nil
	}
	m.mcpTools = nil
	m.mcpErrs = nil
	return m
}

func (m Model) mcpStatus() string {
	if !m.cfg.MCP.Enabled {
		return "MCP off"
	}
	if m.mcp == nil {
		return "MCP sin conectar"
	}
	return fmt.Sprintf("MCP %d tools", len(m.mcpTools))
}
