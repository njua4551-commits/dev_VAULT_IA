package tools

import (
	"context"
	"encoding/json"
	"strings"
)

type questionArgs struct {
	Question string   `json:"question"`
	Options  []string `json:"options"`
}

// runQuestion es un marcador: la TUI intercepta esta herramienta para mostrar
// el modal. Si se llega aquí es que no hay interfaz que responder.
func runQuestion(_ context.Context, _ json.RawMessage) Result {
	return errResult("question solo se puede usar desde la interfaz interactiva")
}

// ParseQuestionArgs valida la pregunta y deja entre 2 y 4 opciones.
func ParseQuestionArgs(raw json.RawMessage) (string, []string, error) {
	var a questionArgs
	if err := decode(raw, &a); err != nil {
		return "", nil, err
	}
	question := strings.TrimSpace(a.Question)
	if question == "" {
		return "", nil, errInvalidQuestion("la pregunta está vacía")
	}
	options := make([]string, 0, len(a.Options))
	seen := map[string]bool{}
	for _, opt := range a.Options {
		opt = strings.TrimSpace(opt)
		if opt == "" || seen[opt] {
			continue
		}
		seen[opt] = true
		options = append(options, opt)
		if len(options) == 4 {
			break
		}
	}
	if len(options) < 2 {
		return "", nil, errInvalidQuestion("necesito entre 2 y 4 opciones distintas")
	}
	return question, options, nil
}

type invalidQuestion string

func (e invalidQuestion) Error() string { return string(e) }

func errInvalidQuestion(msg string) error { return invalidQuestion(msg) }
