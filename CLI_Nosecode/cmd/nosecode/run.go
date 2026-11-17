package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strconv"
	"strings"

	"nosecode/internal/agentloop"
	"nosecode/internal/config"
	"nosecode/internal/lsp"
	"nosecode/internal/tools"
	"nosecode/internal/ui"
)

const runHelp = `nosecode run: ejecuta una tarea sin interfaz (scripts, CI, otros agentes)

Uso:
  nosecode run [opciones] "tarea"      (o con la tarea por stdin: echo "..." | nosecode run)

Opciones:
  --yes            aprueba editar y ejecutar comandos (respeta bash_rules y deny)
  --read-only      solo lectura: investigar sin tocar nada
  --sandbox        aislar bash (modo on: si no hay bubblewrap/sandbox-exec, no ejecuta)
  --json           una línea JSON por evento y un resultado final
  --max-steps N    límite de pasos (por defecto 40)
  --model NOMBRE   modelo a usar en esta ejecución
  --cwd RUTA       carpeta de trabajo

Sin --yes, lo que pediría permiso en la interfaz se deniega.
Códigos de salida: 0 ok · 1 error · 2 uso · 3 límite de pasos alcanzado.
`

// runCommand implementa «nosecode run». Devuelve el código de salida.
func runCommand(cfg *config.Config, args []string, stdin io.Reader, stdout, stderr io.Writer) int {
	var (
		yes, readOnly, asJSON bool
		forceSandbox          bool
		maxSteps              = 40
		model, cwd            string
		words                 []string
	)
	for i := 0; i < len(args); i++ {
		a := args[i]
		next := func() (string, bool) {
			if i+1 >= len(args) {
				return "", false
			}
			i++
			return args[i], true
		}
		switch a {
		case "--yes", "-y":
			yes = true
		case "--sandbox":
			forceSandbox = true
		case "--read-only", "--readonly":
			readOnly = true
		case "--json":
			asJSON = true
		case "--max-steps":
			v, ok := next()
			n, err := strconv.Atoi(v)
			if !ok || err != nil || n < 1 || n > config.MaxStepsLimit {
				fmt.Fprintf(stderr, "--max-steps espera 1..%d\n", config.MaxStepsLimit)
				return 2
			}
			maxSteps = n
		case "--model":
			v, ok := next()
			if !ok {
				fmt.Fprintln(stderr, "--model espera un nombre")
				return 2
			}
			model = v
		case "--cwd":
			v, ok := next()
			if !ok {
				fmt.Fprintln(stderr, "--cwd espera una ruta")
				return 2
			}
			cwd = v
		case "--help", "-h":
			fmt.Fprint(stdout, runHelp)
			return 0
		default:
			if strings.HasPrefix(a, "--") {
				fmt.Fprintf(stderr, "opción desconocida %s\n\n%s", a, runHelp)
				return 2
			}
			words = append(words, a)
		}
	}
	prompt := strings.TrimSpace(strings.Join(words, " "))
	if prompt == "" || prompt == "-" {
		if data, err := io.ReadAll(io.LimitReader(stdin, 1<<20)); err == nil {
			prompt = strings.TrimSpace(string(data))
		}
	}
	if prompt == "" {
		fmt.Fprint(stderr, runHelp)
		return 2
	}
	if cwd != "" {
		if err := os.Chdir(cwd); err != nil {
			fmt.Fprintf(stderr, "no puedo entrar en %s: %v\n", cwd, err)
			return 2
		}
	}
	cwd, _ = os.Getwd()
	name, p := cfg.Active()
	if model != "" {
		p.Model = model
		cfg.Providers[name] = p
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		cfg.SyncVault()
		_, p = cfg.Active()
	}
	if strings.TrimSpace(p.BaseURL) == "" {
		fmt.Fprintln(stderr, "no hay proveedor configurado: abre nosecode y usa /providers o /up")
		return 1
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	enc := json.NewEncoder(stdout)
	onEvent := func(e agentloop.Event) {
		if asJSON {
			_ = enc.Encode(e)
			return
		}
		switch e.Type {
		case "tool":
			fmt.Fprintf(stderr, "→ %s %s\n", e.Name, oneLineArgs(e.Text))
		case "result":
			if e.Failed {
				fmt.Fprintf(stderr, "  ✗ %s\n", oneLineArgs(e.Text))
			}
		case "error":
			fmt.Fprintf(stderr, "error: %s\n", e.Text)
		}
	}
	allowed := func(n string) bool {
		if n == "question" || n == "screenshot" {
			return false
		}
		return !readOnly || agentloop.ReadOnlyTools[n] || n == "bash"
	}
	if forceSandbox {
		cfg.Sandbox.Mode = "on"
	}
	tools.SetSandbox(tools.SandboxConfig{Mode: cfg.Sandbox.Mode, NoNetwork: cfg.Sandbox.NoNetwork, Writable: cfg.Sandbox.Writable})
	tools.SetPersistentShell(cfg.Tools.PersistentShell)
	tools.SubagentRunner = agentloop.Subagent(cfg)
	lsp.Default.Enabled, lsp.Default.Servers = !cfg.LSP.Disabled, cfg.LSP.Servers
	defer lsp.Default.CloseAll()
	res := agentloop.Run(ctx, agentloop.Options{
		Cfg:      cfg,
		System:   ui.HeadlessSystem(),
		Prompt:   prompt,
		Tools:    agentloop.ToolDefs(allowed),
		Policy:   agentloop.NewPolicy(cfg, cwd, readOnly, yes),
		MaxSteps: maxSteps,
		Cwd:      cwd,
		OnEvent:  onEvent,
	})
	code := 0
	switch {
	case res.Err != nil:
		code = 1
	case res.Truncated:
		code = 3
	}
	if asJSON {
		final := map[string]any{"type": "final", "ok": code == 0, "text": res.Text, "steps": res.Steps,
			"tool_calls": res.ToolCalls, "tokens": res.Tokens, "exit_code": code}
		if res.Err != nil {
			final["error"] = res.Err.Error()
		}
		_ = enc.Encode(final)
		return code
	}
	if res.Err != nil {
		fmt.Fprintf(stderr, "error: %v\n", res.Err)
	}
	if res.Truncated {
		fmt.Fprintf(stderr, "se alcanzó el límite de %d pasos\n", maxSteps)
	}
	if res.Text != "" {
		fmt.Fprintln(stdout, res.Text)
	}
	return code
}

func oneLineArgs(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > 160 {
		s = s[:160] + "…"
	}
	return s
}
