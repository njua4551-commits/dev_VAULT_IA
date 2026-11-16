package tools

import (
	"strings"

	"nosecode/internal/config"
)

// DecideBash aplica permissions.bash_rules en orden. Una regla allow solo
// auto-aprueba comandos de la lista cerrada de solo lectura y sin
// metacaracteres de shell.
func DecideBash(command string, rules []config.BashRule, cwd string) string {
	command = normalizeBash(command)
	if command == "" {
		return "ask"
	}
	for _, rule := range rules {
		if !bashPatternMatch(rule.Pattern, command) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(rule.Action)) {
		case "allow":
			if BashReadOnly(command, cwd) && !bashPatternForbidden(rule.Pattern) {
				return "allow"
			}
			return "ask"
		case "deny":
			return "deny"
		default:
			return "ask"
		}
	}
	return "ask"
}

// BashReadOnly comprueba la lista cerrada de comandos que no modifican nada.
func BashReadOnly(command, cwd string) bool {
	command = normalizeBash(command)
	if command == "" || bashHasMeta(command) {
		return false
	}
	switch command {
	case "pwd", "ls", "ls -la", "git status", "git diff", "git log", "uname -a", "whoami":
		return true
	}
	fields := strings.Fields(command)
	if len(fields) == 2 && fields[0] == "cat" {
		path := fields[1]
		if path == "" || strings.HasPrefix(path, "-") {
			return false
		}
		check := CheckReadPath(path, cwd)
		return !check.Sensitive
	}
	return false
}

// bashPatternMatch soporta coincidencia exacta y «prefijo *» con un solo
// argumento (por ejemplo «cat *»). No hay comodines generales.
func bashPatternMatch(pattern, command string) bool {
	pattern = normalizeBash(pattern)
	if pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasSuffix(pattern, " *") {
		prefix := strings.TrimSuffix(pattern, " *")
		if !strings.HasPrefix(command, prefix+" ") {
			return false
		}
		rest := strings.TrimPrefix(command, prefix+" ")
		return rest != "" && len(strings.Fields(rest)) == 1
	}
	return pattern == command
}

// bashHasMeta rechaza cualquier composición de shell.
func bashHasMeta(command string) bool {
	return strings.ContainsAny(command, ";&|`$()<>\n\r\"'\\*?[]{}~!#")
}

// bashPatternForbidden evita que una regla allow abra intérpretes o comandos
// destructivos aunque el usuario la escriba en su configuración.
func bashPatternForbidden(pattern string) bool {
	p := strings.ToLower(normalizeBash(pattern))
	fields := strings.Fields(p)
	if len(fields) == 0 {
		return true
	}
	forbidden := map[string]bool{
		"python": true, "python3": true, "node": true, "nodejs": true,
		"sh": true, "bash": true, "zsh": true, "pwsh": true, "powershell": true,
		"curl": true, "wget": true, "pip": true, "pip3": true,
		"npm": true, "pnpm": true, "yarn": true,
		"rm": true, "sudo": true, "dd": true, "mkfs": true,
		"chmod": true, "chown": true, "mount": true, "umount": true,
		"systemctl": true, "kill": true, "pkill": true,
		"shutdown": true, "reboot": true, "poweroff": true,
		"eval": true, "exec": true, "source": true, "env": true,
	}
	if forbidden[fields[0]] {
		return true
	}
	for _, prefix := range []string{"git push", "git reset --hard", "git clean", "chmod -r"} {
		if p == prefix || strings.HasPrefix(p, prefix+" ") {
			return true
		}
	}
	return false
}

func normalizeBash(s string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
}
