package ui

import (
	"context"
	"strconv"
	"strings"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/config"
	"nosecode/internal/debuglog"
	"nosecode/internal/mcp"
	"nosecode/internal/session"

	"github.com/charmbracelet/bubbles/textarea"
	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
)

type screen int

const (
	screenChat screen = iota
	screenSettings
	screenPalette
	screenRename
	screenConfirm
	screenTool
)

// toolConfirmState pide permiso antes de ejecutar una herramienta MCP.
type toolConfirmState struct {
	calls []api.ToolCall
	idx   int
}

// agentRun mantiene el bucle del agente y su historial de herramientas.
type agentRun struct {
	msgs       []api.ChatMessage
	stream     *api.Streamer
	assistant  string
	calls      []api.ToolCall
	pending    []api.ToolCall
	pendingIdx int
	rounds     int
	steps      int
	connecting bool
	paused     bool
	wrapup     bool
	ctx        context.Context
	cancel     context.CancelFunc
	failures   toolFailureGuard
}

func (a *agentRun) busy() bool {
	return a != nil && (a.stream != nil || a.connecting || len(a.pending) > 0 || len(a.calls) > 0)
}

// activityState describe la espera visible en la barra de estado.
type activityState struct {
	kind    string
	label   string
	started time.Time
	seq     int
}

type activityMsg struct{ seq int }

func activityTick(seq int) tea.Cmd {
	return tea.Tick(time.Second, func(time.Time) tea.Msg { return activityMsg{seq: seq} })
}

func (m Model) startActivity(kind, label string) (Model, tea.Cmd) {
	m.activity = activityState{
		kind:    kind,
		label:   label,
		started: time.Now(),
		seq:     m.activity.seq + 1,
	}
	return m, activityTick(m.activity.seq)
}

func (m Model) stopActivity() Model {
	m.activity = activityState{seq: m.activity.seq + 1}
	return m
}

// renameState edita el titulo de una sesion.
type renameState struct {
	input textinput.Model
	id    string
}

// confirmState pide confirmacion para una accion destructiva.
type confirmState struct {
	id     string
	title  string
	action string
}

const (
	statusHeight = 1
	inputHeight  = 6
	railWidth    = 38
)

// Message es un turno de la conversacion (vive en internal/session).
type Message = session.Message

// Model es el estado raiz de la TUI.
type Model struct {
	cfg         *config.Config
	width       int
	height      int
	screen      screen
	input       textarea.Model
	msgs        []Message
	sessions    []session.Session
	activeID    string
	scroll      int
	toast       string
	toastErr    bool
	quit        bool
	mode        string
	plan        string
	hoverBox    hitBox
	tip         int
	settings    settings
	palette     palette
	rename      renameState
	confirm     confirmState
	hit         *hitStore
	run         runState
	mcp         *mcp.Manager
	mcpTools    []mcp.Tool
	mcpErrs     []string
	mcpLoading  bool
	mcpInFlight bool
	quota       quotaState
	tool        toolConfirmState
	sel         selectState
	buf         *screenBuffer
	slashMenu   slashState
	activity    activityState
	contextWin  int
	usageTotal  int
	usageValid  bool
	ctxWarned   bool

	compacting       bool
	compactingCancel context.CancelFunc
	allowTools       map[string]bool

	clickAt    time.Time
	clickX     int
	clickY     int
	clickCount int
}

// New construye la aplicacion con la configuracion ya cargada.
func New(cfg *config.Config) Model {
	m := newModel(cfg, true)
	if changed, url := cfg.SyncVault(); changed {
		_ = cfg.Save()
		m.toast = "Proveedor actualizado: " + strings.TrimSuffix(url, "/v1")
		m.toastErr = false
	}
	m.contextWin = m.effectiveContextWindow()
	if (cfg.MCP.Enabled || cfg.Tools.MCP || cfg.Tools.Browser) && len(cfg.MCP.Servers) > 0 {
		m.mcpLoading = true
		m.mcpInFlight = true // Init lanza la conexión; evita duplicarla al primer turno
	}
	return m
}

func newModel(cfg *config.Config, withSessions bool) Model {
	applyPalette(PaletteByName(cfg.UI.Theme))

	ta := textarea.New()
	ta.Prompt = ""
	ta.Placeholder = "Escribe un mensaje…"
	ta.ShowLineNumbers = false
	ta.CharLimit = 0
	ta.SetHeight(2)
	ta.Focus()

	m := Model{
		cfg:    cfg,
		screen: screenChat,
		input:  ta,
		mode:   normalizeMode(cfg.UI.Mode),
		hit:    &hitStore{},
		buf:    &screenBuffer{},
		tip:    int(time.Now().Unix()) % len(tips),
	}
	m.input.Placeholder = m.placeholder()
	m.contextWin = m.effectiveContextWindow()

	if !withSessions {
		return m
	}

	list, _ := session.List()
	if len(list) == 0 {
		s := session.New()
		_ = s.Save()
		list = []session.Session{s}
	}
	m.sessions = list

	active := 0
	for i, s := range list {
		if s.Updated.After(list[active].Updated) {
			active = i
		}
	}
	m.activeID = list[active].ID
	m.msgs = append([]Message{}, list[active].Messages...)
	return m
}

// Init implementa tea.Model.
func (m Model) Init() tea.Cmd {
	defs := m.apiTools()
	debuglog.Logf("startup tools=%d approx_tokens=%d browser=%v mcp=%v",
		len(defs), toolDefsTokens(defs), m.cfg.Tools.Browser, m.cfg.Tools.MCP)
	cmds := []tea.Cmd{m.fetchQuota(), quotaTickCmd()}
	if m.mcpLoading {
		cmds = append(cmds, connectMCPCmd(m.cfg.MCP.Servers))
	}
	if m.providerReady() {
		_, p := m.cfg.Active()
		cmds = append(cmds, propsCmd(api.Bases(p.BaseURL, config.VaultHome()), p.APIKey))
	}
	debuglog.Logf("startup context_window=%d", m.contextWin)
	return tea.Batch(cmds...)
}

// Update implementa tea.Model.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		m.sel = selectState{}
		m.layout()
		return m, nil
	case tea.MouseMsg:
		return m.mouse(msg)
	case tea.KeyMsg:
		return m.key(msg)
	case chunkMsg:
		if m.run.agent != nil && m.run.agent.stream == msg.s {
			return m.onAgentChunk(msg)
		}
		return m.onChunk(msg)
	case compactDoneMsg:
		return m.onCompactDone(msg)
	case propsMsg:
		if msg.n > 0 {
			m.contextWin = msg.n
			debuglog.Logf("props n_ctx=%d", msg.n)
		}
		return m, nil
	case mcpReadyMsg:
		return m.onMCPReady(msg)
	case toolDoneMsg:
		return m.onToolDone(msg)
	case activityMsg:
		if msg.seq == m.activity.seq && m.activity.kind != "" {
			return m, activityTick(msg.seq)
		}
		return m, nil
	case vaultMsg:
		return m.onVault(msg)
	case quotaMsg:
		return m.onQuota(msg), nil
	case quotaTickMsg:
		if m.quota.loading {
			return m, quotaTickCmd()
		}
		m.quota.loading = true
		return m, tea.Batch(fetchQuotaCmd(), quotaTickCmd())
	}

	var cmd tea.Cmd
	if m.screen == screenChat {
		before := m.input.Value()
		m.input, cmd = m.input.Update(msg)
		if m.input.Value() != before {
			m.slashMenu.hidden = false
			m.slashMenu.cursor = 0
		}
	}
	if m.screen == screenRename {
		m.rename.input, cmd = m.rename.input.Update(msg)
	}
	return m, cmd
}

func (m *Model) layout() {
	if m.width <= 0 {
		return
	}
	w := m.inputBoxWidth() - 5
	if w < 10 {
		w = 10
	}
	m.input.SetWidth(w)
	m.input.SetHeight(2)
	if m.screen == screenRename {
		m.rename.input.Width = clamp(m.width-30, 18, 52)
	}
	m.layoutSettings()
}

func (m Model) key(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := k.String()

	if key == "ctrl+q" {
		m.cancelAll()
		return m, tea.Quit
	}
	if key == "ctrl+c" {
		// En una terminal, ctrl+c se usa para copiar. No debe cerrar Nosecode.
		if m.sel.active || m.sel.moved {
			return m.copySelection(), nil
		}
		return m.copyLast(), nil
	}

	switch m.screen {
	case screenRename:
		return m.renameKey(k)
	case screenConfirm:
		return m.confirmKey(k)
	case screenTool:
		return m.toolKey(k)
	}

	switch key {
	case "ctrl+p":
		if m.screen == screenPalette {
			m.screen = screenChat
			return m, nil
		}
		return m.openPalette(), nil
	case "f1":
		return m.openSettings(0), nil
	case "ctrl+n":
		return m.newSession(), nil
	case "ctrl+b":
		m.cfg.UI.Sidebar = !m.cfg.UI.Sidebar
		m = m.saveCfg()
		m.toast = "Barra lateral: " + onOff(m.cfg.UI.Sidebar)
		return m, nil
	}

	switch m.screen {
	case screenSettings:
		return m.settingsKey(k)
	case screenPalette:
		return m.paletteKey(k)
	}

	if nm, cmd, handled := m.slashKey(k); handled {
		return nm, cmd
	}

	switch key {
	case "shift+tab":
		return m.toggleMode(), nil
	case "f8":
		if m.run.busy() || m.compacting {
			m.toast, m.toastErr = "Espera a que termine o pulsa esc para cancelar", false
			return m, nil
		}
		return m.openConfirmDown(), nil
	case "f2":
		return m.openRename(m.activeID), nil
	case "ctrl+w":
		return m.openConfirmDelete(m.activeID), nil
	case "ctrl+e":
		if m.plan != "" {
			return m.approvePlan(), nil
		}
		m.toast = "No hay un plan pendiente"
		return m, nil
	case "enter":
		if strings.TrimSpace(m.input.Value()) == "" {
			return m, nil
		}
		if m.run.busy() || m.compacting {
			m.toast, m.toastErr = "Espera a que termine o pulsa esc para cancelar", false
			return m, nil
		}
		return m.submit()
	case "alt+enter", "ctrl+j":
		m.input.InsertString("\n")
		return m, nil
	case "esc":
		if m.run.busy() || m.compacting {
			return m.cancelBusy()
		}
		if m.sel.active || m.sel.moved {
			m.sel = selectState{}
			return m, nil
		}
		if m.input.Value() != "" {
			m.input.Reset()
			m.toast = "Entrada limpiada"
		}
		return m, nil
	case "ctrl+y":
		return m.copyLast(), nil
	case "pgup":
		m.scroll += 5
		return m, nil
	case "pgdown":
		m.scroll -= 5
		if m.scroll < 0 {
			m.scroll = 0
		}
		return m, nil
	}

	var cmd tea.Cmd
	before := m.input.Value()
	m.input, cmd = m.input.Update(k)
	if m.input.Value() != before {
		m.slashMenu.hidden = false
		m.slashMenu.cursor = 0
	}
	return m, cmd
}

func (m Model) mouse(msg tea.MouseMsg) (tea.Model, tea.Cmd) {
	if msg.Action == tea.MouseActionMotion {
		if m.sel.dragging {
			y := clamp(msg.Y, m.sel.panelTop, m.sel.panelBottom)
			x := clamp(msg.X, m.sel.panelX0, m.sel.panelX1-1)
			if x != m.sel.endX || y != m.sel.endY {
				m.sel.endX, m.sel.endY = x, y
				m.sel.moved = m.sel.moved || x != m.sel.startX || y != m.sel.startY
			}
			return m, nil
		}
		if hb, ok := m.hit.At(msg.X, msg.Y); ok {
			m.hoverBox = hb
		} else {
			m.hoverBox = hitBox{}
		}
		return m, nil
	}

	switch msg.Button {
	case tea.MouseButtonWheelUp:
		m.scroll += 3
		return m, nil
	case tea.MouseButtonWheelDown:
		m.scroll -= 3
		if m.scroll < 0 {
			m.scroll = 0
		}
		return m, nil
	}

	if msg.Action == tea.MouseActionPress && msg.Button == tea.MouseButtonLeft {
		if hb, ok := m.hit.At(msg.X, msg.Y); ok {
			m.sel = selectState{}
			m.clickCount = 0
			return m.activate(hb.action, hb.arg)
		}
		if m.screen != screenChat {
			if m.screen != screenTool && m.screen != screenConfirm {
				m.screen = screenChat
			}
			m.sel = selectState{}
			return m, nil
		}
		if m.buf == nil || msg.Y < 0 || msg.Y >= len(m.buf.lines) {
			return m, nil
		}
		ln := m.buf.lines[msg.Y]
		if ln.panel != "chat" {
			m.sel = selectState{}
			return m, nil
		}

		now := time.Now()
		count := 1
		dx := msg.X - m.clickX
		if dx < 0 {
			dx = -dx
		}
		if !m.clickAt.IsZero() && now.Sub(m.clickAt) < 450*time.Millisecond &&
			dx <= 2 && msg.Y == m.clickY {
			count = m.clickCount + 1
		}
		if count > 3 {
			count = 1
		}
		m.clickAt, m.clickX, m.clickY, m.clickCount = now, msg.X, msg.Y, count

		if count == 3 {
			m.sel = selectState{}
			return m.selectMessageAt(msg.X, msg.Y), nil
		}
		if count == 2 {
			m.sel = selectState{}
			return m.selectWordAt(msg.X, msg.Y), nil
		}

		top, bottom, ok := m.panelBounds(msg.Y)
		if !ok {
			return m, nil
		}
		x := clamp(msg.X, ln.panelX0, ln.panelX1-1)
		m.sel = selectState{
			dragging:    true,
			startX:      x,
			startY:      msg.Y,
			endX:        x,
			endY:        msg.Y,
			panel:       ln.panel,
			panelTop:    top,
			panelBottom: bottom,
			panelX0:     ln.panelX0,
			panelX1:     ln.panelX1,
		}
		return m, nil
	}

	if msg.Action == tea.MouseActionRelease && m.sel.dragging {
		m.sel.endY = clamp(msg.Y, m.sel.panelTop, m.sel.panelBottom)
		m.sel.endX = clamp(msg.X, m.sel.panelX0, m.sel.panelX1-1)
		m.sel.dragging = false
		if m.sel.moved {
			m.sel.active = true
			return m.copySelection(), nil
		}
		m.sel = selectState{}
	}
	return m, nil
}

func (m Model) activate(action, arg string) (tea.Model, tea.Cmd) {
	switch action {
	case "focusInput":
		m.input.Focus()
		return m, nil
	case "new":
		return m.newSession(), nil
	case "sessionSwitch":
		return m.loadSession(arg), nil
	case "sessionClose":
		return m.openConfirmDelete(arg), nil
	case "sessionRename":
		return m.openRename(arg), nil
	case "renameSave":
		return m.saveRename()
	case "renameCancel":
		m.screen = screenChat
		return m, nil
	case "confirmDelete":
		return m.deleteSessionConfirmed(), nil
	case "confirmGo":
		return m.confirmConfirmed()
	case "confirmCancel":
		m.screen = screenChat
		return m, nil
	case "toolAllow":
		return m.toolKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("y")})
	case "toolAllowSession":
		return m.toolKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	case "toolDeny":
		return m.toolKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("n")})
	case "slashItem":
		idx, _ := strconv.Atoi(arg)
		matches := m.slashMatches()
		if idx < 0 || idx >= len(matches) {
			return m, nil
		}
		return m.runSlashEntry(matches[idx])
	case "mode":
		return m.toggleMode(), nil
	case "approvePlan":
		return m.approvePlan(), nil
	case "settings":
		return m.openSettings(0), nil
	case "closeSettings":
		m.screen = screenChat
		return m, nil
	case "providerEdit":
		name := arg
		if name == "" {
			names := m.providerNames()
			if len(names) > 0 {
				name = names[0]
			}
		}
		return m.startEdit(name, false), nil
	case "providerNew":
		return m.startEdit("", true), nil
	case "providerDelete":
		return m.deleteProvider()
	case "providerActivate":
		return m.activateProviderNamed(arg)
	case "providerImport":
		m = m.importVault()
		m.sound(soundSuccess)
		return m, nil
	case "editModel":
		name, _ := m.cfg.Active()
		m = m.startEdit(name, false)
		m.settings.field = 3
		m.settings.fields[0].Blur()
		m.settings.fields[3].Focus()
		return m, nil
	case "option":
		return m.toggleOption(arg), nil
	case "perm":
		return m.cyclePermission(arg), nil
	case "field":
		if m.settings.editing {
			m.setField(arg)
		}
		return m, nil
	case "settingRow":
		idx, _ := strconv.Atoi(arg)
		m.settings.cursor = idx
		return m.activateSetting()
	case "saveEdit":
		return m.saveEdit()
	case "cancelEdit":
		m.settings.editing = false
		m.toast = "Edición cancelada"
		return m, nil
	case "paletteItem":
		idx, _ := strconv.Atoi(arg)
		if idx >= 0 && idx < len(m.palette.filtered) {
			m.palette.cursor = idx
			item := m.palette.filtered[idx]
			m.screen = screenChat
			m = item.action(m)
			if m.quit {
				return m, tea.Quit
			}
		}
		return m, nil
	case "theme":
		return m.cycleTheme(), nil
	case "sound":
		m.cfg.UI.Mute = !m.cfg.UI.Mute
		m.saveCfg()
		m.toast = "Sonidos: " + onOff(!m.cfg.UI.Mute)
		return m, nil
	case "themeSet":
		m = m.setTheme(arg)
		return m, nil
	case "quit":
		m.quit = true
		return m, tea.Quit
	}
	return m, nil
}

func (m Model) submit() (tea.Model, tea.Cmd) {
	text := strings.TrimSpace(m.input.Value())
	m.input.Reset()
	m.scroll = 0

	if strings.HasPrefix(text, "/") {
		if cmd, ok := vaultCommand(text); ok {
			return m.startVault(cmd)
		}
		if strings.EqualFold(strings.Fields(text)[0], "/compact") {
			return m.compactStart()
		}
		m = m.slash(text)
		if m.mcpLoading && m.mcp == nil && !m.mcpInFlight && len(m.cfg.MCP.Servers) > 0 {
			m.mcpInFlight = true
			return m, connectMCPCmd(m.cfg.MCP.Servers)
		}
		return m, nil
	}

	m.msgs = append(m.msgs, Message{Role: "user", Text: text})
	if m.mode == "plan" {
		m.plan = text
	} else {
		m.plan = ""
	}
	m.toast = ""

	m = m.syncVault()
	if m.providerReady() {
		m = m.saveActive()
		if m.useAgent() {
			return m.startAgent()
		}
		return m.startChat()
	}

	m.msgs = append(m.msgs, Message{Role: "assistant", Text: m.assistantReply(text)})
	m = m.saveActive()
	m.sound(soundDone)
	m.layout()
	return m, nil
}

func (m Model) assistantReply(prompt string) string {
	name, p := m.cfg.Active()
	connected := p.BaseURL != "" && p.APIKey != ""

	if m.mode == "plan" {
		short := fit(strings.Join(strings.Fields(prompt), " "), 64)
		reply := "Modo Plan · no ejecuto cambios todavía.\n\n" +
			"Plan propuesto para «" + short + "»\n" +
			"1. Revisar el contexto de la sesión.\n" +
			"2. Identificar archivos y acciones implicadas.\n" +
			"3. Describir los cambios, el orden y los riesgos.\n\n" +
			"Aprueba con ctrl+e o desde la paleta para pasar a Build."
		if connected {
			return reply + "\n\nProveedor «" + name + "» listo para generar el plan detallado."
		}
		return reply + "\n\nConecta un proveedor para generar el plan detallado."
	}

	switch {
	case p.BaseURL == "" && p.APIKey == "":
		return "Aún no hay un proveedor configurado.\n\n" +
			"Abre Ajustes → Proveedores para añadir el endpoint y la API key,\n" +
			"o importa la configuración del Vault.\n\n" +
			"Esta versión guarda la conversación localmente."
	case p.BaseURL == "":
		return "El proveedor «" + name + "» tiene API key pero le falta la URL base."
	case p.APIKey == "":
		return "El proveedor «" + name + "» tiene URL base pero le falta la API key."
	default:
		return "Proveedor «" + name + "» listo: " + p.BaseURL + "\n\n" +
			"El envío con streaming se activará cuando conectes el cliente."
	}
}

func (m Model) slash(text string) Model {
	fields := strings.Fields(text)
	cmd := strings.ToLower(fields[0])
	rest := strings.TrimSpace(strings.TrimPrefix(text, fields[0]))

	switch cmd {
	case "/help", "/?":
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: helpText})
	case "/settings":
		return m.openSettings(0)
	case "/config":
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Config: " + m.cfg.Path})
	case "/new", "/clear":
		return m.newSession()
	case "/quit", "/exit":
		m.quit = true
	case "/tokens":
		m.cfg.UI.Tokens = !m.cfg.UI.Tokens
		m.saveCfg()
		m.toast = "Contador de tokens: " + onOff(m.cfg.UI.Tokens)
	case "/sound":
		m.cfg.UI.Mute = !m.cfg.UI.Mute
		m.saveCfg()
		m.toast = "Sonidos: " + onOff(!m.cfg.UI.Mute)
	case "/theme":
		return m.cycleTheme()
	case "/plan":
		return m.setMode("plan")
	case "/build":
		return m.setMode("build")
	case "/sessions", "/sesiones":
		m = m.openPalette()
		m.palette.query = "Sesiones"
		m.palette.filter()
		return m
	case "/rename":
		return m.openRename(m.activeID)
	case "/delete":
		return m.openConfirmDelete(m.activeID)
	case "/model":
		if rest == "" {
			name, p := m.cfg.Active()
			model := p.Model
			if model == "" {
				model = "sin modelo"
			}
			m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Proveedor: " + name + "\nModelo: " + model})
		} else {
			name, p := m.cfg.Active()
			p.Model = rest
			m.cfg.Providers[name] = p
			m.saveCfg()
			m.toast = "Modelo: " + rest
		}
	case "/vault":
		m = m.importVault()
	case "/copy", "/copiar":
		return m.copyCommand(rest)
	case "/mcp":
		return m.mcpSlash(rest)
	case "/tools":
		return m.toolsSlash(rest)
	case "/agent":
		return m.toggleAgent()
	default:
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Comando desconocido: " + cmd + "\nPrueba /help."})
	}

	m = m.saveActive()
	m.layout()
	return m
}

func (m Model) saveCfg() Model {
	if err := m.cfg.Save(); err != nil {
		m.toast = "Error guardando: " + err.Error()
		m.toastErr = true
		return m
	}
	m.toastErr = false
	m.sound(soundDone)
	return m
}

func (m Model) sound(name string) {
	if m.cfg.UI.Mute {
		return
	}
	playSound(name)
}

func (m Model) importVault() Model {
	p, ok := config.ImportVault()
	if !ok {
		m.toast = "No se encontró la configuración del Vault"
		m.toastErr = true
		m.sound(soundError)
		return m
	}
	m.cfg.Providers["vault"] = p
	m.cfg.DefaultProvider = "vault"
	m.saveCfg()
	m.toast = "Proveedor importado del Vault"
	return m
}

func (m Model) setTheme(name string) Model {
	m.cfg.UI.Theme = name
	applyPalette(PaletteByName(name))
	m.saveCfg()
	m.toast = "Tema: " + PaletteByName(name).Name
	return m
}

func (m Model) cycleTheme() Model {
	next := "vercel"
	if PaletteByName(m.cfg.UI.Theme).Name == "vercel" {
		next = "midnight"
	}
	return m.setTheme(next)
}

// ------------------------------------------------------------- modos ---

func normalizeMode(s string) string {
	if s == "plan" {
		return "plan"
	}
	return "build"
}

func modeLabel(s string) string {
	if s == "plan" {
		return "Plan"
	}
	return "Build"
}

func (m Model) placeholder() string {
	if m.mode == "plan" {
		return "Describe la tarea; primero verás el plan…"
	}
	return "Escribe un mensaje…"
}

func (m Model) setMode(mode string) Model {
	if a := m.run.agent; a != nil && a.paused {
		// El historial pausado pertenece al modo anterior.
		m = m.dropAgent()
	}
	mode = normalizeMode(mode)
	m.mode = mode
	m.cfg.UI.Mode = mode
	m.input.Placeholder = m.placeholder()
	m = m.saveCfg()
	m.toast = "Modo " + modeLabel(mode)
	return m
}

func (m Model) toggleMode() Model {
	if m.mode == "plan" {
		return m.setMode("build")
	}
	return m.setMode("plan")
}

// approvePlan pasa a Build y registra la aprobacion del plan pendiente.
func (m Model) approvePlan() Model {
	if m.plan == "" {
		m.toast = "No hay un plan pendiente"
		return m
	}
	prompt := m.plan
	m.plan = ""
	m = m.setMode("build")
	m.msgs = append(m.msgs, Message{
		Role: "assistant",
		Text: "Plan aprobado. Modo Build activo.\n\n" + m.assistantReply(prompt),
	})
	m = m.saveActive()
	m.toast = "Plan aprobado · modo Build"
	m.layout()
	return m
}

// ---------------------------------------------------------- sesiones ---

func (m Model) sessionIndex(id string) int {
	for i, s := range m.sessions {
		if s.ID == id {
			return i
		}
	}
	return -1
}

func (m Model) activeIndex() int { return m.sessionIndex(m.activeID) }

func (m Model) activeTitle() string {
	if idx := m.activeIndex(); idx >= 0 {
		return m.sessions[idx].Title
	}
	return "Nueva sesión"
}

// saveActive sincroniza los mensajes actuales con la sesion en disco.
func (m Model) saveActive() Model {
	idx := m.activeIndex()
	if idx < 0 {
		return m
	}
	s := m.sessions[idx]
	s.Messages = append([]Message{}, m.msgs...)
	if !s.TitleEdited {
		for _, msg := range m.msgs {
			if msg.Role == "user" && strings.TrimSpace(msg.Text) != "" {
				s.Title = session.TitleFrom(msg.Text)
				break
			}
		}
	}
	s.Updated = time.Now()
	m.sessions[idx] = s
	if err := s.Save(); err != nil {
		m.toast = "No se pudo guardar la sesión"
		m.toastErr = true
	}
	return m
}

// -------------------------------------------------- renombrar y borrar ---

func (m Model) openRename(id string) Model {
	idx := m.sessionIndex(id)
	if idx < 0 {
		return m
	}
	ti := textinput.New()
	ti.Prompt = ""
	ti.CharLimit = 60
	ti.Width = clamp(m.width-30, 18, 52)
	ti.SetValue(m.sessions[idx].Title)
	ti.CursorEnd()
	ti.Focus()
	m.rename = renameState{input: ti, id: id}
	m.screen = screenRename
	return m
}

func (m Model) renameKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc":
		m.screen = screenChat
		m.toast = "Renombrado cancelado"
		return m, nil
	case "enter", "ctrl+s":
		return m.saveRename()
	}

	var cmd tea.Cmd
	m.rename.input, cmd = m.rename.input.Update(k)
	return m, cmd
}

func (m Model) saveRename() (tea.Model, tea.Cmd) {
	title := strings.TrimSpace(m.rename.input.Value())
	if title == "" {
		m.toast = "El nombre no puede estar vacío"
		m.toastErr = true
		return m, nil
	}
	idx := m.sessionIndex(m.rename.id)
	if idx < 0 {
		m.screen = screenChat
		return m, nil
	}
	m.sessions[idx].Title = fit(title, 60)
	m.sessions[idx].TitleEdited = true
	m.sessions[idx].Updated = time.Now()
	if err := m.sessions[idx].Save(); err != nil {
		m.toast = "No se pudo guardar el nombre"
		m.toastErr = true
		return m, nil
	}
	m.screen = screenChat
	m.toast = "Sesión renombrada"
	m.sound(soundDone)
	return m, nil
}

func (m Model) openConfirmDelete(id string) Model {
	idx := m.sessionIndex(id)
	if idx < 0 {
		return m
	}
	if len(m.sessions) <= 1 {
		m.toast = "No puedes eliminar la última sesión"
		m.toastErr = true
		return m
	}
	m.confirm = confirmState{id: id, title: m.sessions[idx].Title, action: "delete"}
	m.screen = screenConfirm
	return m
}

func (m Model) openConfirmDown() Model {
	m.confirm = confirmState{title: "Apagar la IA en Kaggle", action: "vaultDown"}
	m.screen = screenConfirm
	return m
}

func (m Model) confirmConfirmed() (tea.Model, tea.Cmd) {
	if m.confirm.action == "vaultDown" {
		m.screen = screenChat
		return m.startVault("down")
	}
	return m.deleteSessionConfirmed(), nil
}

func (m Model) confirmKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "esc", "n":
		m.screen = screenChat
		return m, nil
	case "enter", "y":
		return m.confirmConfirmed()
	}
	return m, nil
}

func (m Model) deleteSessionConfirmed() Model {
	id := m.confirm.id
	m.screen = screenChat
	m = m.closeSession(id)
	m.sound(soundSuccess)
	return m
}

func (m Model) resetContextUsage() Model {
	m.usageValid = false
	m.usageTotal = 0
	m.ctxWarned = false
	return m
}

func (m Model) loadSession(id string) Model {
	if id == m.activeID {
		return m
	}
	m = m.dropAgent()
	m = m.saveActive()
	for _, s := range m.sessions {
		if s.ID == id {
			m.activeID = id
			m.msgs = append([]Message{}, s.Messages...)
			m = m.resetContextUsage()
			m.scroll = 0
			m.input.Reset()
			m.toast = ""
			m.screen = screenChat
			m.layout()
			return m
		}
	}
	return m
}

func (m Model) newSession() Model {
	m = m.dropAgent()
	m = m.saveActive()
	s := session.New()
	if err := s.Save(); err != nil {
		m.toast = "No se pudo crear la sesión"
		m.toastErr = true
		return m
	}
	m.sessions = append(m.sessions, s)
	m.activeID = s.ID
	m.msgs = nil
	m = m.resetContextUsage()
	m.scroll = 0
	m.input.Reset()
	m.toast = "Sesión creada"
	m.screen = screenChat
	m.layout()
	return m
}

func (m Model) closeSession(id string) Model {
	if len(m.sessions) <= 1 {
		m.toast = "No puedes eliminar la última sesión"
		return m
	}
	idx := -1
	for i, s := range m.sessions {
		if s.ID == id {
			idx = i
			break
		}
	}
	if idx < 0 {
		return m
	}
	_ = session.Delete(id)
	m.sessions = append(m.sessions[:idx], m.sessions[idx+1:]...)

	if id != m.activeID {
		m.toast = "Sesión eliminada"
		return m
	}
	next := idx - 1
	if next < 0 {
		next = 0
	}
	m.activeID = m.sessions[next].ID
	m.msgs = append([]Message{}, m.sessions[next].Messages...)
	m = m.resetContextUsage()
	m.scroll = 0
	m.input.Reset()
	m.toast = "Sesión eliminada"
	m.screen = screenChat
	m.layout()
	return m
}

func (m Model) toggleOption(id string) Model {
	switch id {
	case "tokens":
		m.cfg.UI.Tokens = !m.cfg.UI.Tokens
		m.toast = "Contador de tokens: " + onOff(m.cfg.UI.Tokens)
	case "animations":
		m.cfg.UI.Animations = !m.cfg.UI.Animations
		m.toast = "Animaciones: " + onOff(m.cfg.UI.Animations)
	case "sidebar":
		m.cfg.UI.Sidebar = !m.cfg.UI.Sidebar
		m.toast = "Barra lateral: " + onOff(m.cfg.UI.Sidebar)
	case "sound":
		m.cfg.UI.Mute = !m.cfg.UI.Mute
		m.toast = "Sonidos: " + onOff(!m.cfg.UI.Mute)
	}
	return m.saveCfg()
}

func (m Model) cyclePermission(id string) Model {
	var value *string
	switch id {
	case "bash":
		value = &m.cfg.Permissions.Bash
	case "edit":
		value = &m.cfg.Permissions.Edit
	case "web":
		value = &m.cfg.Permissions.Webfetch
	case "mcp":
		value = &m.cfg.Permissions.MCP
	default:
		return m
	}
	next := map[string]string{"ask": "allow", "allow": "deny", "deny": "ask"}[*value]
	if next == "" {
		next = "ask"
	}
	*value = next
	m.toast = "Permiso: " + next
	return m.saveCfg()
}

// ----------------------------------------------------------- layout ---

func (m Model) sidebarVisible() bool {
	return m.cfg.UI.Sidebar && m.width >= 100 && len(m.msgs) > 0
}

func (m Model) inputBoxWidth() int {
	w := m.width
	if m.sidebarVisible() {
		w -= railWidth
	}
	w -= 4
	if len(m.msgs) == 0 && w > 72 {
		w = 72
	}
	if w < 30 {
		w = 30
	}
	return w
}

func (m Model) effectiveContextWindow() int {
	_, p := m.cfg.Active()
	if p.ContextWindow > 0 {
		return p.ContextWindow
	}
	return config.DefaultContextWindow
}

func (m Model) tokensValue() int {
	total := 0
	for _, msg := range m.msgs {
		total += estimateTokens(msg.Text)
	}
	return total
}

// contextEstimate aproxima el prompt completo: sistema, herramientas e historial.
func (m Model) contextEstimate() int {
	if a := m.run.agent; a != nil && len(a.msgs) > 0 {
		total := m.apiToolTokens()
		for _, msg := range a.msgs {
			total += estimateTokens(messageText(msg))
		}
		return total
	}
	return estimateTokens(m.systemPrompt()) + estimateTokens(m.plan) + m.tokensValue()
}

func messageText(msg api.ChatMessage) string {
	switch c := msg.Content.(type) {
	case string:
		return c
	case []api.ContentPart:
		var b strings.Builder
		for _, p := range c {
			b.WriteString(p.Text)
		}
		return b.String()
	}
	return ""
}

// contextUsed prefiere el usage real del servidor; si no, estima.
func (m Model) contextUsed() int {
	if m.usageValid && m.usageTotal > 0 {
		return m.usageTotal
	}
	return m.contextEstimate()
}

func (m Model) contextPercent() int {
	win := m.contextWin
	if win <= 0 {
		win = m.effectiveContextWindow()
	}
	if win <= 0 {
		return 0
	}
	return m.contextUsed() * 100 / win
}

// warnContext avisa una sola vez al llegar al 80 % de la ventana.
func (m Model) warnContext() Model {
	if m.ctxWarned {
		return m
	}
	if pct := m.contextPercent(); pct >= 80 {
		m.ctxWarned = true
		if !m.toastErr {
			m.toast = "Contexto al " + strconv.Itoa(pct) + "% · usa /compact para resumir"
		}
	}
	return m
}

func (m Model) isHover(action, arg string) bool {
	return m.hoverBox.action == action && m.hoverBox.arg == arg
}

func estimateTokens(s string) int {
	n := len([]rune(s))
	if n == 0 {
		return 0
	}
	return (n + 3) / 4
}

func onOff(v bool) string {
	if v {
		return "on"
	}
	return "off"
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

var helpText = strings.Join([]string{
	"Comandos",
	"  /settings   abrir ajustes",
	"  /model [x]  ver o cambiar modelo",
	"  /tokens     contador de tokens",
	"  /sound      activar/desactivar sonidos",
	"  /theme      cambiar de tema",
	"  /plan       modo Plan (describir)",
	"  /build      modo Build (ejecutar)",
	"  /sessions   cambiar de sesión",
	"  /rename     renombrar sesión",
	"  /delete     eliminar sesión",
	"  /tools      herramientas activas y tokens; browser/mcp on|off",
	"  /compact    resumir la conversación para ahorrar contexto",
	"  /vault      importar endpoint del Vault",
	"  /up         encender la IA en Kaggle (vault.py up)",
	"  /down       apagar la IA y dejar de gastar GPU (F8)",
	"  /urls       ver qué túneles están vivos",
	"  /status     estado del notebook en Kaggle",
	"  /copy          copiar la última respuesta completa",
	"  /copy code [n] copiar un bloque de código",
	"  /copy files    guardar los bloques en ~/.nosecode/copies",
	"  /mcp        servidores MCP: import, tools, on/off",
	"  /agent      activar o desactivar herramientas MCP",
	"  /config     ruta del config",
	"  /new        nueva sesión",
	"  /quit       salir",
	"",
	"Atajos",
	"  ctrl+p  comandos    f1  ajustes",
	"  ctrl+n  nueva sesión    f2  renombrar",
	"  shift+tab  Plan / Build    ctrl+e  aprobar plan",
	"  ctrl+w  eliminar sesión    ctrl+b  barra lateral",
	"  enter   enviar      alt+enter  nueva línea",
	"  pgup/pgdn o rueda   desplazar",
	"  arrastra el ratón   seleccionar por caracteres y copiar",
	"  doble clic   palabra    triple clic   cuerpo del mensaje",
	"  ctrl+y  copiar la última respuesta",
	"  ctrl+c  copiar la selección",
	"  f8  apagar la IA    ctrl+q  salir",
}, "\n")

// Demo renderiza una pantalla sin TTY, util para revisar el diseno.
func Demo(cfg *config.Config, kind string, width, height int) string {
	m := newModel(cfg, false)
	updated, _ := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	model := updated.(Model)

	switch kind {
	case "home", "session", "tabs", "plan", "rename", "confirm":
		first := session.New()
		first.ID = "s-1"
		first.Title = "Configurar proveedor"
		first.Messages = []Message{
			{Role: "user", Text: "Hola, ¿qué puedes hacer?"},
			{Role: "assistant", Text: "Puedo guardar tus conversaciones, gestionar proveedores y preparar el endpoint para cuando conectes tu API."},
		}
		second := session.New()
		second.ID = "s-2"
		second.Title = "Revisar la interfaz"
		third := session.New()
		third.ID = "s-3"
		third.Title = "Ideas para el editor"

		model.sessions = []session.Session{first, second, third}
		model.activeID = first.ID
		model.msgs = nil
		if kind != "home" {
			model.msgs = first.Messages
		}
		model.toast = ""

		switch kind {
		case "plan":
			model.mode = "plan"
			model.input.Placeholder = model.placeholder()
			prompt := "Añade modos Plan y Build con confirmación"
			model.msgs = []Message{
				{Role: "user", Text: prompt},
				{Role: "assistant", Text: model.assistantReply(prompt)},
			}
		case "rename":
			model = model.openRename(first.ID)
		case "confirm":
			model = model.openConfirmDelete(second.ID)
		}
	case "settings":
		model = model.openSettings(0)
	case "settings-edit":
		model = model.openSettings(0)
		model = model.startEdit("vault", false)
	case "palette":
		model = model.openPalette()
	}
	model.layout()
	return model.View()
}
