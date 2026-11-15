package ui

import (
	"fmt"
	"strings"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/session"
	"nosecode/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const bashMaxWait = 600

// bashWaitState guarda el comando que sigue vivo tras un timeout suave.
type bashWaitState struct {
	call     api.ToolCall
	proc     *tools.BashProcess
	timeoutS int // intervalo pedido por el modelo
	elapsed  int // segundos totales ya esperados
}

type bashWaitMsg struct {
	call     api.ToolCall
	proc     *tools.BashProcess
	output   string
	err      error
	exitCode int
	timeout  bool
	interval int
	duration time.Duration
}

// waitBashCmd espera a que el proceso termine o a que venza otro intervalo.
func waitBashCmd(call api.ToolCall, proc *tools.BashProcess, interval time.Duration) tea.Cmd {
	return func() tea.Msg {
		select {
		case <-proc.Done():
			return bashWaitMsg{
				call: call, proc: proc, output: proc.Output(), err: proc.Err(),
				exitCode: proc.ExitCode(), duration: time.Since(proc.Started()),
			}
		default:
		}
		select {
		case <-proc.Done():
			return bashWaitMsg{
				call: call, proc: proc, output: proc.Output(), err: proc.Err(),
				exitCode: proc.ExitCode(), duration: time.Since(proc.Started()),
			}
		case <-time.After(interval):
			return bashWaitMsg{
				call: call, proc: proc, output: proc.Output(), timeout: true,
				interval: int(interval.Seconds()), duration: time.Since(proc.Started()),
			}
		}
	}
}

// beginBashWait muestra las opciones seguir/cancelar tras el timeout suave.
func (m Model) beginBashWait(msg toolDoneMsg) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		return m, nil
	}
	m = m.stopActivity()
	a.activeCallID = ""
	timeoutS := msg.timeoutS
	if timeoutS <= 0 {
		timeoutS = 60
	}
	m = m.setToolBlock(msg.call.ID, func(b *session.ToolBlock) {
		b.Status = toolSoftTimeout
		b.TimeoutS = timeoutS
		b.Output = msg.result.Text
	})
	if timeoutS >= bashMaxWait {
		return m.cancelBashWait(msg.call, msg.process, msg.result.Text, timeoutS, true)
	}
	m.bashWait = bashWaitState{call: msg.call, proc: msg.process, timeoutS: timeoutS, elapsed: timeoutS}
	m.screen = screenBashWait
	m.toast = fmt.Sprintf("⌛ %d s · el comando sigue vivo", timeoutS)
	m.scroll = 0
	m.layout()
	return m, nil
}

func (m Model) bashWaitKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch k.String() {
	case "s", "enter", "y":
		return m.continueBashWait()
	case "c", "n", "esc":
		w := m.bashWait
		out := ""
		if w.proc != nil {
			out = w.proc.Output()
		}
		return m.cancelBashWait(w.call, w.proc, out, w.elapsed, false)
	}
	return m, nil
}

// continueBashWait espera otro intervalo sin reiniciar el proceso.
func (m Model) continueBashWait() (tea.Model, tea.Cmd) {
	w := m.bashWait
	if w.proc == nil {
		m.screen = screenChat
		return m, nil
	}
	remaining := bashMaxWait - w.elapsed
	if remaining <= 0 {
		return m.cancelBashWait(w.call, w.proc, w.proc.Output(), w.elapsed, true)
	}
	interval := w.timeoutS
	if interval <= 0 {
		interval = 60
	}
	if interval > remaining {
		interval = remaining
	}
	m.screen = screenChat
	m = m.setToolBlock(w.call.ID, func(b *session.ToolBlock) {
		b.Status = toolRunning
		b.TimeoutS = w.elapsed + interval
	})
	var tick tea.Cmd
	m, tick = m.startActivity("tool", "bash (espera)")
	m.toast = fmt.Sprintf("Esperando %d s más… (esc cancela)", interval)
	return m, tea.Batch(waitBashCmd(w.call, w.proc, time.Duration(interval)*time.Second), tick)
}

func (m Model) onBashWait(msg bashWaitMsg) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		return m, nil
	}
	if m.bashWait.proc == nil || m.bashWait.call.ID != msg.call.ID {
		return m, nil // respuesta vieja tras cancelar
	}
	m = m.stopActivity()
	if msg.timeout {
		w := m.bashWait
		elapsed := w.elapsed + msg.interval
		if elapsed >= bashMaxWait {
			return m.cancelBashWait(msg.call, msg.proc, msg.output, elapsed, true)
		}
		m.bashWait.elapsed = elapsed
		m = m.setToolBlock(msg.call.ID, func(b *session.ToolBlock) {
			b.Status = toolSoftTimeout
			b.TimeoutS = elapsed
			b.Output = msg.output
		})
		m.screen = screenBashWait
		m.toast = fmt.Sprintf("⌛ %d s · el comando sigue vivo", elapsed)
		return m, nil
	}

	text := msg.output
	if msg.err != nil {
		if text != "" {
			text += "\n"
		}
		text += "Error: " + msg.err.Error()
	}
	if strings.TrimSpace(text) == "" {
		text = "(sin salida)"
	}
	exitCode := msg.exitCode
	if msg.err != nil && exitCode == 0 {
		exitCode = 1
	}
	m = m.setToolBlock(msg.call.ID, func(b *session.ToolBlock) {
		if msg.err != nil {
			b.Status = toolError
		} else {
			b.Status = toolOK
		}
	})
	m = m.appendToolResult(msg.call, text, nil, msg.err != nil, exitCode, msg.duration)
	m.bashWait = bashWaitState{}
	a.pendingIdx++
	return m.executeNextTool()
}

// cancelBashWait mata el proceso y devuelve la salida parcial al modelo.
func (m Model) cancelBashWait(call api.ToolCall, proc *tools.BashProcess, output string, elapsed int, hard bool) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if proc != nil {
		proc.Kill()
	}
	text := strings.TrimRight(output, "\n")
	suffix := fmt.Sprintf("El usuario canceló el comando tras el timeout de %d s.", elapsed)
	status := toolCancelled
	if hard {
		suffix = fmt.Sprintf("Error: el comando superó el máximo de %d s y se canceló.", bashMaxWait)
		status = toolTimeout
	}
	if text != "" {
		text += "\n"
	}
	text += suffix
	m = m.setToolBlock(call.ID, func(b *session.ToolBlock) {
		b.Status = status
		b.TimeoutS = elapsed
		b.Output = text
	})
	var duration time.Duration
	if proc != nil {
		duration = time.Since(proc.Started())
	}
	m = m.appendToolResult(call, text, nil, true, 130, duration)
	m.bashWait = bashWaitState{}
	m.screen = screenChat
	m = m.stopActivity()
	if hard {
		m.toast, m.toastErr = fmt.Sprintf("⌛ timeout máximo de %d s: comando cancelado", bashMaxWait), true
	} else {
		m.toast, m.toastErr = "Comando cancelado; salida parcial devuelta al modelo", false
	}
	if a != nil {
		a.pendingIdx++
		return m.executeNextTool()
	}
	return m, nil
}

// bashWaitScreen muestra el timeout suave con la salida parcial.
func (m Model) bashWaitScreen() []uiLine {
	boxW := clamp(m.width-12, 56, 92)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4
	w := m.bashWait
	head := []uiLine{{
		text: modalTitleStyle.Render("⌛ Comando aún en ejecución") +
			spaces(maxInt(0, innerW-lipgloss.Width("⌛ Comando aún en ejecución")-lipgloss.Width("esc cancelar"))) +
			faintStyle.Render("esc cancelar"),
	}}
	body := []uiLine{
		{text: textStyle.Render(fmt.Sprintf("Han pasado %d s de un máximo de %d s.", w.elapsed, bashMaxWait))},
		{text: faintStyle.Render("La salida parcial ya está visible en el chat.")},
		{},
	}
	out := ""
	if w.proc != nil {
		out = strings.TrimRight(sanitizeText(w.proc.Output()), "\n")
	}
	if out != "" {
		lines := strings.Split(out, "\n")
		if len(lines) > 8 {
			lines = lines[len(lines)-8:]
		}
		body = append(body, uiLine{text: faintStyle.Render("Salida parcial")})
		for _, ln := range lines {
			for _, wrapped := range wrapText(ln, innerW-2) {
				body = append(body, uiLine{text: faintStyle.Render(" " + wrapped)})
			}
		}
		body = append(body, uiLine{})
	}
	body = append(body,
		uiLine{text: primaryStyle.Render("[s] Seguir esperando " + fmt.Sprintf("%d s", minInt(w.timeoutS, bashMaxWait-w.elapsed)))},
		uiLine{text: dangerStyle.Render("[c] Cancelar y devolver la salida parcial")},
	)
	n := len(body)
	body[n-2].hits = []relHit{{x: 0, w: innerW, action: "bashContinue"}}
	body[n-1].hits = []relHit{{x: 0, w: innerW, action: "bashCancel"}}
	footer := "s seguir · c cancelar · esc cancelar"
	return m.centerModal(m.modalFrame(boxW, head, body, footer), boxW)
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
