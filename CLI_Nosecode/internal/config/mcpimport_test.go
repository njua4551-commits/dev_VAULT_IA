package config

import (
	"os"
	"testing"
)

func TestStripJSONC(t *testing.T) {
	in := []byte("{\n // c\n \"a\": \"x//y\", /* b */ \"b\": [1,2,],\n}")
	servers, _, err := ParseForeignMCP([]byte(`{ // hola
 "mcpServers": { "fs": { "command": "npx", "args": ["-y","srv"], "env": {"T":"1"}, }, },
}`))
	if err != nil {
		t.Fatal(err)
	}
	s := servers["fs"]
	if s.Type != "local" || len(s.Command) != 3 || s.Command[0] != "npx" || s.Environment["T"] != "1" {
		t.Fatalf("%+v", s)
	}
	if got := string(StripJSONC(in)); got == "" {
		t.Fatal("vacío")
	}
}

func TestParseForeignFormatos(t *testing.T) {
	// opencode: mcp { nombre: {type, command[]} } y remoto
	s, _, err := ParseForeignMCP([]byte(`{"mcp":{"a":{"type":"local","command":["uvx","x"]},"b":{"type":"remote","url":"https://h/mcp","headers":{"A":"b"}}}}`))
	if err != nil || s["a"].Command[0] != "uvx" || s["b"].URL != "https://h/mcp" || s["b"].Type != "remote" {
		t.Fatalf("%v %+v", err, s)
	}
	// vscode: servers
	s, _, err = ParseForeignMCP([]byte(`{"servers":{"v":{"url":"https://v/mcp"}}}`))
	if err != nil || s["v"].Type != "remote" {
		t.Fatalf("%v %+v", err, s)
	}
	// cursor/claude con disabled
	s, _, err = ParseForeignMCP([]byte(`{"mcpServers":{"c":{"command":"node","args":["a.js"],"disabled":true}}}`))
	if err != nil || s["c"].ImportedServerEnabled() {
		t.Fatalf("%v %+v", err, s)
	}
	if _, _, err = ParseForeignMCP([]byte(`{"x":1}`)); err == nil {
		t.Fatal("debía fallar")
	}
}

func TestToolAllowed(t *testing.T) {
	s := MCPServer{Include: []string{"vm_*"}, Exclude: []string{"vm_delete*"}}
	if !s.ToolAllowed("vm_list") || s.ToolAllowed("vm_delete_all") || s.ToolAllowed("other") {
		t.Fatal("filtros")
	}
	if !(MCPServer{}).ToolAllowed("x") {
		t.Fatal("sin filtros todo pasa")
	}
}

func TestImportMCPFromNoPisa(t *testing.T) {
	t.Setenv("NOSECODE_HOME", t.TempDir())
	c := Defaults()
	c.Path = t.TempDir() + "/config.json"
	c.MCP.Servers = map[string]MCPServer{"fs": {Type: "local", Command: []string{"mio"}}}
	f := t.TempDir() + "/m.json"
	if err := writeFile(f, `{"mcpServers":{"fs":{"command":"otro"},"nuevo":{"command":"x"}}}`); err != nil {
		t.Fatal(err)
	}
	added, dup, _, err := c.ImportMCPFrom(f)
	if err != nil || len(added) != 1 || added[0] != "nuevo" || len(dup) != 1 || c.MCP.Servers["fs"].Command[0] != "mio" {
		t.Fatalf("%v %v %v", err, added, dup)
	}
}

func writeFile(p, s string) error { return os.WriteFile(p, []byte(s), 0o600) }
