package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
)

// ToolAllowed aplica include/exclude (patrones glob sobre el nombre original
// de la herramienta) del servidor.
func (s MCPServer) ToolAllowed(tool string) bool {
	match := func(list []string) bool {
		for _, p := range list {
			if ok, _ := path.Match(strings.TrimSpace(p), tool); ok {
				return true
			}
		}
		return false
	}
	if len(s.Include) > 0 && !match(s.Include) {
		return false
	}
	return !match(s.Exclude)
}

// ImportSource es un archivo de configuración MCP de otra aplicación.
type ImportSource struct {
	Name string
	Path string
}

// ImportSources lista los lugares habituales donde otras apps guardan sus
// servidores MCP (solo los que existen se usan al importar).
func ImportSources(cwd string) []ImportSource {
	home, _ := os.UserHomeDir()
	var list []ImportSource
	add := func(name, p string) {
		if p != "" {
			list = append(list, ImportSource{name, p})
		}
	}
	add("opencode", OpenCodeConfig())
	if cwd != "" {
		add("vscode", filepath.Join(cwd, ".vscode", "mcp.json"))
		add("proyecto", filepath.Join(cwd, ".mcp.json"))
	}
	if home != "" {
		add("cursor", filepath.Join(home, ".cursor", "mcp.json"))
		add("claude-code", filepath.Join(home, ".claude.json"))
		switch runtime.GOOS {
		case "windows":
			add("claude", filepath.Join(os.Getenv("APPDATA"), "Claude", "claude_desktop_config.json"))
		case "darwin":
			add("claude", filepath.Join(home, "Library", "Application Support", "Claude", "claude_desktop_config.json"))
		default:
			add("claude", filepath.Join(home, ".config", "Claude", "claude_desktop_config.json"))
		}
	}
	return list
}

// StripJSONC quita comentarios // y /* */ y comas finales para aceptar JSONC.
func StripJSONC(in []byte) []byte {
	out := make([]byte, 0, len(in))
	inStr, esc := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		if inStr {
			out = append(out, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		switch {
		case c == '"':
			inStr = true
			out = append(out, c)
		case c == '/' && i+1 < len(in) && in[i+1] == '/':
			for i < len(in) && in[i] != '\n' {
				i++
			}
			out = append(out, '\n')
		case c == '/' && i+1 < len(in) && in[i+1] == '*':
			i += 2
			for i+1 < len(in) && !(in[i] == '*' && in[i+1] == '/') {
				i++
			}
			i++
		default:
			out = append(out, c)
		}
	}
	// comas finales: , seguido (tras espacios) de } o ]
	res := make([]byte, 0, len(out))
	inStr, esc = false, false
	for i := 0; i < len(out); i++ {
		c := out[i]
		if inStr {
			res = append(res, c)
			switch {
			case esc:
				esc = false
			case c == '\\':
				esc = true
			case c == '"':
				inStr = false
			}
			continue
		}
		if c == '"' {
			inStr = true
		}
		if c == ',' {
			j := i + 1
			for j < len(out) && (out[j] == ' ' || out[j] == '\n' || out[j] == '\t' || out[j] == '\r') {
				j++
			}
			if j < len(out) && (out[j] == '}' || out[j] == ']') {
				continue
			}
		}
		res = append(res, c)
	}
	return res
}

type foreignServer struct {
	Type        string            `json:"type"`
	Command     json.RawMessage   `json:"command"`
	Args        []string          `json:"args"`
	Env         map[string]string `json:"env"`
	Environment map[string]string `json:"environment"`
	Cwd         string            `json:"cwd"`
	URL         string            `json:"url"`
	ServerURL   string            `json:"serverUrl"`
	Headers     map[string]string `json:"headers"`
	Enabled     *bool             `json:"enabled"`
	Disabled    bool              `json:"disabled"`
	Timeout     int               `json:"timeout"`
	Include     []string          `json:"include"`
	Exclude     []string          `json:"exclude"`
}

func (f foreignServer) convert() (MCPServer, error) {
	s := MCPServer{
		Cwd: f.Cwd, Headers: f.Headers, Enabled: f.Enabled, Timeout: f.Timeout,
		Include: f.Include, Exclude: f.Exclude,
	}
	if f.Disabled {
		off := false
		s.Enabled = &off
	}
	s.Environment = f.Environment
	if len(f.Env) > 0 {
		if s.Environment == nil {
			s.Environment = map[string]string{}
		}
		for k, v := range f.Env {
			s.Environment[k] = v
		}
	}
	// command: "npx" + args  |  ["npx","-y",...]
	if len(f.Command) > 0 {
		var one string
		var many []string
		switch {
		case json.Unmarshal(f.Command, &many) == nil:
			s.Command = many
		case json.Unmarshal(f.Command, &one) == nil && strings.TrimSpace(one) != "":
			s.Command = append([]string{one}, f.Args...)
		}
	}
	url := f.URL
	if url == "" {
		url = f.ServerURL
	}
	switch {
	case url != "":
		s.Type, s.URL = "remote", url
	case len(s.Command) > 0:
		s.Type = "local"
	default:
		return s, errors.New("sin command ni url")
	}
	return s, nil
}

// ParseForeignMCP lee servidores MCP de los formatos de OpenCode, Claude
// Desktop/Code, Cursor y VS Code (con o sin comentarios).
func ParseForeignMCP(raw []byte) (map[string]MCPServer, []string, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(StripJSONC(raw), &root); err != nil {
		return nil, nil, fmt.Errorf("JSON no válido: %w", err)
	}
	var group map[string]json.RawMessage
	for _, key := range []string{"mcpServers", "servers"} {
		if v, ok := root[key]; ok {
			if json.Unmarshal(v, &group) == nil && len(group) > 0 {
				break
			}
		}
	}
	if len(group) == 0 {
		if v, ok := root["mcp"]; ok {
			var mcp map[string]json.RawMessage
			if json.Unmarshal(v, &mcp) == nil {
				if inner, ok := mcp["servers"]; ok {
					var g map[string]json.RawMessage
					if json.Unmarshal(inner, &g) == nil && len(g) > 0 {
						group = g
					}
				}
				if len(group) == 0 {
					group = mcp
				}
			}
		}
	}
	if len(group) == 0 {
		return nil, nil, errors.New("no hay servidores MCP en ese archivo")
	}
	out := map[string]MCPServer{}
	var skipped []string
	names := make([]string, 0, len(group))
	for n := range group {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, name := range names {
		var f foreignServer
		if err := json.Unmarshal(group[name], &f); err != nil {
			skipped = append(skipped, name+": "+err.Error())
			continue
		}
		s, err := f.convert()
		if err != nil {
			skipped = append(skipped, name+": "+err.Error())
			continue
		}
		out[name] = s
	}
	if len(out) == 0 {
		return nil, skipped, errors.New("ningún servidor válido")
	}
	return out, skipped, nil
}

// ImportMCPFrom importa desde un archivo concreto sin pisar servidores que ya
// existen: devuelve los nombres añadidos y los omitidos por duplicado.
func (c *Config) ImportMCPFrom(file string) (added, duplicated, invalid []string, err error) {
	raw, err := os.ReadFile(file)
	if err != nil {
		return nil, nil, nil, err
	}
	servers, bad, err := ParseForeignMCP(raw)
	if err != nil {
		return nil, nil, bad, err
	}
	if c.MCP.Servers == nil {
		c.MCP.Servers = map[string]MCPServer{}
	}
	names := make([]string, 0, len(servers))
	for n := range servers {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		if _, exists := c.MCP.Servers[n]; exists {
			duplicated = append(duplicated, n)
			continue
		}
		c.MCP.Servers[n] = servers[n]
		added = append(added, n)
	}
	if len(added) > 0 {
		c.MCP.Enabled = true
		if err := c.Save(); err != nil {
			return added, duplicated, bad, err
		}
	}
	return added, duplicated, bad, nil
}
