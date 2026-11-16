package ui

import (
	"strconv"
	"strings"

	"nosecode/internal/tools"
)

// currentTodos devuelve la última lista guardada con la herramienta todo.
// Se lee del resultado del último bloque `todo`, así sigue a cada sesión sin
// guardar nada extra.
func (m Model) currentTodos() []tools.Todo {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		b := m.msgs[i].Tool
		if b == nil || b.Name != "todo" || b.Status != toolOK {
			continue
		}
		return parseTodoOutput(b.Output)
	}
	return nil
}

func parseTodoOutput(out string) []tools.Todo {
	var todos []tools.Todo
	for _, ln := range strings.Split(out, "\n") {
		for marker, status := range map[string]string{"✓ ": "completed", "▶ ": "in_progress", "○ ": "pending"} {
			if strings.HasPrefix(ln, marker) {
				todos = append(todos, tools.Todo{Content: strings.TrimPrefix(ln, marker), Status: status})
			}
		}
	}
	return todos
}

// todoRailRows son las filas de la barra lateral (máx. 8).
func (m Model) todoRailRows(cw int) []string {
	todos := m.currentTodos()
	if len(todos) == 0 {
		return nil
	}
	done := 0
	for _, t := range todos {
		if t.Status == "completed" {
			done++
		}
	}
	rows := []string{titleStyle.Render("Tareas") + " " + mutedStyle.Render(strconv.Itoa(done)+"/"+strconv.Itoa(len(todos)))}
	for i, t := range todos {
		if i == 8 {
			rows = append(rows, mutedStyle.Render("… +"+strconv.Itoa(len(todos)-8)))
			break
		}
		text := fit(t.Content, cw-2)
		switch t.Status {
		case "completed":
			rows = append(rows, successStyle.Render("✓ ")+faintStyle.Render(text))
		case "in_progress":
			rows = append(rows, primaryStyle.Render("▶ ")+textStyle.Render(text))
		default:
			rows = append(rows, mutedStyle.Render("○ "+text))
		}
	}
	return append(rows, "")
}
