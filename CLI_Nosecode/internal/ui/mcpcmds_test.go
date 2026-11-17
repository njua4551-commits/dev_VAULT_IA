package ui

import (
	"strings"
	"testing"

	"nosecode/internal/api"
	"nosecode/internal/config"
	"nosecode/internal/mcp"
)

func TestParseMCPAddFlags(t *testing.T) {
	name, s, err := parseMCPAdd(`fs --env TOKEN=abc --cwd /x --timeout 20 --include "a*,b" -- npx -y srv .`)
	if err != nil || name != "fs" || s.Type != "local" || len(s.Command) != 4 || s.Command[0] != "npx" {
		t.Fatalf("%v %s %+v", err, name, s)
	}
	if s.Environment["TOKEN"] != "abc" || s.Cwd != "/x" || s.Timeout != 20000 || len(s.Include) != 2 {
		t.Fatalf("%+v", s)
	}
	_, r, err := parseMCPAdd(`api --header "Authorization: Bearer zzz" https://h/mcp`)
	if err != nil || r.Type != "remote" || r.Headers["Authorization"] != "Bearer zzz" {
		t.Fatalf("%v %+v", err, r)
	}
	_, j, err := parseMCPAdd(`pegado {"command":"npx","args":["-y","s"],"env":{"K":"v"}}`)
	if err != nil || len(j.Command) != 3 || j.Environment["K"] != "v" {
		t.Fatalf("%v %+v", err, j)
	}
	if _, _, err := parseMCPAdd(`x --raro 1 cmd`); err == nil {
		t.Fatal("opción desconocida debía fallar")
	}
	d := mcpPendingAdd{name: "fs", server: s}.describe()
	if strings.Contains(d, "abc") || !strings.Contains(d, "TOKEN=••••") {
		t.Fatalf("el token no debe verse: %q", d)
	}
}

func TestMCPFiltrosYPermisos(t *testing.T) {
	m := newTestModel(t)
	m.cfg.MCP.Servers = map[string]config.MCPServer{
		"vm": {Type: "local", Command: []string{"x"}, Exclude: []string{"vm_del*"}, Permission: "allow"},
	}
	m.mcpTools = []mcp.Tool{
		{Server: "vm", Name: "vm_list", Wire: "vm__vm_list"},
		{Server: "vm", Name: "vm_delete", Wire: "vm__vm_delete"},
	}
	if !m.mcpToolVisible(m.mcpTools[0]) || m.mcpToolVisible(m.mcpTools[1]) {
		t.Fatal("exclude")
	}
	var call api.ToolCall
	call.Function.Name, call.Function.Arguments = "vm__vm_list", "{}"
	if m.callNeedsApproval(call) {
		t.Fatal("permiso allow por servidor no debía pedir aprobación")
	}
	s := m.cfg.MCP.Servers["vm"]
	s.Permission = "ask"
	m.cfg.MCP.Servers["vm"] = s
	if !m.callNeedsApproval(call) {
		t.Fatal("permiso ask por servidor debía pedir aprobación")
	}
	s.Permission = "deny"
	m.cfg.MCP.Servers["vm"] = s
	if m.mcpToolVisible(m.mcpTools[0]) {
		t.Fatal("deny oculta las herramientas")
	}
	txt := m.mcpStatusText()
	if !strings.Contains(txt, "• vm (local)") || !strings.Contains(txt, "permiso deny") {
		t.Fatal(txt)
	}
}
