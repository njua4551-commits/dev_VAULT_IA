package ui

import (
	"fmt"

	"nosecode/internal/api"
	"nosecode/internal/tools"
)

// Autocompactado dentro de una tarea del agente: cuando los mensajes que se van
// a enviar no caben en la ventana (menos el colchón), se reducen sin llamar al
// modelo, de menos a más agresivo:
//  1. recortar salidas y mensajes muy largos del medio,
//  2. borrar salidas de herramienta antiguas,
//  3. omitir los grupos de mensajes más viejos (nunca separa una llamada de su resultado).

const keepTailMsgs = 6

func chatMessageTokens(msg api.ChatMessage) int {
	n := 4
	switch c := msg.Content.(type) {
	case string:
		n += estimateTokens(c)
	case []api.ContentPart:
		for _, p := range c {
			n += estimateTokens(p.Text)
			if p.ImageURL != nil {
				n += 1000
			}
		}
	}
	for _, tc := range msg.ToolCalls {
		n += estimateTokens(tc.Function.Name) + estimateTokens(tc.Function.Arguments)
	}
	return n
}

func messagesTokens(msgs []api.ChatMessage) int {
	total := 0
	for _, msg := range msgs {
		total += chatMessageTokens(msg)
	}
	return total
}

// shrinkMessages devuelve los mensajes ajustados al presupuesto y si cambió algo.
func shrinkMessages(msgs []api.ChatMessage, budget int) ([]api.ChatMessage, bool) {
	if budget <= 0 || messagesTokens(msgs) <= budget {
		return msgs, false
	}
	// 1. recortar textos largos del medio (todo menos el sistema y la cola)
	for i := 1; i < len(msgs)-keepTailMsgs; i++ {
		if s, ok := msgs[i].Content.(string); ok && len(s) > 3000 {
			msgs[i].Content = tools.TruncateText(s, 1500)
		}
	}
	if messagesTokens(msgs) <= budget {
		return msgs, true
	}
	// 2. borrar salidas de herramienta antiguas
	microcompact(msgs, 1)
	if messagesTokens(msgs) <= budget {
		return msgs, true
	}
	// 3. omitir los grupos más viejos, conservando sistema, primera petición y cola
	start := 1
	if len(msgs) > 1 && msgs[1].Role == "user" {
		start = 2 // la tarea original se conserva
	}
	dropped := 0
	out := msgs
	for len(out)-start > keepTailMsgs && messagesTokens(out) > budget {
		end := start + 1
		if out[start].Role == "assistant" && len(out[start].ToolCalls) > 0 {
			for end < len(out) && out[end].Role == "tool" {
				end++
			}
		}
		if len(out)-end < keepTailMsgs {
			break
		}
		out = append(append([]api.ChatMessage{}, out[:start]...), out[end:]...)
		dropped += end - start
	}
	if dropped > 0 {
		note := api.ChatMessage{Role: "user", Content: fmt.Sprintf("[%d mensajes antiguos de esta tarea se omitieron para caber en el contexto; si necesitas algo de ellos, vuelve a consultarlo]", dropped)}
		out = append(append(append([]api.ChatMessage{}, out[:start]...), note), out[start:]...)
	}
	return out, true
}
