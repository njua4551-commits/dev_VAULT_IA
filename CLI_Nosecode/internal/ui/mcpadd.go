package ui

import (
	"errors"
	"fmt"
	"regexp"
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

// parseMCPAdd valida «nombre comando args…» o «nombre https://url».
func parseMCPAdd(rest string) (string, config.MCPServer, error) {
	parts, err := splitCommand(rest)
	if err != nil {
		return "", config.MCPServer{}, err
	}
	if len(parts) < 2 {
		return "", config.MCPServer{}, errors.New("uso: /mcp add <nombre> <comando [args…]> o /mcp add <nombre> <https://url>")
	}
	name := strings.ToLower(parts[0])
	if !mcpNameRe.MatchString(name) {
		return "", config.MCPServer{}, errors.New("nombre inválido: usa a-z, 0-9, - y _ (máx. 32)")
	}
	if len(parts) == 2 && (strings.HasPrefix(parts[1], "https://") || strings.HasPrefix(parts[1], "http://")) {
		return name, config.MCPServer{Type: "remote", URL: parts[1]}, nil
	}
	return name, config.MCPServer{Type: "local", Command: parts[1:]}, nil
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

func (p mcpPendingAdd) describe() string {
	if p.server.Type == "remote" {
		return p.server.URL
	}
	return strings.Join(p.server.Command, " ")
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
	m.toast, m.toastErr = "MCP «"+p.name+"» añadido; se conecta en el próximo mensaje", false
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: fmt.Sprintf(
		"Servidor MCP «%s» añadido:\n  %s\n\nSe conectará en segundo plano. Sus herramientas se envían al modelo con /tools mcp on. Quítalo con /mcp remove %s.",
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
		for _, ln := range wrapText(m.mcpPending.describe(), innerW-2) {
			body = append(body, uiLine{text: textStyle.Render(" " + ln)})
		}
	}
	body = append(body, uiLine{}, uiLine{text: faintStyle.Render("Solo acepta si confías en este paquete.")}, uiLine{},
		m.buttonRow(uiButton{"Añadir", "confirmGo", "", false}, uiButton{"Cancelar", "confirmCancel", "", true}))
	return m.centerModal(m.modalFrame(boxW, head, body, "enter añadir · esc cancelar"), boxW)
}
