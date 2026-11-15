package ui

import (
	"fmt"
	"strings"
	"time"
)

// statusReport describe el estado del agente para /status.
func (m Model) statusReport() string {
	var out strings.Builder
	phase := "listo"
	secs := 0
	switch {
	case m.compacting:
		phase = "compactando"
		secs = int(time.Since(m.activity.started).Seconds())
	case m.bashWait.proc != nil:
		phase = "timeout suave (esperando decisión)"
		secs = m.bashWait.elapsed
	case m.screen == screenTool:
		phase = "esperando tu aprobación"
	case m.run.agent != nil && m.run.agent.paused:
		phase = "pausado"
	case m.activity.kind != "":
		phase = m.activity.kind
		secs = int(time.Since(m.activity.started).Seconds())
	}
	fmt.Fprintf(&out, "Modo: %s\n", modeLabel(m.mode))
	fmt.Fprintf(&out, "Fase: %s", phase)
	if secs > 0 {
		fmt.Fprintf(&out, " · %d s", secs)
	}
	out.WriteString("\n")

	tool := "ninguna"
	if blk := m.activeToolBlock(); blk != nil {
		args := oneLine(blk.Args, 90)
		tool = blk.Name
		if args != "" {
			tool += " " + args
		}
		if !blk.Started.IsZero() {
			tool += fmt.Sprintf(" · %d s", int(time.Since(blk.Started).Seconds()))
		}
	}
	fmt.Fprintf(&out, "Herramienta en curso: %s\n", tool)

	if m.lastServerAt.IsZero() {
		out.WriteString("Última respuesta del servidor: (ninguna en esta sesión)\n")
	} else {
		fmt.Fprintf(&out, "Última respuesta del servidor: %s · hace %d s\n",
			serverLabel(m.lastServer), int(time.Since(m.lastServerAt).Seconds()))
	}

	pending := 0
	if m.screen == screenTool {
		pending = len(m.tool.calls)
	} else {
		for i := range m.msgs {
			if m.msgs[i].Tool != nil && m.msgs[i].Tool.Status == toolApproval {
				pending++
			}
		}
	}
	fmt.Fprintf(&out, "Aprobaciones pendientes: %d\n", pending)
	fmt.Fprintf(&out, "Mensajes en cola: %d\n", len(m.queue))
	if a := m.run.agent; a != nil {
		fmt.Fprintf(&out, "Pasos: %d/%d\n", a.steps, m.maxSteps())
	} else {
		fmt.Fprintf(&out, "Pasos: 0/%d\n", m.maxSteps())
	}
	kind := "estimado"
	if m.usageValid {
		kind = "real"
	}
	fmt.Fprintf(&out, "Contexto: %d tok · %d%% (%s)\n", m.contextUsed(), m.contextPercent(), kind)
	return strings.TrimRight(out.String(), "\n")
}

// serverLabel acorta una URL base para mostrarla en /status.
func serverLabel(base string) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return "(desconocido)"
	}
	base = strings.TrimSuffix(base, "/v1")
	base = strings.TrimPrefix(base, "https://")
	base = strings.TrimPrefix(base, "http://")
	base = strings.TrimSuffix(base, "/")
	return base
}
