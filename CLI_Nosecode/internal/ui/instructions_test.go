package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadInstructions(t *testing.T) {
	cwd, home := t.TempDir(), t.TempDir()
	mem := filepath.Join(cwd, "NOSECODE.md")
	os.WriteFile(mem, []byte("nota de memoria"), 0o600)
	os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte("regla del proyecto"), 0o600)
	os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte("regla del usuario"), 0o600)
	got := loadInstructions(cwd, home, mem)
	for _, want := range []string{"nota de memoria", "regla del proyecto", "regla del usuario"} {
		if !strings.Contains(got, want) {
			t.Fatalf("falta %q en %q", want, got)
		}
	}
	// tope total
	os.WriteFile(filepath.Join(cwd, "AGENTS.md"), []byte(strings.Repeat("x", 100<<10)), 0o600)
	os.WriteFile(filepath.Join(home, "AGENTS.md"), []byte(strings.Repeat("y", 100<<10)), 0o600)
	if n := len(loadInstructions(cwd, home, mem)); n > instructionsCap+2<<10 {
		t.Fatalf("demasiado grande: %d", n)
	}
	// sin archivos: vacío
	if loadInstructions(t.TempDir(), t.TempDir(), "") != "" {
		t.Fatal("debe ser vacío")
	}
}

func TestUnknownToolListsTools(t *testing.T) {
	m := newTestModel(t)
	msg := m.unknownToolMessage("subagente")
	if !strings.Contains(msg, "subagente") || !strings.Contains(msg, "read_file") || !strings.Contains(msg, "subagentes") {
		t.Fatalf("mensaje: %s", msg)
	}
}

func TestMaxOutputTokensYReserva(t *testing.T) {
	m := newTestModel(t)
	if m.cfg.Tools.CompactReserved != 8192 {
		t.Fatalf("compact_reserved por defecto: %d", m.cfg.Tools.CompactReserved)
	}
	m.contextWin = 100000
	m.usageValid, m.usageTotal = true, 50000
	if got := m.maxOutputTokens(); got != 49936 {
		t.Fatalf("max_tokens: %d", got)
	}
	m.usageTotal = 99900
	if got := m.maxOutputTokens(); got != 256 {
		t.Fatalf("mínimo: %d", got)
	}
	// aviso por colchón: 92000 de 100000 es 92 % (>80), prueba con ventana grande
	m.contextWin = 200000
	m.usageTotal = 195000 // 97 %
	m.ctxWarned = false
	if !m.warnContext().ctxWarned {
		t.Fatal("debe avisar")
	}
	m.cfg.Tools.CompactReserved = 100000
	m.usageTotal = 120000 // 60 % pero dentro del colchón (200000-100000)
	m.ctxWarned = false
	if !m.warnContext().ctxWarned {
		t.Fatal("debe avisar por el colchón")
	}
}
