package ui

import (
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// thinkingSetting traduce ui.thinking a enable_thinking (nil = el servidor decide).
func (m Model) thinkingSetting() *bool {
	var v bool
	switch strings.ToLower(strings.TrimSpace(m.cfg.UI.Thinking)) {
	case "on":
		v = true
	case "off":
		v = false
	default:
		return nil
	}
	return &v
}

func (m Model) thinkingSlash(rest string) Model {
	arg := strings.ToLower(strings.TrimSpace(rest))
	switch arg {
	case "on", "off", "auto":
		if arg == "auto" {
			m.cfg.UI.Thinking = ""
		} else {
			m.cfg.UI.Thinking = arg
		}
		m = m.saveCfg()
		m.toast = "Pensamiento del modelo: " + valueOr(m.cfg.UI.Thinking, "automático (lo decide el servidor)")
	case "show", "ver":
		m.showThink = true
		m.toast = "Mostrando el razonamiento (ctrl+t alterna)"
	case "hide", "ocultar":
		m.showThink = false
		m.toast = "Razonamiento plegado (ctrl+t alterna)"
	case "":
		m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Pensamiento: " +
			valueOr(m.cfg.UI.Thinking, "auto") + " · visible: " + onOff(m.showThink) +
			"\n\n/thinking on|off|auto — activa o apaga el razonamiento del modelo (más lento pero mejor en tareas difíciles)." +
			"\n/thinking show|hide o ctrl+t — muestra u oculta el razonamiento ya generado."})
	default:
		m.toast, m.toastErr = "Uso: /thinking on|off|auto|show|hide", true
	}
	m = m.saveActive()
	m.layout()
	return m
}

// thinkLines dibuja el razonamiento: plegado en una línea o desplegado en gris.
func (m Model) thinkLines(i int, think string, width, botW int) []uiLine {
	think = strings.TrimSpace(think)
	if think == "" {
		return nil
	}
	row := func(text string) uiLine {
		return uiLine{text: text, panel: "chat", panelX0: 0, panelX1: width}
	}
	words := len(strings.Fields(think))
	if !m.showThink {
		return []uiLine{row(spaces(5) + faintStyle.Render("▸ Razonamiento · "+strconv.Itoa(words)+" palabras · ctrl+t para ver")), row("")}
	}
	out := []uiLine{row(spaces(5) + faintStyle.Render("▾ Razonamiento · ctrl+t para plegar"))}
	for _, ln := range wrapText(sanitizeText(think), botW-2) {
		out = append(out, row(spaces(7)+mutedStyle.Render(ln)))
	}
	_ = lipgloss.Width
	return append(out, row(""))
}

const initPrompt = "Analiza este proyecto (estructura, lenguaje, comandos de build y pruebas, convenciones) " +
	"leyendo solo lo necesario, y crea o actualiza AGENTS.md en la raíz con: resumen, cómo compilar y " +
	"probar, estructura de carpetas y reglas importantes. Máximo 60 líneas, sin secretos. Si ya existe, " +
	"mejóralo sin borrar lo útil."

// initStart pide al agente que genere AGENTS.md (como /init de opencode).
func (m Model) initStart() (tea.Model, tea.Cmd) {
	if m.mode == "plan" {
		m.toast, m.toastErr = "/init escribe un archivo: pasa a Build (shift+tab)", true
		return m, nil
	}
	if !m.providerReady() {
		m.toast, m.toastErr = "Conecta un proveedor para usar /init", true
		return m, nil
	}
	m.msgs = append(m.msgs, Message{Role: "user", Text: initPrompt})
	m = m.saveActive()
	return m.startAgent()
}
