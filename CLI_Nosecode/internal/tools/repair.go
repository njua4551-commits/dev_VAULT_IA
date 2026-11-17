package tools

import (
	"encoding/json"
	"regexp"
	"strings"
)

// Los modelos pequeños suelen equivocarse en el nombre de la herramienta, en
// los nombres de los argumentos o en el JSON. NormalizeCall lo corrige antes
// de ejecutar, sin cambiar el significado de la llamada.

var toolAliases = map[string]string{
	"read": "read_file", "cat": "read_file", "open": "read_file", "readfile": "read_file", "file_read": "read_file",
	"write": "write_file", "create_file": "write_file", "writefile": "write_file", "file_write": "write_file", "save_file": "write_file",
	"edit": "edit_file", "replace": "edit_file", "editfile": "edit_file", "str_replace": "edit_file", "patch": "apply_patch", "applypatch": "apply_patch", "apply_diff": "apply_patch",
	"run": "bash", "shell": "bash", "sh": "bash", "exec": "bash", "cmd": "bash", "execute": "bash", "run_command": "bash", "command": "bash",
	"ls": "list_dir", "list": "list_dir", "dir": "list_dir", "listdir": "list_dir", "list_files": "list_dir", "list_directory": "list_dir",
	"search": "grep", "find": "grep", "rg": "grep", "search_files": "grep",
	"fetch": "web_fetch", "web": "web_fetch", "browse": "web_fetch", "http_get": "web_fetch", "fetch_url": "web_fetch", "webfetch": "web_fetch",
	"ask": "question", "ask_user": "question",
	"todowrite": "todo", "todo_write": "todo", "todos": "todo", "tasks": "todo", "plan": "todo",
	"view": "view_image", "image": "view_image",
}

var argAliases = map[string]map[string][]string{
	"read_file":  {"path": {"file_path", "filepath", "filename", "file", "filePath", "target"}},
	"write_file": {"path": {"file_path", "filepath", "filename", "file", "filePath"}, "content": {"contents", "text", "data", "body", "code"}},
	"edit_file": {"path": {"file_path", "filepath", "filename", "file", "filePath"},
		"old": {"old_string", "oldString", "old_text", "search", "find", "original"},
		"new": {"new_string", "newString", "new_text", "replace", "replacement"}},
	"list_dir":   {"path": {"dir", "directory", "folder", "dir_path", "file_path", "target"}},
	"view_image": {"path": {"file_path", "filepath", "filename", "file", "image", "image_path"}},
	"bash":       {"command": {"cmd", "script", "shell_command", "code", "input", "commands"}},
	"web_fetch":  {"url": {"link", "uri", "href", "page", "address"}},
	"grep":       {"pattern": {"query", "regex", "search", "text", "term"}, "path": {"dir", "directory", "folder", "file_path"}},
	"todo":       {"todos": {"tasks", "items", "list", "plan"}},
	"question":   {"question": {"text", "prompt", "message"}, "options": {"choices", "answers"}},
}

// CanonicalName quita prefijos habituales y resuelve sinónimos.
func CanonicalName(name string) string {
	n := strings.TrimSpace(name)
	for _, p := range []string{"functions.", "function.", "tools.", "tool.", "default_api:", "default_api.", "native."} {
		n = strings.TrimPrefix(n, p)
	}
	if _, ok := registry[n]; ok {
		return n
	}
	low := strings.ToLower(n)
	if _, ok := registry[low]; ok {
		return low
	}
	if c, ok := toolAliases[low]; ok {
		return c
	}
	return name // MCP u otra herramienta: se deja tal cual
}

var trailingComma = regexp.MustCompile(`,\s*([}\]])`)

// RepairJSON intenta arreglar el JSON de argumentos; si no puede, lo devuelve igual.
func RepairJSON(s string) string {
	t := strings.TrimSpace(s)
	if t == "" || json.Valid([]byte(t)) {
		var str string
		if json.Unmarshal([]byte(t), &str) == nil && strings.HasPrefix(strings.TrimSpace(str), "{") && json.Valid([]byte(str)) {
			return str // JSON codificado dentro de un string
		}
		return s
	}
	t = strings.TrimPrefix(t, "```json")
	t = strings.TrimPrefix(t, "```")
	t = strings.TrimSuffix(strings.TrimSpace(t), "```")
	if i, j := strings.Index(t, "{"), strings.LastIndex(t, "}"); i >= 0 && j > i {
		t = t[i : j+1]
	}
	t = trailingComma.ReplaceAllString(t, "$1")
	if json.Valid([]byte(t)) {
		return t
	}
	if !strings.Contains(t, `"`) {
		if u := strings.ReplaceAll(t, "'", `"`); json.Valid([]byte(u)) {
			return u
		}
	}
	return s
}

// NormalizeCall devuelve el nombre canónico y los argumentos reparados.
func NormalizeCall(name, args string) (string, string) {
	canon := CanonicalName(name)
	args = RepairJSON(args)
	aliases, ok := argAliases[canon]
	if !ok {
		return canon, args
	}
	var m map[string]json.RawMessage
	if json.Unmarshal([]byte(args), &m) != nil || m == nil {
		return canon, args
	}
	changed := false
	for want, alts := range aliases {
		if _, has := m[want]; has {
			continue
		}
		for _, alt := range alts {
			if v, has := m[alt]; has {
				m[want] = v
				delete(m, alt)
				changed = true
				break
			}
		}
	}
	if !changed {
		return canon, args
	}
	out, err := json.Marshal(m)
	if err != nil {
		return canon, args
	}
	return canon, string(out)
}
