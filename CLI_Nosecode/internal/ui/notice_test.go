package ui

import (
	"strings"
	"testing"
	"time"

	"nosecode/internal/api"
)

func TestUpDownNoEnsucianElChat(t *testing.T) {
	m := newTestModel(t)
	m.run.vault = &api.VaultRun{}
	m.vaultQuiet, m.vaultCmd = true, "up"
	before := len(m.msgs)
	for _, l := range []string{"Kernel iniciando…", "Descargando modelo 40%", "Listo: https://x.ngrok.app"} {
		nm, _ := m.onVaultQuiet(vaultMsg{v: m.run.vault, l: api.VaultLine{Text: l}})
		m = nm.(Model)
	}
	if len(m.msgs) != before {
		t.Fatalf("el chat no debe cambiar: %d → %d", before, len(m.msgs))
	}
	if !strings.Contains(m.activityLineText(), "Listo: https://x.ngrok.app") {
		t.Fatalf("aviso: %q", m.activityLineText())
	}
	if len(m.vaultLog) != 3 {
		t.Fatalf("bitácora: %v", m.vaultLog)
	}
	// error: aviso rojo y detalle a pedido
	nm, _ := m.onVaultQuiet(vaultMsg{v: m.run.vault, l: api.VaultLine{Done: true, Err: errTest("falló")}})
	m = nm.(Model)
	if !m.vaultNoticeErr || !strings.Contains(m.activeNotice(), "falló") || len(m.msgs) != before {
		t.Fatalf("error: %q msgs=%d", m.activeNotice(), len(m.msgs))
	}
	m = m.vaultLogSlash()
	if len(m.msgs) != before+1 || !strings.Contains(m.msgs[before].Text, "Descargando modelo") {
		t.Fatal("/vault log debe mostrar la bitácora")
	}
}

type errTest string

func (e errTest) Error() string { return string(e) }

func TestAvisoCaduca(t *testing.T) {
	m := newTestModel(t)
	m.notify("hola", false, 50*time.Millisecond)
	if m.activeNotice() != "hola" {
		t.Fatal("vigente")
	}
	time.Sleep(80 * time.Millisecond)
	if m.activeNotice() != "" {
		t.Fatal("debe caducar")
	}
}
