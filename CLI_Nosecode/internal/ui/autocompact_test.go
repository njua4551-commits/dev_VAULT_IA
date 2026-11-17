package ui

import (
	"strings"
	"testing"

	"nosecode/internal/api"
)

func tc(id string) []api.ToolCall {
	var c api.ToolCall
	c.ID = id
	c.Function.Name, c.Function.Arguments = "read_file", `{"path":"x"}`
	return []api.ToolCall{c}
}

func TestShrinkMessagesNoSeparaLlamadaYResultado(t *testing.T) {
	big := strings.Repeat("palabra ", 1500) // ~3000 tokens
	msgs := []api.ChatMessage{{Role: "system", Content: "sis"}, {Role: "user", Content: "tarea original"}}
	for i := 0; i < 10; i++ {
		id := string(rune('a' + i))
		msgs = append(msgs,
			api.ChatMessage{Role: "assistant", Content: "", ToolCalls: tc(id)},
			api.ChatMessage{Role: "tool", Content: big, ToolCallID: id})
	}
	before := messagesTokens(msgs)
	out, changed := shrinkMessages(msgs, before/4)
	if !changed || messagesTokens(out) > before/4+200 {
		t.Fatalf("debía reducir: antes=%d después=%d", before, messagesTokens(out))
	}
	if out[0].Role != "system" || out[1].Content != "tarea original" {
		t.Fatal("sistema y tarea original se conservan")
	}
	// integridad: cada mensaje tool está precedido por su assistant con tool_calls
	for i, m := range out {
		if m.Role == "tool" {
			j := i - 1
			for j > 0 && out[j].Role == "tool" {
				j--
			}
			if out[j].Role != "assistant" || len(out[j].ToolCalls) == 0 {
				t.Fatalf("tool huérfano en %d", i)
			}
		}
	}
	if _, changed := shrinkMessages(out, 1<<30); changed {
		t.Fatal("con presupuesto de sobra no toca nada")
	}
}
