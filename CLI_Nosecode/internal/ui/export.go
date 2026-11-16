package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"nosecode/internal/config"
)

// exportMarkdown convierte la conversación a Markdown. No incluye el
// razonamiento y oculta claves conocidas con redactSecrets.
func exportMarkdown(msgs []Message, now time.Time) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# Conversación Nosecode · %s\n", now.Format("2006-01-02 15:04"))
	for _, msg := range msgs {
		switch {
		case msg.Tool != nil:
			t := msg.Tool
			fmt.Fprintf(&b, "\n> ⚙ `%s` %s · %s\n", t.Name, redactSecrets(strings.TrimSpace(t.Args)), t.Status)
			if t.Diff != "" {
				fmt.Fprintf(&b, "\n```diff\n%s\n```\n", redactSecrets(t.Diff))
			}
		case msg.Role == "user":
			fmt.Fprintf(&b, "\n## Tú\n\n%s\n", redactSecrets(msg.Text))
		case msg.Role == "assistant" && strings.TrimSpace(msg.Text) != "":
			if strings.HasPrefix(msg.Text, toolTag) || strings.HasPrefix(msg.Text, vaultTag) {
				continue
			}
			fmt.Fprintf(&b, "\n## Nosecode\n\n%s\n", redactSecrets(msg.Text))
		}
	}
	return b.String()
}

// exportSlash guarda la conversación en ~/.nosecode/exports/.
func (m Model) exportSlash() Model {
	if len(m.msgs) == 0 {
		m.toast, m.toastErr = "No hay nada que exportar", true
		return m
	}
	dir := filepath.Join(config.Home(), "exports")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		m.toast, m.toastErr = "No pude crear "+dir+": "+err.Error(), true
		return m
	}
	now := time.Now()
	path := filepath.Join(dir, "conversacion-"+now.Format("20060102-150405")+".md")
	if err := os.WriteFile(path, []byte(exportMarkdown(m.msgs, now)), 0o600); err != nil {
		m.toast, m.toastErr = "No pude guardar: "+err.Error(), true
		return m
	}
	m.toast, m.toastErr = "Exportado en "+path, false
	return m
}
