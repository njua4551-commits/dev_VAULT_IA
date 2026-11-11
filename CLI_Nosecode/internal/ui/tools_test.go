package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nosecode/internal/api"
	"nosecode/internal/mcp"
)

func defNames(defs []api.ToolDef) []string {
	out := make([]string, 0, len(defs))
	for _, d := range defs {
		out = append(out, d.Function.Name)
	}
	return out
}

func hasTool(defs []api.ToolDef, name string) bool {
	for _, d := range defs {
		if d.Function.Name == name {
			return true
		}
	}
	return false
}

func TestApiToolsNativasPorDefecto(t *testing.T) {
	m := newTestModel(t)
	m.mcpTools = []mcp.Tool{
		{Server: "x", Name: "foo", Wire: "x__foo", Description: "foo"},
		{Server: "playwright", Name: "browser_navigate", Wire: "playwright__browser_navigate", Description: "nav"},
	}
	defs := m.apiTools()
	if hasTool(defs, "x__foo") || hasTool(defs, "browser_navigate") {
		t.Fatalf("MCP no debe enviarse por defecto: %v", defNames(defs))
	}
	for _, name := range []string{"bash", "read_file", "write_file", "edit_file", "list_dir", "grep", "screenshot", "view_image", "web_fetch", "remember"} {
		if !hasTool(defs, name) {
			t.Fatalf("falta %s en %v", name, defNames(defs))
		}
	}
}

func TestApiToolsBrowserSoloConToggle(t *testing.T) {
	m := newTestModel(t)
	m.mcpTools = []mcp.Tool{
		{Server: "playwright", Name: "browser_navigate", Wire: "playwright__browser_navigate", Description: "nav"},
		{Server: "playwright", Name: "browser_click", Wire: "playwright__browser_click", Description: "click"},
		{Server: "playwright", Name: "browser_type", Wire: "playwright__browser_type", Description: "type"},
		{Server: "playwright", Name: "browser_snapshot", Wire: "playwright__browser_snapshot", Description: "snap"},
	}
	m.cfg.Tools.Browser = true
	defs := m.apiTools()
	for _, name := range []string{"browser_navigate", "browser_click", "browser_type"} {
		if !hasTool(defs, name) {
			t.Fatalf("falta %s en %v", name, defNames(defs))
		}
	}
	if hasTool(defs, "browser_snapshot") || hasTool(defs, "playwright__browser_navigate") {
		t.Fatalf("no debe enviar otras herramientas MCP: %v", defNames(defs))
	}
}

func TestApiToolsMCPOnSinNavegador(t *testing.T) {
	m := newTestModel(t)
	m.mcpTools = []mcp.Tool{
		{Server: "x", Name: "foo", Wire: "x__foo", Description: "foo"},
		{Server: "playwright", Name: "browser_navigate", Wire: "playwright__browser_navigate", Description: "nav"},
	}
	m.cfg.Tools.MCP = true
	defs := m.apiTools()
	if !hasTool(defs, "x__foo") {
		t.Fatalf("falta x__foo en %v", defNames(defs))
	}
	if hasTool(defs, "browser_navigate") || hasTool(defs, "playwright__browser_navigate") {
		t.Fatalf("navegador debe seguir apagado: %v", defNames(defs))
	}
}

func TestMicrocompactConservaUltimas(t *testing.T) {
	msgs := []api.ChatMessage{
		{Role: "system", Content: "s"},
		{Role: "tool", Content: "uno"},
		{Role: "assistant", Content: "a"},
		{Role: "tool", Content: "dos"},
		{Role: "tool", Content: "tres"},
		{Role: "tool", Content: "cuatro"},
	}
	microcompact(msgs, 2)
	if msgs[1].Content != "[salida borrada]" || msgs[3].Content != "[salida borrada]" {
		t.Fatalf("no compactó: %+v", msgs)
	}
	if msgs[4].Content != "tres" || msgs[5].Content != "cuatro" {
		t.Fatalf("borró de más: %+v", msgs)
	}
}

func TestSystemPromptIncluyeMemoria(t *testing.T) {
	m := newTestModel(t)
	path := filepath.Join(t.TempDir(), "NOSECODE.md")
	if err := os.WriteFile(path, []byte("- recuerda 131072\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NOSECODE_MEMORY", path)
	got := m.systemPrompt()
	if !strings.Contains(got, "NOSECODE.md") || !strings.Contains(got, "recuerda 131072") {
		t.Fatalf("prompt = %q", got)
	}
}
