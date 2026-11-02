package ui

import (
	"strings"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/x/ansi"
)

// selectState guarda un arrastre de ratón sobre el lienzo.
type selectState struct {
	dragging bool
	moved    bool
	active   bool
	startX   int
	startY   int
	endX     int
	endY     int
}

// screenBuffer conserva el último render para copiar por coordenadas.
type screenBuffer struct {
	lines  []uiLine
	width  int
	height int
}

func (s selectState) rangeFor(y, width int) (int, int, bool) {
	if !s.active && !s.dragging {
		return 0, 0, false
	}
	if !s.moved {
		return 0, 0, false
	}
	y0, y1 := s.startY, s.endY
	x0, x1 := s.startX, s.endX
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		y0, y1 = y1, y0
		x0, x1 = x1, x0
	}
	if y < y0 || y > y1 {
		return 0, 0, false
	}
	left, right := 0, width
	if y == y0 {
		left = x0
	}
	if y == y1 {
		right = x1
	}
	if left < 0 {
		left = 0
	}
	if right > width {
		right = width
	}
	if right <= left {
		return 0, 0, false
	}
	return left, right, true
}

// selectionView resalta la selección sin perder el ancho ni los estilos base.
func (m Model) selectionView(lines []uiLine) []uiLine {
	for y := range lines {
		left, right, ok := m.sel.rangeFor(y, m.width)
		if !ok {
			continue
		}
		styled := lines[y].text
		before := ansi.Cut(styled, 0, left)
		mid := ansi.Cut(styled, left, right)
		after := ansi.Cut(styled, right, m.width)
		lines[y].text = before + paintWith(selectStyle, mid) + after
	}
	return lines
}

// selectedText extrae el texto plano de la selección actual.
func (m Model) selectedText() string {
	if m.buf == nil || len(m.buf.lines) == 0 {
		return ""
	}
	var out []string
	for y := 0; y < len(m.buf.lines); y++ {
		left, right, ok := m.sel.rangeFor(y, m.buf.width)
		if !ok {
			continue
		}
		styled := ansi.Cut(m.buf.lines[y].text, left, right)
		line := strings.TrimRight(ansi.Strip(styled), " ")
		out = append(out, line)
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// copyText escribe en el portapapeles del sistema.
func (m Model) copyText(text, label string) Model {
	text = strings.TrimRight(text, "\n")
	if strings.TrimSpace(text) == "" {
		m.toast, m.toastErr = "No hay texto para copiar", true
		return m
	}
	if err := clipboard.WriteAll(text); err != nil {
		m.toast, m.toastErr = "No pude copiar: "+err.Error(), true
		m.sound(soundError)
		return m
	}
	n := len([]rune(text))
	m.toast, m.toastErr = "Copiado: "+label+" ("+itoa(n)+" caracteres)", false
	m.sound(soundDone)
	return m
}

// copySelection copia el arrastre actual.
func (m Model) copySelection() Model {
	text := m.selectedText()
	if strings.TrimSpace(text) == "" {
		m.toast, m.toastErr = "Selecciona texto con el ratón", true
		return m
	}
	return m.copyText(text, "selección")
}

// copyLast copia la última respuesta del asistente.
func (m Model) copyLast() Model {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		msg := m.msgs[i]
		if msg.Role != "assistant" || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		if strings.HasPrefix(msg.Text, vaultTag) || strings.HasPrefix(msg.Text, toolTag) {
			continue
		}
		return m.copyText(msg.Text, "última respuesta")
	}
	if text := m.selectedText(); strings.TrimSpace(text) != "" {
		return m.copyText(text, "selección")
	}
	m.toast, m.toastErr = "No hay respuesta para copiar", true
	return m
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
