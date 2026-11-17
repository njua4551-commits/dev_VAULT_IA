// Package agentloop es el bucle del agente sin interfaz: lo usan «nosecode run»
// (modo headless/CI) y los subagentes. La TUI tiene su propio bucle con
// aprobaciones interactivas; aquí las decisiones las toma una Policy.
package agentloop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"nosecode/internal/api"
	"nosecode/internal/config"
	"nosecode/internal/lsp"
	"nosecode/internal/tools"
)

// Policy decide si una llamada puede ejecutarse. Si no, devuelve el motivo
// (se le entrega al modelo como resultado de la herramienta).
type Policy func(call api.ToolCall) (ok bool, reason string)

// Event se emite durante la ejecución (para --json o para depurar).
type Event struct {
	Type   string `json:"type"` // text | tool | result | error | done
	Step   int    `json:"step,omitempty"`
	Name   string `json:"name,omitempty"`
	Text   string `json:"text,omitempty"`
	Failed bool   `json:"failed,omitempty"`
}

// Options configura una ejecución.
type Options struct {
	Cfg      *config.Config
	System   string
	Prompt   string
	Tools    []api.ToolDef
	Policy   Policy
	MaxSteps int
	Cwd      string
	OnEvent  func(Event)
	// Stream permite inyectar el envío al modelo (tests); nil usa api.Stream.
	Stream func(ctx context.Context, req api.Request) *api.Streamer
}

// Result resume la ejecución.
type Result struct {
	Text      string // última respuesta del modelo
	Steps     int
	ToolCalls int
	Tokens    int
	Err       error
	Truncated bool // se alcanzó MaxSteps
}

// ToolDefs arma las definiciones de las herramientas nativas permitidas.
func ToolDefs(allowed func(name string) bool) []api.ToolDef {
	var out []api.ToolDef
	for _, t := range tools.Builtins() {
		if !allowed(t.Name) {
			continue
		}
		out = append(out, api.ToolDef{Type: "function", Function: api.FunctionDef{
			Name: t.Name, Description: t.Description, Parameters: t.Parameters,
		}})
	}
	return out
}

// Run ejecuta el bucle hasta que el modelo responde sin herramientas.
func Run(ctx context.Context, o Options) Result {
	emit := func(e Event) {
		if o.OnEvent != nil {
			o.OnEvent(e)
		}
	}
	if o.MaxSteps <= 0 {
		o.MaxSteps = 40
	}
	if o.Cwd == "" {
		o.Cwd, _ = os.Getwd()
	}
	stream := o.Stream
	if stream == nil {
		stream = api.Stream
	}
	_, p := o.Cfg.Active()
	model := strings.TrimSpace(p.Model)
	if model == "" {
		model = "default"
	}
	msgs := []api.ChatMessage{}
	if strings.TrimSpace(o.System) != "" {
		msgs = append(msgs, api.ChatMessage{Role: "system", Content: o.System})
	}
	msgs = append(msgs, api.ChatMessage{Role: "user", Content: o.Prompt})

	var res Result
	lastKey, streak := "", 0
	for step := 1; step <= o.MaxSteps; step++ {
		if ctx.Err() != nil {
			res.Err = ctx.Err()
			return res
		}
		res.Steps = step
		s := stream(ctx, api.Request{
			Bases:       api.Bases(p.BaseURL, config.VaultHome()),
			APIKey:      p.APIKey,
			Model:       model,
			Messages:    msgs,
			Tools:       o.Tools,
			IdleTimeout: 180 * time.Second,
		})
		var text strings.Builder
		var calls []api.ToolCall
		for {
			c := s.Next()
			if c.Text != "" {
				text.WriteString(c.Text)
			}
			mergeDeltas(&calls, c.ToolCalls)
			if c.Usage != nil {
				res.Tokens = c.Usage.TotalTokens
			}
			if c.Err != nil {
				res.Err = c.Err
				emit(Event{Type: "error", Step: step, Text: c.Err.Error()})
				return res
			}
			if c.Done {
				break
			}
		}
		res.Text = strings.TrimSpace(text.String())
		if res.Text != "" {
			emit(Event{Type: "text", Step: step, Text: res.Text})
		}
		if len(calls) == 0 {
			emit(Event{Type: "done", Step: step})
			return res
		}
		for i := range calls {
			if calls[i].ID == "" {
				calls[i].ID = fmt.Sprintf("call_%d_%d", step, i)
			}
			calls[i].Type = "function"
			calls[i].Function.Name, calls[i].Function.Arguments =
				tools.NormalizeCall(calls[i].Function.Name, calls[i].Function.Arguments)
		}
		msgs = append(msgs, api.ChatMessage{Role: "assistant", Content: res.Text, ToolCalls: calls})
		for _, call := range calls {
			res.ToolCalls++
			emit(Event{Type: "tool", Step: step, Name: call.Function.Name, Text: call.Function.Arguments})
			key := call.Function.Name + "\x00" + call.Function.Arguments
			if key == lastKey {
				streak++
			} else {
				lastKey, streak = key, 1
			}
			out, failed := "", false
			switch {
			case streak >= 3:
				out, failed = "Error: repetiste la misma llamada 3 veces sin avanzar. Cambia de enfoque o responde con lo que sabes.", true
			default:
				out, failed = execute(ctx, o, call)
			}
			emit(Event{Type: "result", Step: step, Name: call.Function.Name, Text: tools.TruncateText(out, 2000), Failed: failed})
			msgs = append(msgs, api.ChatMessage{Role: "tool", Content: out, ToolCallID: call.ID})
		}
	}
	res.Truncated = true
	emit(Event{Type: "done", Step: o.MaxSteps, Text: "límite de pasos"})
	return res
}

func execute(ctx context.Context, o Options, call api.ToolCall) (string, bool) {
	if o.Policy != nil {
		if ok, why := o.Policy(call); !ok {
			return "Denegado: " + why, true
		}
	}
	t, ok := tools.Lookup(call.Function.Name)
	if !ok {
		return "Error: herramienta desconocida " + call.Function.Name + ". Disponibles: " + strings.Join(tools.Names(), ", "), true
	}
	r := t.Run(ctx, json.RawMessage(call.Function.Arguments))
	text := r.Text
	if !r.Error {
		files := r.Edited
		if call.Function.Name == "write_file" || call.Function.Name == "edit_file" {
			var a struct {
				Path string `json:"path"`
			}
			if json.Unmarshal([]byte(call.Function.Arguments), &a) == nil && a.Path != "" {
				files = []string{a.Path}
			}
		}
		text += lsp.Default.PostEditNotes(files)
	}
	if r.Soft && r.Process != nil {
		// Sin persona que decida: se mata y se informa. (Para procesos largos, background:true.)
		r.Process.Kill()
		text += fmt.Sprintf("\n[el comando superó %d s y se canceló; usa timeout_s mayor o background:true]", r.TimeoutS)
		r.Error = true
	}
	if len(r.Images) > 0 {
		text += "\n[imagen omitida en modo sin interfaz]"
	}
	return text, r.Error
}

func mergeDeltas(calls *[]api.ToolCall, ds []api.ToolCallDelta) {
	for _, d := range ds {
		for len(*calls) <= d.Index {
			*calls = append(*calls, api.ToolCall{Type: "function"})
		}
		c := &(*calls)[d.Index]
		if d.ID != "" {
			c.ID = d.ID
		}
		if d.Name != "" && !strings.HasSuffix(c.Function.Name, d.Name) {
			c.Function.Name += d.Name
		}
		c.Function.Arguments += d.Arguments
	}
}
