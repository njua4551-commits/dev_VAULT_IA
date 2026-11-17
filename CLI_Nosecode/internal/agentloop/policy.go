package agentloop

import (
	"encoding/json"

	"nosecode/internal/api"
	"nosecode/internal/config"
	"nosecode/internal/tools"
)

// ReadOnlyTools son las herramientas sin efectos (Plan y subagentes).
var ReadOnlyTools = map[string]bool{
	"read_file": true, "list_dir": true, "grep": true, "web_fetch": true,
	"view_image": true, "skill": true, "repo_map": true, "todo": true, "task": true, "diagnostics": true, "lsp": true,
}

// NewPolicy arma la política de un modo sin interfaz:
//   - readOnly: solo herramientas de lectura y bash de solo lectura.
//   - yes: aprueba lo que en la TUI pediría permiso (editar, bash), salvo
//     lo que las reglas deny, sudo o permissions=deny prohíban.
//   - sin yes: lo que pediría permiso se deniega con un mensaje claro.
func NewPolicy(cfg *config.Config, cwd string, readOnly, yes bool) Policy {
	return func(call api.ToolCall) (bool, string) {
		name := call.Function.Name
		if readOnly && !ReadOnlyTools[name] && name != "bash" {
			return false, "modo solo lectura: " + name + " no está permitido"
		}
		switch name {
		case "bash":
			var a struct {
				Command string `json:"command"`
			}
			_ = json.Unmarshal([]byte(call.Function.Arguments), &a)
			if cfg.Permissions.Bash == "deny" {
				return false, "bash está desactivado (permissions.bash = deny)"
			}
			if tools.UsesSudo(a.Command) {
				return false, "sudo no está disponible sin interfaz"
			}
			d := tools.DecideBash(a.Command, cfg.Permissions.BashRules, cwd)
			if d == "deny" {
				return false, "bloqueado por permissions.bash_rules"
			}
			if readOnly {
				if tools.BashReadOnly(a.Command, cwd) {
					return true, ""
				}
				return false, "modo solo lectura: solo comandos de lectura (ls, cat, git status/diff/log…)"
			}
			if d == "allow" || yes || cfg.Permissions.Bash == "allow" {
				return true, ""
			}
			return false, "este comando necesita aprobación; ejecuta con --yes o añade una regla allow"
		case "terminal":
			var a struct {
				Action  string `json:"action"`
				Command string `json:"command"`
			}
			_ = json.Unmarshal([]byte(call.Function.Arguments), &a)
			if a.Action != "open" && !(a.Action == "" && a.Command != "") {
				return true, "" // send/read/close: la sesión ya se aprobó al abrirla
			}
			if cfg.Permissions.Bash == "deny" {
				return false, "terminal está desactivada (permissions.bash = deny)"
			}
			if tools.DecideBash(a.Command, cfg.Permissions.BashRules, cwd) == "deny" {
				return false, "bloqueado por permissions.bash_rules"
			}
			if yes || cfg.Permissions.Bash == "allow" {
				return true, ""
			}
			return false, "abrir una terminal necesita aprobación; ejecuta con --yes"
		case "write_file", "edit_file", "apply_patch":
			if cfg.Permissions.Edit == "deny" {
				return false, "editar está desactivado (permissions.edit = deny)"
			}
			if yes || cfg.Permissions.Edit == "allow" {
				return true, ""
			}
			return false, "editar necesita aprobación; ejecuta con --yes"
		case "web_fetch":
			if cfg.Permissions.Webfetch == "deny" {
				return false, "web_fetch desactivado"
			}
		}
		return true, ""
	}
}
