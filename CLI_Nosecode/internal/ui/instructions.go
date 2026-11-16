package ui

import (
	"os"
	"path/filepath"
	"strings"

	"nosecode/internal/tools"
)

// instructionsCap limita el texto de instrucciones del proyecto (~4000 tokens).
const instructionsCap = 16 << 10

// loadInstructions junta NOSECODE.md, AGENTS.md del proyecto y AGENTS.md del
// usuario (~/.nosecode). Es texto del usuario, pero se marca como memoria y no
// como reglas del sistema; el tope total evita comerse el contexto.
func loadInstructions(cwd, home, memoryPath string) string {
	type src struct{ title, path string }
	sources := []src{
		{"Memoria del proyecto (NOSECODE.md)", memoryPath},
		{"Instrucciones del proyecto (AGENTS.md)", filepath.Join(cwd, "AGENTS.md")},
		{"Instrucciones del usuario (~/.nosecode/AGENTS.md)", filepath.Join(home, "AGENTS.md")},
	}
	var out strings.Builder
	left := instructionsCap
	seen := map[string]bool{}
	for _, s := range sources {
		if left <= 0 || s.path == "" {
			continue
		}
		if abs, err := filepath.Abs(s.path); err == nil {
			if seen[abs] {
				continue
			}
			seen[abs] = true
		}
		data, err := os.ReadFile(s.path)
		text := strings.TrimSpace(string(data))
		if err != nil || text == "" {
			continue
		}
		n := left
		if n > 8<<10 {
			n = 8 << 10
		}
		text = tools.TruncateText(text, n)
		out.WriteString("\n\n" + s.title + ":\n" + text)
		left -= len(text)
	}
	return out.String()
}

// maxOutputTokens deja que la respuesta use lo que queda de la ventana, para
// que el servidor no se desborde. 0 si no se conoce la ventana.
func (m Model) maxOutputTokens() int {
	win := m.contextWin
	if win <= 0 {
		win = m.effectiveContextWindow()
	}
	if win <= 0 {
		return 0
	}
	left := win - m.contextUsed() - 64
	if left < 256 {
		left = 256
	}
	return left
}

// unknownToolMessage lista las herramientas disponibles para que el modelo se corrija.
func (m Model) unknownToolMessage(name string) string {
	var names []string
	for _, d := range m.apiTools() {
		names = append(names, d.Function.Name)
	}
	return "Error: la herramienta «" + name + "» no existe o está desactivada. No hay subagentes. " +
		"Herramientas disponibles: " + strings.Join(names, ", ") + ". Usa solo una de ellas."
}
