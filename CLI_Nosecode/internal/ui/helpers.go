package ui

import (
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// lipglossStyle es un alias local para no repetir el import en cada archivo.
type lipglossStyle = lipgloss.Style

var lipglossCenter = lipgloss.NewStyle()

func osGetwd() (string, error) { return os.Getwd() }

func lipglossWidth(s string) int { return lipgloss.Width(s) }

// shortPath abrevia la carpeta personal como ~ (igual que opencode).
func shortPath(p string) string {
	if home, err := os.UserHomeDir(); err == nil && home != "" && strings.HasPrefix(p, home) {
		return "~" + strings.TrimPrefix(p, home)
	}
	return p
}
