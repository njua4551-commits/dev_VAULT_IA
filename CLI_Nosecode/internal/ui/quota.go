package ui

import (
	"context"
	"fmt"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"nosecode/internal/api"
)

// quotaRefresh es cada cuanto se vuelve a consultar la cuota de Kaggle.
const quotaRefresh = 10 * time.Minute

// quotaState guarda la ultima cuota de GPU conocida.
type quotaState struct {
	q       api.VaultQuota
	ok      bool
	err     bool
	loading bool
	at      time.Time
}

type quotaMsg struct {
	q   api.VaultQuota
	err error
}

type quotaTickMsg time.Time

// fetchQuotaCmd consulta la cuota de GPU sin bloquear la TUI.
func fetchQuotaCmd() tea.Cmd {
	return func() tea.Msg {
		script, err := api.FindScript("")
		if err != nil {
			return quotaMsg{err: err}
		}
		py, err := api.FindPython()
		if err != nil {
			return quotaMsg{err: err}
		}
		q, err := api.RunVaultQuota(context.Background(), py, script)
		return quotaMsg{q: q, err: err}
	}
}

func quotaTickCmd() tea.Cmd {
	return tea.Tick(quotaRefresh, func(t time.Time) tea.Msg { return quotaTickMsg(t) })
}

// fetchQuota lanza una consulta si no hay otra en curso.
func (m Model) fetchQuota() tea.Cmd {
	if m.quota.loading {
		return nil
	}
	m.quota.loading = true
	return fetchQuotaCmd()
}

func (m Model) onQuota(msg quotaMsg) Model {
	m.quota.loading = false
	m.quota.at = time.Now()
	if msg.err != nil {
		m.quota.err = true
		// conserva la ultima cuota valida si la habia
		return m
	}
	m.quota = quotaState{q: msg.q, ok: true, at: time.Now()}
	return m
}

// quotaLabel pinta «GPU 26.8h» o un estado mientras se consulta.
func (m Model) quotaLabel() string {
	q := m.quota
	switch {
	case q.loading && !q.ok:
		return mutedStyle.Render("GPU …")
	case q.ok:
		text := "GPU " + formatQuotaHours(q.q.RemainingHours) + "h"
		if q.q.RemainingHours < 2 {
			return warningStyle.Render(text)
		}
		if q.err {
			return mutedStyle.Render(text)
		}
		return successStyle.Render(text)
	case q.err:
		return faintStyle.Render("GPU —")
	default:
		return mutedStyle.Render("GPU …")
	}
}

// formatQuotaHours usa dos decimales cuando queda menos de una hora.
func formatQuotaHours(h float64) string {
	if h < 10 {
		return fmt.Sprintf("%.2f", h)
	}
	return fmt.Sprintf("%.1f", h)
}
