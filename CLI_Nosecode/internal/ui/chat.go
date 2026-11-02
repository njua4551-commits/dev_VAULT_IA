package ui

import (
	"context"
	"strings"

	"nosecode/internal/api"
	"nosecode/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

// runState agrupa lo que esta corriendo en segundo plano: una respuesta del
// modelo (streaming) o un comando del Dev Vault (up/down...).
type runState struct {
	stream *api.Streamer
	vault  *api.VaultRun
	agent  *agentRun
}

func (r runState) busy() bool {
	return r.stream != nil || r.vault != nil || r.agent.busy()
}

type chunkMsg struct {
	s *api.Streamer
	c api.Chunk
}

type vaultMsg struct {
	v *api.VaultRun
	l api.VaultLine
}

func waitChunk(s *api.Streamer) tea.Cmd {
	return func() tea.Msg { return chunkMsg{s: s, c: s.Next()} }
}

func waitVault(v *api.VaultRun) tea.Cmd {
	return func() tea.Msg { return vaultMsg{v: v, l: v.Next()} }
}

const vaultTag = "Vault · "

// vaultCommands son los comandos de barra que ejecutan client/vault.py.
var vaultCommands = map[string]string{"/up": "up", "/down": "down", "/urls": "urls", "/status": "status"}

func vaultCommand(text string) (string, bool) {
	f := strings.Fields(text)
	if len(f) == 0 {
		return "", false
	}
	c, ok := vaultCommands[strings.ToLower(f[0])]
	return c, ok
}

const planSystem = "Estás en modo Plan: no ejecutes ni escribas código final todavía. " +
	"Responde con un plan numerado y breve: pasos, archivos implicados y riesgos."

// apiMessages convierte la conversacion en el formato de la API.
func apiMessages(msgs []Message, plan bool) []api.ChatMessage {
	out := make([]api.ChatMessage, 0, len(msgs)+1)
	if plan {
		out = append(out, api.ChatMessage{Role: "system", Content: planSystem})
	}
	for _, msg := range msgs {
		text := strings.TrimSpace(msg.Text)
		if text == "" || (msg.Role != "user" && msg.Role != "assistant") {
			continue
		}
		if msg.Role == "assistant" && strings.HasPrefix(text, vaultTag) {
			continue // salida de up/down: no es parte de la conversacion
		}
		if msg.Role == "assistant" && strings.HasPrefix(text, toolTag) {
			continue // actividad de herramientas: vive en el historial del agente
		}
		out = append(out, api.ChatMessage{Role: msg.Role, Content: text})
	}
	return out
}

// providerReady dice si el proveedor activo tiene URL y API key.
func (m Model) providerReady() bool {
	_, p := m.cfg.Active()
	return strings.TrimSpace(p.BaseURL) != "" && strings.TrimSpace(p.APIKey) != ""
}

// startChat envia la conversacion (el mensaje del usuario ya esta en m.msgs).
func (m Model) startChat() (Model, tea.Cmd) {
	_, p := m.cfg.Active()
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "default"
	}
	s := api.Stream(context.Background(), api.Request{
		Bases:    api.Bases(p.BaseURL, config.VaultHome()),
		APIKey:   p.APIKey,
		Model:    model,
		Messages: apiMessages(m.msgs, m.mode == "plan"),
	})
	m.run.stream = s
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: ""})
	m.toast, m.toastErr = "Escribiendo… (esc cancela)", false
	return m, waitChunk(s)
}

func (m Model) lastIdx() int { return len(m.msgs) - 1 }

func (m Model) onChunk(msg chunkMsg) (tea.Model, tea.Cmd) {
	if msg.s != m.run.stream || m.run.stream == nil || m.lastIdx() < 0 {
		return m, nil // respuesta vieja o cancelada
	}
	c := msg.c
	i := m.lastIdx()
	if c.Text != "" {
		m.msgs[i].Text += c.Text
		m.scroll = 0
		m.layout()
	}
	if !c.Done {
		return m, waitChunk(msg.s)
	}

	m.run.stream = nil
	switch {
	case c.Err == nil:
		m.toast, m.toastErr = "", false
		if strings.TrimSpace(m.msgs[i].Text) == "" {
			m.msgs[i].Text = "(respuesta vacía)"
		}
		m.sound(soundDone)
	case c.Err == api.ErrCanceled:
		m.toast, m.toastErr = "Respuesta cancelada", false
		if strings.TrimSpace(m.msgs[i].Text) == "" {
			m.msgs[i].Text = "(cancelado)"
		}
	default:
		m.toast, m.toastErr = "Error: "+c.Err.Error(), true
		if strings.TrimSpace(m.msgs[i].Text) == "" {
			m.msgs[i].Text = "⚠ No pude obtener respuesta: " + c.Err.Error() +
				"\n\n¿Está encendida la IA? Usa /up. Si ya lo está, revisa /urls."
		} else {
			m.msgs[i].Text += "\n\n⚠ Respuesta interrumpida: " + c.Err.Error()
		}
		m.sound(soundError)
	}
	m = m.saveActive()
	m.layout()
	return m, nil
}

// startVault ejecuta `vault.py <cmd>` y muestra su salida linea a linea.
func (m Model) startVault(cmd string) (Model, tea.Cmd) {
	script, err := api.FindScript("")
	if err == nil {
		var py string
		if py, err = api.FindPython(); err == nil {
			v := api.RunVault(context.Background(), py, script, cmd)
			m.run.vault = v
			m.msgs = append(m.msgs, Message{Role: "assistant", Text: vaultTag + cmd + "\n"})
			m.toast, m.toastErr = "vault.py "+cmd+" en curso… (esc cancela)", false
			m.layout()
			return m, waitVault(v)
		}
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: "No pude ejecutar vault.py: " + err.Error()})
	m.toast, m.toastErr = err.Error(), true
	m.sound(soundError)
	m.layout()
	return m, nil
}

const maxVaultLines = 60

func (m Model) onVault(msg vaultMsg) (tea.Model, tea.Cmd) {
	if msg.v != m.run.vault || m.run.vault == nil || m.lastIdx() < 0 {
		return m, nil
	}
	i := m.lastIdx()
	l := msg.l
	if !l.Done {
		lines := strings.Split(strings.TrimRight(m.msgs[i].Text, "\n"), "\n")
		lines = append(lines, l.Text)
		if len(lines) > maxVaultLines {
			lines = append(lines[:1], lines[len(lines)-maxVaultLines+1:]...)
		}
		m.msgs[i].Text = strings.Join(lines, "\n") + "\n"
		m.scroll = 0
		m.layout()
		return m, waitVault(msg.v)
	}

	m.run.vault = nil
	if l.Err != nil {
		m.toast, m.toastErr = "vault.py: "+l.Err.Error(), true
		m.msgs[i].Text += "\n⚠ " + l.Err.Error()
		m.sound(soundError)
	} else {
		m.toast, m.toastErr = "vault.py terminó", false
		if strings.HasPrefix(m.msgs[i].Text, vaultTag+"up") {
			m = m.importVault() // la URL pudo cambiar: recarga el proveedor
		}
		m.sound(soundDone)
	}
	m = m.saveActive()
	m.layout()
	return m, nil
}

// cancelBusy corta la respuesta o el comando en curso.
func (m Model) cancelBusy() (tea.Model, tea.Cmd) {
	if m.run.agent != nil {
		if m.run.agent.stream != nil {
			m.run.agent.stream.Cancel()
		}
		m = m.clearConnecting()
		m.run.agent = nil
		m.screen = screenChat
		m.toast, m.toastErr = "Agente cancelado", false
	}
	if m.run.stream != nil {
		m.run.stream.Cancel()
		m.run.stream = nil
		if i := m.lastIdx(); i >= 0 && strings.TrimSpace(m.msgs[i].Text) == "" {
			m.msgs[i].Text = "(cancelado)"
		}
		m.toast, m.toastErr = "Respuesta cancelada", false
	}
	if m.run.vault != nil {
		m.run.vault.Cancel()
		m.run.vault = nil
		m.toast, m.toastErr = "Comando cancelado (la IA en Kaggle puede seguir encendida: usa /down)", false
	}
	m = m.saveActive()
	m.layout()
	return m, nil
}

// cancelAll se usa al salir.
func (m Model) cancelAll() {
	if m.run.agent != nil && m.run.agent.stream != nil {
		m.run.agent.stream.Cancel()
	}
	if m.run.stream != nil {
		m.run.stream.Cancel()
	}
	if m.run.vault != nil {
		m.run.vault.Cancel()
	}
	m.closeMCP()
}
