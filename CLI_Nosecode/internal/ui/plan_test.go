package ui

import (
	"strings"
	"testing"

	"nosecode/internal/api"
)

func callOf(name, args string) api.ToolCall {
	c := api.ToolCall{ID: "c1", Type: "function"}
	c.Function.Name, c.Function.Arguments = name, args
	return c
}

func TestPlanSoloLectura(t *testing.T) {
	m := newTestModel(t)
	m.mode = "plan"
	defs := m.apiTools()
	for _, bad := range []string{"write_file", "edit_file", "remember"} {
		if hasTool(defs, bad) {
			t.Fatalf("%s no debe ofrecerse en Plan", bad)
		}
	}
	for _, ok := range []string{"read_file", "list_dir", "grep", "web_fetch", "question", "bash"} {
		if !hasTool(defs, ok) {
			t.Fatalf("falta %s en Plan: %v", ok, defNames(defs))
		}
	}
	if !m.planDenied("write_file") || m.planDenied("read_file") {
		t.Fatal("planDenied incorrecto")
	}
	if d, _ := m.bashDecision(callOf("bash", `{"command":"rm -rf x"}`)); d != "deny" {
		t.Fatalf("bash destructivo en Plan: %s", d)
	}
	if d, _ := m.bashDecision(callOf("bash", `{"command":"git status"}`)); d != "allow" {
		t.Fatalf("git status en Plan: %s", d)
	}
	if !strings.Contains(m.systemPrompt(), "MODO PLAN") {
		t.Fatal("el prompt debe indicar el modo Plan")
	}
}

func TestBashDecisionBuild(t *testing.T) {
	m := newTestModel(t)
	if d, _ := m.bashDecision(callOf("bash", `{"command":"pwd"}`)); d != "allow" {
		t.Fatalf("pwd: %s", d)
	}
	if m.callNeedsApproval(callOf("bash", `{"command":"pwd"}`)) {
		t.Fatal("pwd no debe pedir permiso")
	}
	if !m.callNeedsApproval(callOf("bash", `{"command":"npm install"}`)) {
		t.Fatal("npm install debe pedir permiso")
	}
	m.cfg.Permissions.Bash = "deny"
	if d, _ := m.bashDecision(callOf("bash", `{"command":"pwd"}`)); d != "deny" {
		t.Fatalf("permissions.bash=deny: %s", d)
	}
}

func TestQuestionTeclas(t *testing.T) {
	m := newTestModel(t)
	m.question = questionState{options: []string{"a", "b"}}
	_, ans := m.questionKey("2", nil, "")
	if ans != "b" {
		t.Fatalf("tecla 2: %q", ans)
	}
	m2, _ := m.questionKey("o", nil, "")
	m2, _ = m2.questionKey("x", []rune("hola"), "")
	_, ans = m2.questionKey("enter", nil, "enter")
	if ans != "hola" {
		t.Fatalf("texto libre: %q", ans)
	}
	_, ans = m.questionKey("3", nil, "")
	if ans != "" {
		t.Fatal("opción inexistente")
	}
}

func TestLoopRepeated(t *testing.T) {
	a := &agentRun{}
	c := callOf("bash", `{"command":"ls"}`)
	if loopRepeated(a, c) || loopRepeated(a, c) {
		t.Fatal("las dos primeras son válidas")
	}
	if !loopRepeated(a, c) {
		t.Fatal("la tercera seguida debe bloquearse")
	}
	if loopRepeated(a, callOf("bash", `{"command":"pwd"}`)) {
		t.Fatal("otra llamada reinicia la cuenta")
	}
	q := callOf("question", `{"question":"x","options":["a","b"]}`)
	for i := 0; i < 5; i++ {
		if loopRepeated(a, q) {
			t.Fatal("question nunca se bloquea")
		}
	}
}
