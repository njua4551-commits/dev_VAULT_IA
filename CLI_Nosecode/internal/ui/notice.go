package ui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Avisos: mensajes breves que salen en la línea de actividad (sobre el
// campo de texto) y NO se guardan en el chat. Sirven para encender/apagar la
// IA, que antes llenaba la conversación de líneas.

type noticeExpireMsg struct{}

const (
	noticeLife  = 8 * time.Second
	maxVaultLog = 300 // tope de la bitácora en memoria (/vault log)
)

// notify muestra un aviso. life = 0 lo deja fijo hasta el próximo aviso (mientras corre algo).
func (m *Model) notify(text string, isErr bool, life time.Duration) {
	m.vaultNotice, m.vaultNoticeErr = text, isErr
	if life > 0 {
		m.vaultNoticeUntil = time.Now().Add(life)
	} else {
		m.vaultNoticeUntil = time.Time{}
	}
}

// activeNotice devuelve el aviso vigente: fijo mientras vault.py corre, o hasta que caduque.
func (m Model) activeNotice() string {
	if m.vaultNotice == "" {
		return ""
	}
	if m.run.vault != nil && m.vaultQuiet {
		return m.vaultNotice
	}
	if !m.vaultNoticeUntil.IsZero() && time.Now().Before(m.vaultNoticeUntil) {
		return m.vaultNotice
	}
	return ""
}

func vaultLabel(cmd string) string {
	switch cmd {
	case "up":
		return "encender la IA"
	case "down":
		return "apagar la IA"
	}
	return "vault.py " + cmd
}

// onVaultQuiet procesa la salida de vault.py up/down sin escribir en el chat.
func (m Model) onVaultQuiet(msg vaultMsg) (tea.Model, tea.Cmd) {
	l := msg.l
	if !l.Done {
		if t := strings.TrimSpace(sanitizeText(l.Text)); t != "" {
			m.vaultLog = append(m.vaultLog, t)
			if len(m.vaultLog) > maxVaultLog {
				m.vaultLog = m.vaultLog[len(m.vaultLog)-maxVaultLog:]
			}
			prefix := map[string]string{"up": "Encendiendo la IA · ", "down": "Apagando la IA · "}[m.vaultCmd]
			m.notify(prefix+t, false, 0)
		}
		return m, waitVault(msg.v)
	}

	m.run.vault = nil
	m.vaultQuiet = false
	var cmds []tea.Cmd
	if l.Err != nil {
		m.notify("No pude "+vaultLabel(m.vaultCmd)+": "+l.Err.Error()+" · /vault log muestra el detalle", true, 15*time.Second)
		m.sound(soundError)
	} else {
		switch m.vaultCmd {
		case "up":
			m = m.importVault() // la URL pudo cambiar: recarga el proveedor
			m.notify("✓ IA encendida y lista", false, noticeLife)
		case "down":
			m.notify("✓ IA apagada", false, noticeLife)
		}
		m.sound(soundDone)
		cmds = append(cmds, m.fetchQuota())
	}
	// importVault deja un toast propio; el aviso ya informa, así que no se duplica.
	m.toast = ""
	cmds = append(cmds, tea.Tick(noticeLife+time.Second, func(time.Time) tea.Msg { return noticeExpireMsg{} }))
	m.layout()
	return m, tea.Batch(cmds...)
}

// vaultLogSlash muestra en el chat (a pedido) la bitácora del último up/down.
func (m Model) vaultLogSlash() Model {
	if len(m.vaultLog) == 0 {
		m.toast, m.toastErr = "No hay bitácora del Vault todavía", true
		return m
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: vaultTag + "bitácora de " + m.vaultCmd + "\n" + strings.Join(m.vaultLog, "\n") + "\n"})
	m.scroll = 0
	m = m.saveActive()
	m.layout()
	return m
}
