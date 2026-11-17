package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"nosecode/internal/api"
	"nosecode/internal/session"
	"nosecode/internal/tools"
)

// planTools son las únicas herramientas del modo Plan (agente de solo lectura).
var planTools = map[string]bool{
	"read_file": true, "list_dir": true, "grep": true, "web_fetch": true,
	"view_image": true, "screenshot": true, "skill": true, "repo_map": true, "task": true, "diagnostics": true, "lsp": true, "question": true, "bash": true, "todo": true,
}

func planToolAllowed(name string) bool { return planTools[name] }

const planAgentSystem = "\n\nMODO PLAN (solo lectura): investiga el proyecto con read_file, list_dir, grep, " +
	"web_fetch y bash de solo lectura (pwd, ls, git status, git diff, git log, cat). NO puedes escribir ni editar " +
	"archivos ni ejecutar otros comandos. Termina con un plan numerado de 3 a 5 pasos: archivos implicados, " +
	"cambios y riesgos. Si falta una decisión del usuario, usa question. El usuario aprobará el plan con ctrl+e " +
	"para pasar a Build."

// bashDecision combina permissions.bash, las reglas por patrón y el modo Plan.
// Devuelve "allow", "ask" o "deny" y el motivo si se deniega.
func (m Model) bashDecision(call api.ToolCall) (string, string) {
	cmd, ok := bashCommand(call)
	if !ok {
		return "ask", ""
	}
	if m.cfg.Permissions.Bash == "deny" {
		return "deny", "bash está desactivado (permissions.bash = deny)"
	}
	cwd, _ := os.Getwd()
	d := tools.DecideBash(cmd, m.cfg.Permissions.BashRules, cwd)
	if d == "deny" {
		return "deny", "bloqueado por una regla de permissions.bash_rules"
	}
	if m.mode == "plan" {
		if tools.BashReadOnly(cmd, cwd) {
			return "allow", ""
		}
		return "deny", "en modo Plan solo se permiten comandos de solo lectura (pwd, ls, git status, git diff, git log, cat)"
	}
	return d, ""
}

// planDenied indica si una llamada no es válida en modo Plan.
func (m Model) planDenied(name string) bool {
	return m.mode == "plan" && !planToolAllowed(shortTool(name))
}

// ------------------------------------------------------------ question ---

// questionState es el modal de la herramienta nativa question.
type questionState struct {
	call    api.ToolCall
	text    string
	options []string
	sel     int
	typing  bool
	free    string
}

// startQuestion pausa el agente y pregunta al usuario.
func (m Model) startQuestion(call api.ToolCall) (Model, bool) {
	text, opts, err := tools.ParseQuestionArgs(json.RawMessage(call.Function.Arguments))
	if err != nil {
		m = m.setToolBlock(call.ID, func(b *session.ToolBlock) { b.Status = toolError })
		m = m.appendToolResult(call, "Error: "+err.Error(), nil, true, 1, 0)
		return m, false
	}
	m = m.stopActivity()
	m = m.setToolBlock(call.ID, func(b *session.ToolBlock) { b.Status = toolApproval })
	m.question = questionState{call: call, text: text, options: opts}
	m.screen = screenQuestion
	m.toast = ""
	return m, true
}

// answerQuestion devuelve la respuesta al modelo y sigue con la siguiente herramienta.
func (m Model) answerQuestion(answer string) (Model, bool) {
	a := m.run.agent
	if a == nil {
		m.screen = screenChat
		return m, false
	}
	call := m.question.call
	m.question = questionState{}
	m.screen = screenChat
	m = m.setToolBlock(call.ID, func(b *session.ToolBlock) { b.Status = toolOK })
	m = m.appendToolResult(call, "El usuario respondió: "+answer, nil, false, 0, 0)
	a.pendingIdx++
	return m, true
}

func (m Model) questionKey(k string, runes []rune, special string) (Model, string) {
	q := m.question
	n := len(q.options)
	if q.typing {
		switch special {
		case "esc":
			q.typing = false
		case "backspace":
			if r := []rune(q.free); len(r) > 0 {
				q.free = string(r[:len(r)-1])
			}
		case "enter":
			if t := strings.TrimSpace(q.free); t != "" {
				m.question = q
				return m, t
			}
		default:
			if len(runes) > 0 {
				q.free += string(runes)
			} else if special == "space" {
				q.free += " "
			}
		}
		m.question = q
		return m, ""
	}
	switch k {
	case "up", "k":
		q.sel = (q.sel + n) % (n + 1) // n = «otra respuesta»
		if q.sel == 0 {
			q.sel = n
		} else {
			q.sel--
		}
	case "down", "j", "tab":
		q.sel = (q.sel + 1) % (n + 1)
	case "o":
		q.typing = true
	case "1", "2", "3", "4":
		if i := int(k[0] - '1'); i < n {
			m.question = q
			return m, q.options[i]
		}
	case "enter":
		if q.sel < n {
			m.question = q
			return m, q.options[q.sel]
		}
		q.typing = true
	}
	m.question = q
	return m, ""
}

func (m Model) questionScreen() []uiLine {
	boxW := clamp(m.width-12, 52, 88)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4
	q := m.question
	head := []uiLine{{
		text: modalTitleStyle.Render("Pregunta del agente") +
			spaces(maxInt(0, innerW-lipgloss.Width("Pregunta del agente")-lipgloss.Width("esc cancelar turno"))) +
			faintStyle.Render("esc cancelar turno"),
	}}
	var body []uiLine
	for _, ln := range wrapText(q.text, innerW) {
		body = append(body, uiLine{text: textStyle.Render(ln)})
	}
	body = append(body, uiLine{})
	for i, opt := range q.options {
		label := fmt.Sprintf("[%d] %s", i+1, opt)
		style := textStyle
		if i == q.sel && !q.typing {
			style = primaryStyle
		}
		for j, ln := range wrapText(label, innerW) {
			if j > 0 {
				ln = "    " + ln
			}
			body = append(body, uiLine{text: style.Render(ln)})
		}
	}
	other := "[o] Otra respuesta…"
	style := faintStyle
	if q.sel == len(q.options) && !q.typing {
		style = primaryStyle
	}
	body = append(body, uiLine{text: style.Render(other)})
	if q.typing {
		body = append(body, uiLine{}, uiLine{text: faintStyle.Render("Tu respuesta (enter envía · esc vuelve)")},
			uiLine{text: textStyle.Render("› " + q.free + "▏")})
	}
	footer := "1-4 elegir · ↑↓ mover · enter aceptar · o escribir · esc cancelar turno"
	return m.centerModal(m.modalFrame(boxW, head, body, footer), boxW)
}

// questionInput traduce la tecla pulsada y, si hay respuesta, continúa el agente.
func (m Model) questionInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	if m.run.agent == nil {
		m.screen = screenChat
		return m, nil
	}
	key := k.String()
	if key == "esc" && !m.question.typing {
		return m.cancelBusy()
	}
	special := ""
	switch k.Type {
	case tea.KeyEsc:
		special = "esc"
	case tea.KeyBackspace:
		special = "backspace"
	case tea.KeyEnter:
		special = "enter"
	case tea.KeySpace:
		special = "space"
	}
	nm, answer := m.questionKey(key, k.Runes, special)
	if answer == "" {
		return nm, nil
	}
	nm, ok := nm.answerQuestion(answer)
	if !ok {
		return nm, nil
	}
	return nm.executeNextTool()
}

// loopRepeated detecta al modelo atascado: la misma llamada (mismo nombre y
// argumentos) 3 veces seguidas. Las preguntas al usuario nunca cuentan.
func loopRepeated(a *agentRun, call api.ToolCall) bool {
	if shortTool(call.Function.Name) == "question" {
		a.lastCallKey, a.callStreak = "", 0
		return false
	}
	key := call.Function.Name + "\x00" + strings.TrimSpace(call.Function.Arguments)
	if key == a.lastCallKey {
		a.callStreak++
	} else {
		a.lastCallKey, a.callStreak = key, 1
	}
	return a.callStreak >= 3
}

// undoSlash maneja /undo, /undo list, /undo force y /redo.
func (m Model) undoSlash(cmd, rest string) Model {
	arg := strings.ToLower(strings.TrimSpace(rest))
	var msg string
	var err error
	switch {
	case cmd == "/redo":
		msg, err = tools.Redo()
	case arg == "list" || arg == "lista":
		list := tools.UndoList()
		if len(list) == 0 {
			msg = "No hay cambios del agente que deshacer."
		} else {
			msg = "Cambios que /undo puede revertir (el más reciente primero):\n• " + strings.Join(list, "\n• ")
		}
	default:
		msg, err = tools.Undo(arg == "force")
	}
	if err != nil {
		m.toast, m.toastErr = err.Error(), true
		return m
	}
	m.toast, m.toastErr = msg, false
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: msg + "\n\n(/undo solo cubre write_file y edit_file, no lo que hagan los comandos de bash.)"})
	m = m.saveActive()
	m.layout()
	return m
}
