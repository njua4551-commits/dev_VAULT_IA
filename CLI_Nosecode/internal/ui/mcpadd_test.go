package ui

import (
	"reflect"
	"testing"
)

func TestSplitCommand(t *testing.T) {
	got, err := splitCommand(`npx -y "@scope/pkg name" 'a b' c`)
	want := []string{"npx", "-y", "@scope/pkg name", "a b", "c"}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("%v %v", got, err)
	}
	if _, err := splitCommand(`a "b`); err == nil {
		t.Fatal("comillas sin cerrar")
	}
}

func TestParseMCPAdd(t *testing.T) {
	n, s, err := parseMCPAdd("fs npx -y @modelcontextprotocol/server-filesystem .")
	if err != nil || n != "fs" || s.Type != "local" || len(s.Command) != 4 {
		t.Fatalf("%v %v %v", n, s, err)
	}
	_, s, err = parseMCPAdd("web https://example.com/mcp")
	if err != nil || s.Type != "remote" || s.URL == "" {
		t.Fatalf("%v %v", s, err)
	}
	for _, bad := range []string{"", "solo", "Mal/Nombre cmd", "../x cmd"} {
		if _, _, err := parseMCPAdd(bad); err == nil {
			t.Errorf("%q debe fallar", bad)
		}
	}
}

func TestMCPAddFlujo(t *testing.T) {
	m := newTestModel(t)
	m = m.mcpAdd("fs echo hola")
	if m.screen != screenConfirm || m.mcpPending == nil {
		t.Fatal("debe pedir confirmación")
	}
	if len(m.cfg.MCP.Servers) != 0 {
		t.Fatal("no debe guardar antes de confirmar")
	}
	if out := m.View(); out == "" {
		t.Fatal("vista vacía")
	}
	m = m.mcpAddConfirmed()
	if _, ok := m.cfg.MCP.Servers["fs"]; !ok || !m.cfg.MCP.Enabled {
		t.Fatal("no se guardó")
	}
	m = m.mcpRemove("fs")
	if _, ok := m.cfg.MCP.Servers["fs"]; ok {
		t.Fatal("no se quitó")
	}
}
