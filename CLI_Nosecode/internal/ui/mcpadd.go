package ui

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"nosecode/internal/config"
)

var mcpNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]{0,31}$`)

// splitCommand parte una línea como lo haría un shell simple (comillas ' y "),
// sin expandir variables ni ejecutar nada.
func splitCommand(s string) ([]string, error) {
	var out []string
	var cur strings.Builder
	var quote rune
	started := false
	for _, r := range s {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote, started = r, true
		case r == ' ' || r == '\t':
			if started || cur.Len() > 0 {
				out = append(out, cur.String())
				cur.Reset()
				started = false
			}
		default:
			cur.WriteRune(r)
		}
	}
	if quote != 0 {
		return nil, errors.New("comillas sin cerrar")
	}
	if started || cur.Len() > 0 {
		out = append(out, cur.String())
	}
	return out, nil
}

// parseMCPAdd valida «nombre [opciones] comando args…», «nombre [opciones] https://url»
// o «nombre {json}». Opciones: --env K=V, --header "K: V", --cwd ruta,
// --timeout seg, --type local|remote, --include a,b, --exclude a,b y «--» para
// marcar el inicio del comando.
func parseMCPAdd(rest string) (string, config.MCPServer, error) {
	const usage = "uso: /mcp add <nombre> [--env K=V] [--header \"K: V\"] [--cwd ruta] [--timeout s] <comando [args…]|https://url|{json}>"
	rest = strings.TrimSpace(rest)
	parts, err := splitCommand(rest)
	if err != nil {
		// un JSON pegado puede traer comillas que splitCommand no entiende
		if !strings.Contains(rest, "{") {
			return "", config.MCPServer{}, err
		}
	}
	if len(parts) < 2 {
		return "", config.MCPServer{}, errors.New(usage)
	}
	name := strings.ToLower(parts[0])
	if !mcpNameRe.MatchString(name) {
		return "", config.MCPServer{}, errors.New("nombre inválido: usa a-z, 0-9, - y _ (máx. 32)")
	}
	if tail := strings.TrimSpace(rest[len(parts[0]):]); strings.HasPrefix(tail, "{") {
		srvs, _, err := config.ParseForeignMCP([]byte(`{"mcpServers":{"` + name + `":` + tail + `}}`))
		if err != nil {
			return "", config.MCPServer{}, fmt.Errorf("JSON del servidor: %w", err)
		}
		return name, srvs[name], nil
	}
	srv := config.MCPServer{}
	args := parts[1:]
	forced := ""
	for len(args) > 0 && strings.HasPrefix(args[0], "--") {
		flag := args[0]
		if flag == "--" {
			args = args[1:]
			break
		}
		if len(args) < 2 {
			return "", config.MCPServer{}, fmt.Errorf("a %s le falta su valor", flag)
		}
		val := args[1]
		args = args[2:]
		switch flag {
		case "--env":
			k, v, ok := strings.Cut(val, "=")
			if !ok || k == "" {
				return "", config.MCPServer{}, errors.New("--env espera CLAVE=valor")
			}
			if srv.Environment == nil {
				srv.Environment = map[string]string{}
			}
			srv.Environment[k] = v
		case "--header":
			k, v, ok := strings.Cut(val, ":")
			if !ok {
				k, v, ok = strings.Cut(val, "=")
			}
			if !ok || strings.TrimSpace(k) == "" {
				return "", config.MCPServer{}, errors.New("--header espera \"Nombre: valor\"")
			}
			if srv.Headers == nil {
				srv.Headers = map[string]string{}
			}
			srv.Headers[strings.TrimSpace(k)] = strings.TrimSpace(v)
		case "--cwd":
			srv.Cwd = val
		case "--timeout":
			n, err := strconv.Atoi(val)
			if err != nil || n < 1 || n > 600 {
				return "", config.MCPServer{}, errors.New("--timeout espera segundos (1..600)")
			}
			srv.Timeout = n * 1000
		case "--type":
			forced = strings.ToLower(val)
			if forced != "local" && forced != "remote" {
				return "", config.MCPServer{}, errors.New("--type espera local o remote")
			}
		case "--transport":
			if val != "sse" && val != "http" {
				return "", config.MCPServer{}, errors.New("--transport espera sse o http")
			}
			if val == "sse" {
				srv.Transport = "sse"
			}
		case "--include":
			srv.Include = splitList(val)
		case "--exclude":
			srv.Exclude = splitList(val)
		default:
			return "", config.MCPServer{}, fmt.Errorf("opción desconocida %s", flag)
		}
	}
	if len(args) == 0 {
		return "", config.MCPServer{}, errors.New(usage)
	}
	isURL := len(args) == 1 && (strings.HasPrefix(args[0], "https://") || strings.HasPrefix(args[0], "http://"))
	if forced == "remote" || (forced == "" && isURL) {
		if !isURL {
			return "", config.MCPServer{}, errors.New("un servidor remote necesita una URL http(s)")
		}
		srv.Type, srv.URL = "remote", args[0]
		return name, srv, nil
	}
	srv.Type, srv.Command = "local", args
	return name, srv, nil
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// mcpAdd prepara la alta y pide confirmación mostrando el comando exacto.
func (m Model) mcpAdd(rest string) Model {
	name, srv, err := parseMCPAdd(rest)
	if err != nil {
		m.toast, m.toastErr = err.Error(), true
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: "MCP: " + err.Error()})
		return m
	}
	if _, exists := m.cfg.MCP.Servers[name]; exists {
		m.toast, m.toastErr = "Ya existe un servidor MCP llamado "+name, true
		return m
	}
	m.mcpPending = &mcpPendingAdd{name: name, server: srv}
	m.confirm = confirmState{title: name, action: "mcpAdd"}
	m.screen = screenConfirm
	return m
}

type mcpPendingAdd struct {
	name   string
	server config.MCPServer
}

// describe muestra lo que se va a ejecutar; los valores de env y headers
// (tokens) nunca se enseñan.
func (p mcpPendingAdd) describe() string {
	var b strings.Builder
	if p.server.Type == "remote" {
		b.WriteString(p.server.URL)
	} else {
		b.WriteString(strings.Join(p.server.Command, " "))
	}
	for _, k := range sortedKeys(p.server.Environment) {
		b.WriteString("\n  env " + k + "=••••")
	}
	for _, k := range sortedKeys(p.server.Headers) {
		b.WriteString("\n  header " + k + ": ••••")
	}
	if p.server.Cwd != "" {
		b.WriteString("\n  cwd " + p.server.Cwd)
	}
	return b.String()
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// mcpAddConfirmed guarda el servidor y lo conecta en segundo plano.
func (m Model) mcpAddConfirmed() Model {
	p := m.mcpPending
	m.mcpPending = nil
	m.screen = screenChat
	if p == nil {
		return m
	}
	if m.cfg.MCP.Servers == nil {
		m.cfg.MCP.Servers = map[string]config.MCPServer{}
	}
	m.cfg.MCP.Servers[p.name] = p.server
	m.cfg.MCP.Enabled = true
	m = m.saveCfg()
	m = m.closeMCP()
	m.mcpLoading = true
	m.toast, m.toastErr = "MCP «"+p.name+"» añadido; probando conexión…", false
	m.mcpTestName = p.name
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: fmt.Sprintf(
		"Servidor MCP «%s» añadido:\n  %s\n\nProbando la conexión… Sus herramientas se envían al modelo con /tools mcp on. Quítalo con /mcp remove %s.",
		p.name, p.describe(), p.name)})
	m = m.saveActive()
	m.layout()
	return m
}

// mcpRemove borra un servidor de la configuración.
func (m Model) mcpRemove(name string) Model {
	name = strings.ToLower(strings.TrimSpace(name))
	if _, ok := m.cfg.MCP.Servers[name]; !ok {
		m.toast, m.toastErr = "No existe el servidor MCP "+name, true
		return m
	}
	delete(m.cfg.MCP.Servers, name)
	m = m.saveCfg()
	m = m.closeMCP()
	if len(m.cfg.MCP.Servers) > 0 {
		m.mcpLoading = true
	}
	m.toast, m.toastErr = "MCP «"+name+"» eliminado", false
	return m
}

func (m Model) mcpAddScreen() []uiLine {
	boxW := clamp(m.width-12, 52, 90)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4
	head := []uiLine{{text: modalTitleStyle.Render("Añadir servidor MCP")}}
	body := []uiLine{
		{text: faintStyle.Render("Nombre   ") + titleStyle.Render(m.confirm.title)},
		{text: warningStyle.Render("Se ejecutará este comando en tu equipo:")},
	}
	if m.mcpPending != nil {
		for _, raw := range strings.Split(m.mcpPending.describe(), "\n") {
			for _, ln := range wrapText(raw, innerW-2) {
				body = append(body, uiLine{text: textStyle.Render(" " + ln)})
			}
		}
	}
	body = append(body, uiLine{}, uiLine{text: faintStyle.Render("Solo acepta si confías en este paquete.")}, uiLine{},
		m.buttonRow(uiButton{"Añadir", "confirmGo", "", false}, uiButton{"Cancelar", "confirmCancel", "", true}))
	return m.centerModal(m.modalFrame(boxW, head, body, "enter añadir · esc cancelar"), boxW)
}
