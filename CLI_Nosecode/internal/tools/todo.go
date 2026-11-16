package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// Todo es un paso de la lista de tareas del agente (como todowrite de opencode).
type Todo struct {
	Content string `json:"content"`
	Status  string `json:"status"` // pending | in_progress | completed
}

const maxTodos = 12

type todoArgs struct {
	Todos []Todo `json:"todos"`
}

func normStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "in_progress", "in-progress", "doing", "active", "en_curso", "en curso", "working":
		return "in_progress"
	case "completed", "complete", "done", "hecho", "hecha", "completada", "completado", "finished":
		return "completed"
	}
	return "pending"
}

// ParseTodos valida y normaliza la lista (reemplaza la anterior por completo).
func ParseTodos(raw json.RawMessage) ([]Todo, error) {
	var a todoArgs
	if err := decode(raw, &a); err != nil {
		return nil, err
	}
	out := make([]Todo, 0, len(a.Todos))
	for _, t := range a.Todos {
		c := strings.TrimSpace(t.Content)
		if c == "" {
			continue
		}
		if r := []rune(c); len(r) > 120 {
			c = string(r[:119]) + "…"
		}
		out = append(out, Todo{Content: c, Status: normStatus(t.Status)})
		if len(out) == maxTodos {
			break
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("la lista está vacía; envía al menos una tarea con content y status")
	}
	return out, nil
}

// TodoMarker devuelve el símbolo de cada estado.
func TodoMarker(status string) string {
	switch status {
	case "completed":
		return "✓"
	case "in_progress":
		return "▶"
	}
	return "○"
}

func runTodo(_ context.Context, raw json.RawMessage) Result {
	todos, err := ParseTodos(raw)
	if err != nil {
		return errResult("%v", err)
	}
	var sb strings.Builder
	done, doing := 0, 0
	for _, t := range todos {
		switch t.Status {
		case "completed":
			done++
		case "in_progress":
			doing++
		}
		sb.WriteString(TodoMarker(t.Status) + " " + t.Content + "\n")
	}
	head := fmt.Sprintf("Lista actualizada: %d tareas, %d hechas, %d en curso.\n", len(todos), done, doing)
	return Result{Text: head + strings.TrimRight(sb.String(), "\n")}
}
