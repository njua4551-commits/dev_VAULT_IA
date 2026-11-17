package ui

import (
	"strings"

	"nosecode/internal/config"
	"nosecode/internal/tools"
)

// applySandbox pasa la configuración al paquete tools.
func applySandbox(cfg *config.Config) {
	tools.SetSandbox(tools.SandboxConfig{Mode: cfg.Sandbox.Mode, NoNetwork: cfg.Sandbox.NoNetwork, Writable: cfg.Sandbox.Writable})
	tools.SetPersistentShell(cfg.Tools.PersistentShell)
}

// shellSlash: /shell [on|off|reset|status].
func (m Model) shellSlash(args []string) Model {
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "on":
			m.cfg.Tools.PersistentShell = true
		case "off":
			m.cfg.Tools.PersistentShell = false
		case "reset", "reiniciar":
			tools.ResetShell()
		case "status", "estado":
		default:
			m.toast, m.toastErr = "Uso: /shell [on|off|reset|status]", true
			return m
		}
		_ = m.cfg.Save()
		applySandbox(m.cfg)
	}
	var b strings.Builder
	switch {
	case !tools.PersistentShellOn():
		b.WriteString("Shell persistente: desactivado (cada comando de bash es independiente). /shell on para activarlo.")
	default:
		alive, dir := tools.ShellStatus()
		b.WriteString("Shell persistente: activado. cd, variables exportadas y entornos virtuales (source .venv/bin/activate) se mantienen entre comandos.")
		if alive {
			b.WriteString("\nDirectorio actual del shell: " + dir)
		} else {
			b.WriteString("\nAún no hay shell abierto (se crea con el primer comando).")
		}
		b.WriteString("\n\n/shell reset → reiniciar (pierde variables)  ·  /shell off → desactivar\nLímites: sin programas interactivos (no hay PTY); cancelar un comando reinicia el shell; no disponible en Windows.")
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: b.String()})
	return m
}

// sandboxPrompt avisa al modelo de las restricciones cuando el sandbox está activo.
func sandboxPrompt() string {
	_, active, _ := tools.SandboxState()
	if !active {
		return ""
	}
	return "\n\nSandbox activo: bash solo puede escribir en la carpeta del proyecto, /tmp y cachés; el resto del disco es de solo lectura " +
		"y las claves/configuración (~/.ssh, ~/.aws, Nosecode…) están ocultas. sudo no funciona. Si un comando falla por «read-only file system» " +
		"o permiso denegado fuera del proyecto, no lo intentes de otra forma: explícaselo al usuario (puede cambiarlo con /sandbox)."
}

// sandboxSlash: /sandbox [off|auto|on|net on|net off|status]
func (m Model) sandboxSlash(args []string) Model {
	say := func(t string) Model { m.msgs = append(m.msgs, Message{Role: "assistant", Text: t}); return m }
	if len(args) > 0 {
		switch strings.ToLower(args[0]) {
		case "off", "auto", "on":
			m.cfg.Sandbox.Mode = strings.ToLower(args[0])
			if m.cfg.Sandbox.Mode == "off" {
				m.cfg.Sandbox.Mode = ""
			}
		case "net", "red":
			if len(args) < 2 || (args[1] != "on" && args[1] != "off") {
				m.toast, m.toastErr = "Uso: /sandbox net on|off (on = con red)", true
				return m
			}
			m.cfg.Sandbox.NoNetwork = args[1] == "off"
		case "status", "estado":
		default:
			m.toast, m.toastErr = "Uso: /sandbox [off|auto|on|net on|net off|status]", true
			return m
		}
		_ = m.cfg.Save()
		applySandbox(m.cfg)
	}
	mode, active, note := tools.SandboxState()
	var b strings.Builder
	b.WriteString("Sandbox de bash: modo " + mode)
	if active {
		b.WriteString(" · ACTIVO (" + note + ")\n\nSolo se escribe en el proyecto, /tmp y cachés (~/.cache, ~/go, ~/.npm, ~/.cargo); lo demás es de solo lectura y los secretos (~/.ssh, ~/.aws, config de Nosecode) están ocultos. sudo no funciona dentro.")
	} else if mode == "off" {
		b.WriteString(" · desactivado\n\n/sandbox auto  → aislar si hay herramienta (bubblewrap en Linux, sandbox-exec en macOS)\n/sandbox on    → obligatorio: sin herramienta, bash se niega\n/sandbox net off → además sin red")
	} else {
		b.WriteString(" · NO se puede aplicar: " + note)
		if mode == "on" {
			b.WriteString("\nEn modo «on» bash se negará a ejecutar hasta resolverlo (o usa /sandbox auto|off).")
		}
	}
	if len(m.cfg.Sandbox.Writable) > 0 {
		b.WriteString("\nRutas extra con escritura: " + strings.Join(m.cfg.Sandbox.Writable, ", "))
	}
	b.WriteString("\nLo que NO cubre: write_file/edit_file (tienen sus propios permisos), servidores MCP y hooks.")
	return say(b.String())
}
