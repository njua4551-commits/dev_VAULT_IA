package api

import (
	"context"
	"encoding/json"
	"os"
	"testing"
	"time"

	"nosecode/internal/config"
)

func TestLiveToolCalling(t *testing.T) {
	if os.Getenv("NOSECODE_LIVE_LLM") == "" {
		t.Skip("define NOSECODE_LIVE_LLM=1")
	}
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("config: %v", err)
	}
	_, p := cfg.Active()
	if p.APIKey == "" || p.BaseURL == "" {
		t.Skip("proveedor sin configurar")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
	defer cancel()
	req := Request{
		Bases:  Bases(p.BaseURL, config.VaultHome()),
		APIKey: p.APIKey,
		Model:  p.Model,
		Messages: []ChatMessage{
			{Role: "system", Content: "Eres un agente. Usa la herramienta cuando el usuario lo pida. No respondas sin llamarla."},
			{Role: "user", Content: "Usa list_windows para decirme qué ventanas hay abiertas."},
		},
		Tools: []ToolDef{{
			Type: "function",
			Function: FunctionDef{
				Name:        "list_windows",
				Description: "Lista las ventanas visibles del escritorio.",
				Parameters:  json.RawMessage(`{"type":"object","properties":{}}`),
			},
		}},
	}
	s := Stream(ctx, req)
	var text string
	var calls []string
	for {
		c := s.Next()
		if c.Text != "" {
			text += c.Text
		}
		for _, tc := range c.ToolCalls {
			if tc.Name != "" {
				calls = append(calls, tc.Name)
			}
			if tc.ID != "" {
				calls = append(calls, "id="+tc.ID)
			}
			if tc.Arguments != "" {
				calls = append(calls, "args="+tc.Arguments)
			}
		}
		if c.Done {
			t.Logf("done err=%v", c.Err)
			break
		}
	}
	t.Logf("tool_calls=%v", calls)
	t.Logf("texto=%q", text)
	if len(calls) == 0 {
		t.Errorf("el modelo no emitió tool_calls")
	}
}
