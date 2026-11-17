package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"nosecode/internal/api"
)

func TestKeyToBytes(t *testing.T) {
	cases := map[string]tea.KeyMsg{
		"\r":      {Type: tea.KeyEnter},
		"\x03":    {Type: tea.KeyCtrlC},
		"\x04":    {Type: tea.KeyCtrlD},
		"\x1b[A":  {Type: tea.KeyUp},
		"\x7f":    {Type: tea.KeyBackspace},
		"ñ":       {Type: tea.KeyRunes, Runes: []rune("ñ")},
		"\x1bx":   {Type: tea.KeyRunes, Runes: []rune("x"), Alt: true},
		"\x1b[5~": {Type: tea.KeyPgUp},
	}
	for want, k := range cases {
		if got := string(keyToBytes(k)); got != want {
			t.Errorf("%v → %q, quería %q", k, got, want)
		}
	}
}

func TestTerminalAprobacionSoloAlAbrir(t *testing.T) {
	m := newTestModel(t)
	call := func(args string) api.ToolCall {
		c := api.ToolCall{ID: "1"}
		c.Function.Name, c.Function.Arguments = "terminal", args
		return c
	}
	if !m.callNeedsApproval(call(`{"action":"open","command":"python3"}`)) {
		t.Error("abrir una terminal debe pedir permiso")
	}
	if m.callNeedsApproval(call(`{"action":"send","id":1,"text":"1+1\n"}`)) {
		t.Error("send a una sesión ya aprobada no debe pedir permiso")
	}
	if got := terminalAction(call(`{"command":"ls"}`)); got != "open" {
		t.Errorf("sin action + command debe ser open, fue %q", got)
	}
}
