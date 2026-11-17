package tools

import (
	"context"
	"encoding/json"
	"strings"
)

// SubagentRunner lo instala la aplicación (la TUI o «nosecode run»): ejecuta un
// subagente de solo lectura con contexto propio y devuelve su resumen. Vive aquí
// como variable para que tools no dependa del bucle del agente.
var SubagentRunner func(ctx context.Context, prompt string) (string, error)

func runTask(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Prompt string `json:"prompt"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	if strings.TrimSpace(a.Prompt) == "" {
		return errResult("prompt vacío: describe qué debe investigar el subagente")
	}
	if SubagentRunner == nil {
		return errResult("los subagentes no están disponibles en este modo")
	}
	out, err := SubagentRunner(ctx, a.Prompt)
	if err != nil {
		return errResult("el subagente falló: %v", err)
	}
	out = TruncateText(strings.TrimSpace(out), 8<<10)
	if out == "" {
		out = "(el subagente no devolvió nada)"
	}
	return Result{Text: out, Bytes: len(out)}
}
