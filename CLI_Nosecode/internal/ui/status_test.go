package ui

import (
	"strings"
	"testing"
	"time"

	"nosecode/internal/api"
)

func TestContadorContextoSeActualizaYReemplaza(t *testing.T) {
	m := newTestModel(t)
	m.usageValid = true
	m.usageTotal = 1000
	m = m.addContextEstimate(strings.Repeat("x", 400))
	if m.usageTotal != 1100 {
		t.Fatalf("tras herramienta = %d, quería 1100", m.usageTotal)
	}

	s := &api.Streamer{}
	m.run.agent = &agentRun{stream: s}
	m.msgs = append(m.msgs, Message{Role: "assistant"})
	updated, _ := m.onAgentChunk(chunkMsg{s: s, c: api.Chunk{
		Usage: &api.Usage{TotalTokens: 77},
		Base:  "https://tunel.ejemplo/v1",
	}})
	nm := updated.(Model)
	if nm.usageTotal != 77 || !nm.usageValid {
		t.Fatalf("usage real = %d valid=%v", nm.usageTotal, nm.usageValid)
	}
	if nm.lastServer != "https://tunel.ejemplo/v1" || nm.lastServerAt.IsZero() {
		t.Fatalf("última respuesta = %q %v", nm.lastServer, nm.lastServerAt)
	}
}

func TestContadorContextoSeInvalidaSinUsage(t *testing.T) {
	m := newTestModel(t)
	m.usageValid = true
	m.usageTotal = 900
	s := &api.Streamer{}
	m.run.agent = &agentRun{stream: s}
	m.msgs = append(m.msgs, Message{Role: "assistant"})
	updated, _ := m.onAgentChunk(chunkMsg{s: s, c: api.Chunk{Done: true}})
	nm := updated.(Model)
	if nm.usageValid {
		t.Fatal("sin usage el contador no debe quedarse en un valor viejo")
	}
}

func TestStatusReportIncluyeFaseYCola(t *testing.T) {
	m := newTestModel(t)
	m.contextWin = 1000
	m.usageValid = true
	m.usageTotal = 250
	m.run.agent = &agentRun{steps: 3}
	m.queue = []string{"pendiente"}
	m.lastServer = "https://tunel.ejemplo/v1"
	m.lastServerAt = time.Now()
	report := m.statusReport()
	for _, want := range []string{"Fase:", "Pasos: 3/", "Mensajes en cola: 1", "Contexto: 250 tok · 25% (real)", "tunel.ejemplo"} {
		if !strings.Contains(report, want) {
			t.Fatalf("falta %q en:\n%s", want, report)
		}
	}
}

func TestBashWaitModalSinProceso(t *testing.T) {
	m := newTestModel(t)
	m.bashWait = bashWaitState{elapsed: 60, timeoutS: 60}
	lines := m.bashWaitScreen()
	if len(lines) == 0 {
		t.Fatal("el modal de timeout no debe estar vacío")
	}
}
