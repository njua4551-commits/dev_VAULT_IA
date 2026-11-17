package agentloop

import (
	"context"
	"errors"
	"os"

	"nosecode/internal/config"
)

const subagentSystem = "Eres un subagente explorador de solo lectura. Investiga lo que te piden con read_file, list_dir, grep, repo_map, web_fetch y bash de solo lectura. " +
	"No puedes editar nada ni lanzar otros subagentes. Responde con hallazgos concretos y verificables " +
	"(archivo:línea, nombres de funciones, comandos) en menos de 300 palabras, sin relleno, en español."

// Subagent devuelve la función que implementa la herramienta task: un bucle
// aparte, con contexto limpio, de solo lectura y con pocos pasos.
func Subagent(cfg *config.Config) func(ctx context.Context, prompt string) (string, error) {
	return func(ctx context.Context, prompt string) (string, error) {
		cwd, _ := os.Getwd()
		res := Run(ctx, Options{
			Cfg:      cfg,
			System:   subagentSystem,
			Prompt:   prompt,
			Tools:    ToolDefs(func(n string) bool { return (ReadOnlyTools[n] && n != "todo" && n != "task") || n == "bash" }),
			Policy:   NewPolicy(cfg, cwd, true, false),
			MaxSteps: 12,
			Cwd:      cwd,
		})
		if res.Err != nil {
			return "", res.Err
		}
		if res.Text == "" {
			return "", errors.New("sin respuesta")
		}
		if res.Truncated {
			return res.Text + "\n[el subagente alcanzó su límite de pasos; el resumen puede estar incompleto]", nil
		}
		return res.Text, nil
	}
}
