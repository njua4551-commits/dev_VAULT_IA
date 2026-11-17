package ui

import (
	"strings"
	"testing"

	"nosecode/internal/tools"
)

func TestSandboxSlash(t *testing.T) {
	t.Setenv("NOSECODE_HOME", t.TempDir())
	defer tools.SetSandbox(tools.SandboxConfig{})
	m := newTestModel(t)
	m = m.sandboxSlash(nil)
	if last := m.msgs[len(m.msgs)-1].Text; !strings.Contains(last, "desactivado") {
		t.Fatal(last)
	}
	m = m.sandboxSlash([]string{"on"})
	if m.cfg.Sandbox.Mode != "on" {
		t.Fatal("debía guardar el modo")
	}
	m = m.sandboxSlash([]string{"net", "off"})
	if !m.cfg.Sandbox.NoNetwork {
		t.Fatal("net off = sin red")
	}
	m = m.sandboxSlash([]string{"off"})
	if m.cfg.Sandbox.Mode != "" {
		t.Fatal("off limpia el modo")
	}
	m = m.sandboxSlash([]string{"raro"})
	if !m.toastErr {
		t.Fatal("argumento inválido debía avisar")
	}
}
