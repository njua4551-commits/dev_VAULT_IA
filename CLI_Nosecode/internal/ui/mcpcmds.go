package ui

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"nosecode/internal/config"
	"nosecode/internal/mcp"

	tea "github.com/charmbracelet/bubbletea"
)

// mcpTestMsg lleva el resultado de «/mcp test» (o de la prueba tras añadir).
type mcpTestMsg struct {
	name string
	res  mcp.ProbeResult
	err  error
}

func testMCPCmd(name string, cfg config.MCPServer) tea.Cmd {
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 40*time.Second)
		defer cancel()
		res, err := mcp.Probe(ctx, name, cfg)
		return mcpTestMsg{name: name, res: res, err: err}
	}
}

// takeMCPTests devuelve los comandos de prueba pendientes y los vacía.
func (m Model) takeMCPTests() (Model, tea.Cmd) {
	if m.mcpTestName == "" {
		return m, nil
	}
	var cmds []tea.Cmd
	for _, n := range strings.Fields(m.mcpTestName) {
		if cfg, ok := m.cfg.MCP.Servers[n]; ok {
			cmds = append(cmds, testMCPCmd(n, cfg))
		}
	}
	m.mcpTestName = ""
	return m, tea.Batch(cmds...)
}

func (m Model) onMCPTest(msg mcpTestMsg) (tea.Model, tea.Cmd) {
	var text string
	if msg.err != nil {
		text = fmt.Sprintf("✗ MCP «%s» no respondió:\n%s\n\nPrueba /mcp doctor %s para revisar comando, PATH y variables.", msg.name, msg.err.Error(), msg.name)
		m.toast, m.toastErr = "MCP «"+msg.name+"»: error", true
	} else {
		names := make([]string, 0, len(msg.res.Tools))
		for _, t := range msg.res.Tools {
			names = append(names, t.Name)
		}
		shown := names
		if len(shown) > 12 {
			shown = append(append([]string{}, shown[:12]...), fmt.Sprintf("… (+%d)", len(names)-12))
		}
		info := msg.res.Info
		if info == "" {
			info = "servidor sin nombre"
		}
		text = fmt.Sprintf("✓ MCP «%s» · %s · %d herramientas · %.1f s\n  %s",
			msg.name, info, len(names), msg.res.Latency.Seconds(), strings.Join(shown, ", "))
		if len(names) > 20 {
			text += fmt.Sprintf("\n\nSon muchas herramientas (%d): cada una gasta contexto. Limita con /mcp filter %s include <patrones>.", len(names), msg.name)
		}
		m.toast, m.toastErr = "MCP «"+msg.name+"»: ok", false
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: text})
	m = m.saveActive()
	m.layout()
	return m, nil
}

// mcpStatusText es la tabla por servidor de «/mcp» y «/mcp list».
func (m Model) mcpStatusText() string {
	names := make([]string, 0, len(m.cfg.MCP.Servers))
	for n := range m.cfg.MCP.Servers {
		names = append(names, n)
	}
	sort.Strings(names)
	var b strings.Builder
	b.WriteString("MCP · agente " + onOff(m.cfg.MCP.Enabled) + " · herramientas al modelo " + onOff(m.cfg.Tools.MCP) +
		" · permiso " + m.cfg.Permissions.MCP + "\n")
	if len(names) == 0 {
		b.WriteString("\nNo hay servidores. Añade uno con /mcp add o /mcp import.\n")
	}
	connected := map[string]bool{}
	if m.mcp != nil {
		for _, s := range m.mcp.Servers() {
			connected[s] = true
		}
	}
	for _, n := range names {
		s := m.cfg.MCP.Servers[n]
		total, visible := 0, 0
		for _, t := range m.mcpTools {
			if t.Server == n {
				total++
				if m.mcpToolVisible(t) {
					visible++
				}
			}
		}
		state := "sin conectar"
		switch {
		case !s.ImportedServerEnabled():
			state = "desactivado"
		case connected[n]:
			state = fmt.Sprintf("conectado · %d/%d herramientas", visible, total)
		case m.mcpLoading:
			state = "conectando…"
		default:
			for _, e := range m.mcpErrs {
				if strings.HasPrefix(e, n+": ") {
					state = "error: " + oneLine(strings.TrimPrefix(e, n+": "), 110)
				}
			}
		}
		kind := s.Type
		if kind == "" {
			kind = "local"
		}
		extra := ""
		if s.Permission != "" {
			extra += " · permiso " + s.Permission
		}
		if len(s.Include) > 0 {
			extra += " · include " + strings.Join(s.Include, ",")
		}
		if len(s.Exclude) > 0 {
			extra += " · exclude " + strings.Join(s.Exclude, ",")
		}
		fmt.Fprintf(&b, "\n• %s (%s) — %s%s", n, kind, state, extra)
	}
	b.WriteString("\n\n/mcp add · remove · test [nombre] · doctor <nombre> · enable|disable <nombre> · import [fuente|ruta] · filter <nombre> include|exclude|clear · perm <nombre> ask|allow|deny · tools · reload")
	return b.String()
}

// mcpToolVisible aplica include/exclude y el permiso «deny» por servidor.
func (m Model) mcpToolVisible(t mcp.Tool) bool {
	s, ok := m.cfg.MCP.Servers[t.Server]
	if !ok {
		return true
	}
	if s.Permission == "deny" {
		return false
	}
	return s.ToolAllowed(t.Name)
}

// mcpServerPerm devuelve el permiso propio del servidor dueño de la herramienta.
func (m Model) mcpServerPerm(wire string) string {
	for _, t := range m.mcpTools {
		if t.Wire == wire {
			return m.cfg.MCP.Servers[t.Server].Permission
		}
	}
	return ""
}

// mcpDoctor revisa lo básico sin conectar: comando, PATH, URL y variables.
func mcpDoctor(name string, s config.MCPServer) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Diagnóstico de «%s»\n", name)
	ok := func(good bool, msg string) {
		if good {
			b.WriteString("  ✓ " + msg + "\n")
		} else {
			b.WriteString("  ✗ " + msg + "\n")
		}
	}
	ok(s.ImportedServerEnabled(), "habilitado en la configuración")
	if s.Type == "remote" {
		ok(strings.HasPrefix(s.URL, "https://"), "URL "+s.URL+" (se recomienda https)")
		for _, k := range sortedKeys(s.Headers) {
			ok(strings.TrimSpace(s.Headers[k]) != "", "header "+k+" tiene valor")
		}
	} else if len(s.Command) == 0 {
		ok(false, "falta el comando")
	} else {
		p, err := exec.LookPath(s.Command[0])
		ok(err == nil, "comando «"+s.Command[0]+"» "+map[bool]string{true: "en " + p, false: "no está en el PATH"}[err == nil])
		switch s.Command[0] {
		case "npx", "npm", "node":
			_, e := exec.LookPath("node")
			ok(e == nil, "node instalado")
		case "uvx", "uv":
			_, e := exec.LookPath("uv")
			ok(e == nil, "uv instalado")
		}
		if s.Cwd != "" {
			_, e := os.Stat(s.Cwd)
			ok(e == nil, "carpeta cwd "+s.Cwd)
		}
		for _, k := range sortedKeys(s.Environment) {
			ok(strings.TrimSpace(s.Environment[k]) != "", "variable "+k+" tiene valor")
		}
	}
	b.WriteString("Ahora pruebo la conexión; si falla verás el stderr del servidor.")
	return b.String()
}

// mcpSubcommand atiende los subcomandos nuevos de /mcp. ok=false si no es suyo.
func (m Model) mcpSubcommand(f []string) (Model, bool) {
	if len(f) == 0 {
		return m, false
	}
	need := func(usage string) (string, bool) {
		if len(f) < 2 {
			m.toast, m.toastErr = "Uso: "+usage, true
			return "", false
		}
		n := strings.ToLower(f[1])
		if _, exists := m.cfg.MCP.Servers[n]; !exists {
			m.toast, m.toastErr = "No existe el servidor MCP "+n, true
			return "", false
		}
		return n, true
	}
	switch strings.ToLower(f[0]) {
	case "list", "ls", "estado", "status":
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: m.mcpStatusText()})
	case "test", "probar":
		if len(f) == 1 {
			var all []string
			for n, s := range m.cfg.MCP.Servers {
				if s.ImportedServerEnabled() {
					all = append(all, n)
				}
			}
			sort.Strings(all)
			if len(all) == 0 {
				m.toast, m.toastErr = "No hay servidores MCP activos que probar", true
				return m, true
			}
			m.mcpTestName = strings.Join(all, " ")
		} else {
			n, ok := need("/mcp test <nombre>")
			if !ok {
				return m, true
			}
			m.mcpTestName = n
		}
		m.toast, m.toastErr = "Probando MCP…", false
	case "doctor":
		n, ok := need("/mcp doctor <nombre>")
		if !ok {
			return m, true
		}
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: mcpDoctor(n, m.cfg.MCP.Servers[n])})
		m.mcpTestName = n
	case "enable", "disable", "activar", "desactivar":
		n, ok := need("/mcp enable|disable <nombre>")
		if !ok {
			return m, true
		}
		on := f[0] == "enable" || f[0] == "activar"
		s := m.cfg.MCP.Servers[n]
		s.Enabled = &on
		m.cfg.MCP.Servers[n] = s
		m = m.saveCfg()
		m = m.closeMCP()
		if len(m.cfg.MCP.Servers) > 0 {
			m.mcpLoading = true
		}
		m.toast, m.toastErr = "MCP «"+n+"»: "+map[bool]string{true: "activado", false: "desactivado"}[on], false
	case "filter", "filtro":
		n, ok := need("/mcp filter <nombre> include|exclude|clear [patrones,coma]")
		if !ok {
			return m, true
		}
		if len(f) < 3 {
			m.toast, m.toastErr = "Uso: /mcp filter "+n+" include|exclude|clear [patrones,coma]", true
			return m, true
		}
		s := m.cfg.MCP.Servers[n]
		switch strings.ToLower(f[2]) {
		case "include":
			s.Include = splitList(strings.Join(f[3:], ","))
		case "exclude":
			s.Exclude = splitList(strings.Join(f[3:], ","))
		case "clear":
			s.Include, s.Exclude = nil, nil
		default:
			m.toast, m.toastErr = "Usa include, exclude o clear", true
			return m, true
		}
		m.cfg.MCP.Servers[n] = s
		m = m.saveCfg()
		m.toast, m.toastErr = "Filtro de «"+n+"» guardado (aplica al instante)", false
	case "perm", "permiso":
		n, ok := need("/mcp perm <nombre> ask|allow|deny|clear")
		if !ok {
			return m, true
		}
		v := ""
		if len(f) > 2 {
			v = strings.ToLower(f[2])
		}
		if v != "ask" && v != "allow" && v != "deny" && v != "clear" {
			m.toast, m.toastErr = "Usa ask, allow, deny o clear", true
			return m, true
		}
		s := m.cfg.MCP.Servers[n]
		if v == "clear" {
			v = ""
		}
		s.Permission = v
		m.cfg.MCP.Servers[n] = s
		m = m.saveCfg()
		m.toast, m.toastErr = "Permiso de «"+n+"» guardado", false
	case "import", "importar":
		return m.mcpImport(f[1:]), true
	default:
		return m, false
	}
	m = m.saveActive()
	m.layout()
	return m, true
}

// mcpImport importa de OpenCode, Claude, Cursor, VS Code o de una ruta; no pisa
// servidores existentes.
func (m Model) mcpImport(args []string) Model {
	cwd, _ := os.Getwd()
	var files []config.ImportSource
	if len(args) > 0 {
		want := strings.ToLower(args[0])
		found := false
		for _, s := range config.ImportSources(cwd) {
			if s.Name == want {
				files, found = append(files, s), true
			}
		}
		if !found {
			files = []config.ImportSource{{Name: "archivo", Path: strings.Join(args, " ")}}
		}
	} else {
		for _, s := range config.ImportSources(cwd) {
			if _, err := os.Stat(s.Path); err == nil {
				files = append(files, s)
			}
		}
	}
	var b strings.Builder
	total := 0
	for _, src := range files {
		added, dup, bad, err := m.cfg.ImportMCPFrom(src.Path)
		switch {
		case err != nil:
			fmt.Fprintf(&b, "• %s (%s): %v\n", src.Name, src.Path, err)
		default:
			fmt.Fprintf(&b, "• %s (%s): %d añadidos", src.Name, src.Path, len(added))
			if len(added) > 0 {
				b.WriteString(" [" + strings.Join(added, ", ") + "]")
			}
			if len(dup) > 0 {
				b.WriteString(" · ya existían (no se tocaron): " + strings.Join(dup, ", "))
			}
			b.WriteString("\n")
			total += len(added)
		}
		for _, e := range bad {
			b.WriteString("    omitido " + e + "\n")
		}
	}
	if len(files) == 0 {
		b.WriteString("No encontré configuraciones MCP de otras apps. Usa /mcp import <ruta al .json> (opencode, claude, cursor, vscode).\n")
	}
	if total > 0 {
		m = m.closeMCP()
		m.mcpLoading = true
		b.WriteString("\nSe conectarán en segundo plano; /mcp test para comprobarlos, /tools mcp on para enviarlos al modelo.")
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Importación MCP:\n" + strings.TrimRight(b.String(), "\n")})
	m.toast, m.toastErr = fmt.Sprintf("%d servidores MCP importados", total), false
	return m
}
