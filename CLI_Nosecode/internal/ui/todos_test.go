package ui

import (
	"strings"
	"testing"

	"nosecode/internal/session"
	"nosecode/internal/tools"
)

func TestTodoHerramientaYSidebar(t *testing.T) {
	res, _ := tools.Lookup("todo")
	r := res.Run(nil, []byte(`{"todos":[{"content":"leer","status":"done"},{"content":"editar","status":"en curso"},{"content":"probar","status":"x"}]}`))
	if r.Error || !strings.Contains(r.Text, "3 tareas, 1 hechas, 1 en curso") {
		t.Fatalf("%+v", r)
	}
	m := newTestModel(t)
	m.msgs = append(m.msgs, Message{Role: "assistant", Tool: &session.ToolBlock{Name: "todo", Status: toolOK, Output: r.Text}})
	todos := m.currentTodos()
	if len(todos) != 3 || todos[0].Status != "completed" || todos[1].Status != "in_progress" || todos[2].Status != "pending" {
		t.Fatalf("%+v", todos)
	}
	rows := strings.Join(m.todoRailRows(20), "\n")
	if !strings.Contains(rows, "1/3") || !strings.Contains(rows, "editar") {
		t.Fatalf("rail: %q", rows)
	}
	if rr := res.Run(nil, []byte(`{"todos":[]}`)); !rr.Error {
		t.Fatal("vacía debe fallar")
	}
}

func TestTodoPermitidaEnPlan(t *testing.T) {
	if !planToolAllowed("todo") {
		t.Fatal("todo debe estar en Plan")
	}
}
