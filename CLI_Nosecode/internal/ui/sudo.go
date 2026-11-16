package ui

import (
	"runtime"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"nosecode/internal/api"
	"nosecode/internal/debuglog"
	"nosecode/internal/session"
	"nosecode/internal/tools"
)

// sudoCacheFor es cuánto se recuerda la contraseña (solo en memoria).
const sudoCacheFor = 5 * time.Minute

// sudoState es el modal que pide la contraseña de sudo. La contraseña no se
// envía al modelo, no se guarda en disco y no se escribe en el registro.
type sudoState struct {
	call api.ToolCall
	buf  []rune
}

func (m Model) sudoCached() bool {
	return m.sudoPW != "" && time.Now().Before(m.sudoUntil)
}

// needsSudoPassword: hay que pedirla si el comando usa sudo y no hay una vigente.
func (m Model) needsSudoPassword(call api.ToolCall) bool {
	if runtime.GOOS == "windows" {
		return false
	}
	cmd, ok := bashCommand(call)
	return ok && tools.UsesSudo(cmd) && !m.sudoCached()
}

func (m Model) startSudoPrompt(call api.ToolCall) Model {
	m = m.stopActivity()
	m = m.setToolBlock(call.ID, func(b *session.ToolBlock) { b.Status = toolApproval })
	m.sudo = sudoState{call: call}
	m.screen = screenSudo
	m.toast = ""
	debuglog.Logf("sudo prompt tool=%s", call.Function.Name)
	return m
}

// sudoInput maneja las teclas del modal. enter acepta; esc deniega.
func (m Model) sudoInput(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	a := m.run.agent
	if a == nil {
		m.screen = screenChat
		m.sudo = sudoState{}
		return m, nil
	}
	switch k.Type {
	case tea.KeyEsc:
		call := m.sudo.call
		m.sudo = sudoState{}
		m.screen = screenChat
		m = m.setToolBlock(call.ID, func(b *session.ToolBlock) { b.Status = toolDenied })
		m = m.appendToolResult(call, "Denegado: el usuario no dio la contraseña de sudo. No insistas con sudo; propón otra forma o pídele que ejecute el comando él.", nil, true, 1, 0)
		a.pendingIdx++
		return m.executeNextTool()
	case tea.KeyEnter:
		if len(m.sudo.buf) == 0 {
			return m, nil
		}
		m.sudoPW = string(m.sudo.buf)
		m.sudoUntil = time.Now().Add(sudoCacheFor)
		m.sudo = sudoState{}
		m.screen = screenChat
		a.resume = true // no cuenta como paso nuevo ni como repetición
		return m.executeNextTool()
	case tea.KeyBackspace:
		if n := len(m.sudo.buf); n > 0 {
			m.sudo.buf = m.sudo.buf[:n-1]
		}
	case tea.KeyCtrlU:
		m.sudo.buf = nil
	case tea.KeySpace:
		m.sudo.buf = append(m.sudo.buf, ' ')
	case tea.KeyRunes:
		m.sudo.buf = append(m.sudo.buf, k.Runes...)
	}
	return m, nil
}

func (m Model) forgetSudo() Model {
	m.sudoPW, m.sudoUntil = "", time.Time{}
	return m
}

func (m Model) sudoScreen() []uiLine {
	boxW := clamp(m.width-12, 52, 84)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4
	head := []uiLine{{text: modalTitleStyle.Render("Contraseña de sudo")}}
	cmd, _ := bashCommand(m.sudo.call)
	body := []uiLine{{text: warningStyle.Render("El agente quiere ejecutar con permisos de administrador:")}}
	shown := 0
	for _, ln := range wrapText(cmd, innerW-2) {
		if shown == 3 {
			body = append(body, uiLine{text: faintStyle.Render(" …")})
			break
		}
		body = append(body, uiLine{text: textStyle.Render(" " + ln)})
		shown++
	}
	dots := strings.Repeat("•", len(m.sudo.buf))
	body = append(body, uiLine{},
		uiLine{text: faintStyle.Render("Tu contraseña no se envía al modelo ni se guarda en disco.")},
		uiLine{text: faintStyle.Render("Se recuerda 5 min solo en memoria.")},
		uiLine{},
		uiLine{text: textStyle.Render("› " + dots + "▏")})
	return m.centerModal(m.modalFrame(boxW, head, body, "enter aceptar · ctrl+u borrar · esc denegar"), boxW)
}
