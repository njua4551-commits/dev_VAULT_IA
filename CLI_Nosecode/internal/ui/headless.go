package ui

import (
	"os"

	"nosecode/internal/config"
	"nosecode/internal/tools"
)

// HeadlessSystem es el prompt de sistema para «nosecode run» y los subagentes:
// las mismas reglas del agente, sin interlocutor al que preguntar.
func HeadlessSystem() string {
	cwd, _ := os.Getwd()
	return agentSystem +
		"\n\nModo sin interfaz: no hay usuario para responder preguntas ni aprobar permisos. " +
		"No uses question. Si una herramienta se deniega, no insistas: busca otra vía o explica el bloqueo. " +
		"Termina siempre con un resumen breve de lo hecho y de lo que quedó pendiente." +
		loadInstructions(cwd, config.Home(), tools.MemoryPath()) + projectPrompt(cwd) + sandboxPrompt()
}
