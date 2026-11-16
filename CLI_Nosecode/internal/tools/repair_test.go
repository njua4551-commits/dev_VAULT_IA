package tools

import (
	"encoding/json"
	"testing"
)

func TestCanonicalName(t *testing.T) {
	for in, want := range map[string]string{
		"read": "read_file", "functions.bash": "bash", "Shell": "bash", "ls": "list_dir",
		"read_file": "read_file", "default_api:grep": "grep", "mcp__x__y": "mcp__x__y", "write": "write_file",
	} {
		if got := CanonicalName(in); got != want {
			t.Errorf("%q → %q, quería %q", in, got, want)
		}
	}
}

func TestRepairJSON(t *testing.T) {
	cases := map[string]string{
		"```json\n{\"command\": \"ls\"}\n```": `{"command": "ls"}`,
		`{"command": "ls",}`:                  `{"command": "ls"}`,
		`Claro: {"command":"ls"} listo`:       `{"command":"ls"}`,
		`{'command': 'ls'}`:                   `{"command": "ls"}`,
		`"{\"command\":\"ls\"}"`:              `{"command":"ls"}`,
		`{"command":"ls"}`:                    `{"command":"ls"}`,
		`basura sin llaves`:                   `basura sin llaves`,
	}
	for in, want := range cases {
		if got := RepairJSON(in); got != want {
			t.Errorf("RepairJSON(%q) = %q, quería %q", in, got, want)
		}
	}
}

func TestNormalizeCallArgs(t *testing.T) {
	name, args := NormalizeCall("run", `{"cmd": "ls -la",}`)
	var m map[string]string
	if err := json.Unmarshal([]byte(args), &m); err != nil || name != "bash" || m["command"] != "ls -la" {
		t.Fatalf("%s %s %v", name, args, err)
	}
	name, args = NormalizeCall("edit", `{"file_path":"a","old_string":"x","new_string":"y"}`)
	json.Unmarshal([]byte(args), &m)
	if name != "edit_file" || m["path"] != "a" || m["old"] != "x" || m["new"] != "y" {
		t.Fatalf("%s %s", name, args)
	}
	// no pisa un argumento correcto
	_, args = NormalizeCall("bash", `{"command":"a","cmd":"b"}`)
	json.Unmarshal([]byte(args), &m)
	if m["command"] != "a" {
		t.Fatalf("pisó command: %s", args)
	}
	// herramienta MCP: intacta
	n, a := NormalizeCall("srv__tool", `{"x":1}`)
	if n != "srv__tool" || a != `{"x":1}` {
		t.Fatalf("%s %s", n, a)
	}
}
