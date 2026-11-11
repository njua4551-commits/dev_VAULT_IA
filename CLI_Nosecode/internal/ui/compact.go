package ui

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/config"
	"nosecode/internal/debuglog"
	"nosecode/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
)

const (
	compactTag  = "[Resumen de la conversación anterior]"
	compactNote = "[…parte intermedia de la conversación omitida…]"
)

const compactPrompt = "Resume la conversación que recibes en español, en viñetas breves. " +
	"Conserva: objetivo del usuario, decisiones tomadas, archivos y rutas tocadas, comandos ejecutados, " +
	"errores y pendientes. No inventes nada. Responde solo con el resumen, sin preámbulo."

type compactDoneMsg struct {
	summary string
	count   int
	err     error
}

// compactStart resume la conversación con el propio modelo y la sustituye por
// el resumen, que será el contexto de los siguientes mensajes.
func (m Model) compactStart() (Model, tea.Cmd) {
	if m.compacting || m.run.busy() {
		m.toast, m.toastErr = "Espera a que termine o pulsa esc para cancelar", false
		return m, nil
	}
	if len(m.msgs) == 0 {
		m.toast, m.toastErr = "No hay conversación que resumir", false
		return m, nil
	}
	if !m.providerReady() {
		m.toast, m.toastErr = "Conecta un proveedor antes de usar /compact", true
		return m, nil
	}
	_, p := m.cfg.Active()
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "default"
	}
	transcript := compactTranscript(m.msgs, m.contextChars())
	if strings.TrimSpace(transcript) == "" {
		m.toast, m.toastErr = "No hay texto que resumir", false
		return m, nil
	}
	count := len(m.msgs)
	ctx, cancel := context.WithCancel(context.Background())
	m.compacting = true
	m.compactingCancel = cancel
	m.toast, m.toastErr = "Resumiendo la conversación… (esc cancela)", false
	debuglog.Logf("compact start msgs=%d chars=%d", count, len(transcript))

	req := api.Request{
		Bases:  api.Bases(p.BaseURL, config.VaultHome()),
		APIKey: p.APIKey,
		Model:  model,
		Messages: []api.ChatMessage{
			{Role: "system", Content: compactPrompt},
			{Role: "user", Content: transcript},
		},
		IdleTimeout: 120 * time.Second,
	}
	cmd := func() tea.Msg {
		summary, err := api.Complete(ctx, req)
		cancel()
		return compactDoneMsg{summary: summary, count: count, err: err}
	}
	return m, cmd
}

// contextChars limita el texto que se manda a resumir para no repetir el 400.
func (m Model) contextChars() int {
	win := m.contextWin
	if win <= 0 {
		win = m.effectiveContextWindow()
	}
	max := win * 2 // ~3 caracteres por token: dos tercios de la ventana
	if max < 8000 {
		max = 8000
	}
	if max > 400000 {
		max = 400000
	}
	return max
}

// compactTranscript conserva el principio y el final; si sobra, omite el medio.
func compactTranscript(msgs []Message, maxChars int) string {
	entries := make([]string, 0, len(msgs))
	total := 0
	for _, msg := range msgs {
		text := strings.TrimSpace(msg.Text)
		if text == "" || strings.HasPrefix(text, vaultTag) {
			continue
		}
		if len(text) > 4000 {
			text = tools.TruncateText(text, 4000)
		}
		entry := msg.Role + ": " + text
		entries = append(entries, entry)
		total += len(entry) + 2
	}
	if len(entries) == 0 {
		return ""
	}
	if total <= maxChars {
		return strings.Join(entries, "\n\n")
	}

	headBudget := maxChars * 2 / 5
	tailBudget := maxChars - headBudget
	var head []string
	used := 0
	for _, e := range entries {
		if used+len(e)+2 > headBudget {
			break
		}
		head = append(head, e)
		used += len(e) + 2
	}
	var tail []string
	used = 0
	for i := len(entries) - 1; i >= len(head); i-- {
		e := entries[i]
		if used+len(e)+2 > tailBudget {
			break
		}
		tail = append(tail, e)
		used += len(e) + 2
	}
	for i, j := 0, len(tail)-1; i < j; i, j = i+1, j-1 {
		tail[i], tail[j] = tail[j], tail[i]
	}
	parts := append([]string{}, head...)
	parts = append(parts, compactNote)
	parts = append(parts, tail...)
	return strings.Join(parts, "\n\n")
}

func (m Model) onCompactDone(msg compactDoneMsg) (tea.Model, tea.Cmd) {
	if !m.compacting {
		return m, nil
	}
	m.compacting = false
	m.compactingCancel = nil
	if msg.err != nil {
		debuglog.Logf("compact error: %s", oneLine(msg.err.Error(), 200))
		if errors.Is(msg.err, api.ErrCanceled) || errors.Is(msg.err, context.Canceled) {
			m.toast, m.toastErr = "Compactación cancelada", false
			return m, nil
		}
		m.toast, m.toastErr = "No pude resumir: "+oneLine(msg.err.Error(), 120), true
		m.msgs = append(m.msgs, Message{Role: "assistant",
			Text: "⚠ No pude resumir la conversación: " + msg.err.Error()})
		m.layout()
		return m, nil
	}
	summary := strings.TrimSpace(msg.summary)
	if summary == "" {
		m.toast, m.toastErr = "El resumen salió vacío; no cambié la conversación", true
		return m, nil
	}
	summary = tools.TruncateText(summary, 16<<10)
	kept := len([]rune(summary))
	m.msgs = []Message{{Role: "assistant", Text: compactTag + "\n\n" + summary}}
	m.run.agent = nil
	m.usageValid = false
	m.usageTotal = 0
	m.ctxWarned = false
	m.scroll = 0
	m = m.saveActive()
	m.layout()
	m.toast, m.toastErr = fmt.Sprintf("Conversación resumida: %d mensajes → %d caracteres", msg.count, kept), false
	debuglog.Logf("compact done msgs=%d summary_chars=%d", msg.count, kept)
	return m, nil
}
