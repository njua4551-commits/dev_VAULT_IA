package ui

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
	"unicode"

	"nosecode/internal/config"

	"github.com/atotto/clipboard"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// selectState guarda un arrastre de ratón sobre un panel.
type selectState struct {
	dragging bool
	moved    bool
	active   bool
	startX   int
	startY   int
	endX     int
	endY     int

	panel       string
	panelTop    int
	panelBottom int
	panelX0     int
	panelX1     int
}

// screenBuffer conserva el último render para copiar por coordenadas.
type screenBuffer struct {
	lines  []uiLine
	width  int
	height int
}

// normalized ordena los extremos del arrastre por fila y columna.
func (s selectState) normalized() (y0, x0, y1, x1 int) {
	y0, x0, y1, x1 = s.startY, s.startX, s.endY, s.endX
	if y0 > y1 || (y0 == y1 && x0 > x1) {
		y0, x0, y1, x1 = y1, x1, y0, x0
	}
	return
}

// rangeFor devuelve las columnas resaltadas/copiadas de una línea. Solo cubre
// texto real: ni relleno, ni bordes, ni la barra lateral.
func (s selectState) rangeFor(y int, ln uiLine) (int, int, bool) {
	if (!s.active && !s.dragging) || !s.moved {
		return 0, 0, false
	}
	y0, x0, y1, x1 := s.normalized()
	if y < y0 || y > y1 || y < s.panelTop || y > s.panelBottom || ln.panel != s.panel || !ln.selectable {
		return 0, 0, false
	}
	left, right := ln.selStart, ln.selEnd
	if y == y0 && x0 > left {
		left = x0
	}
	if y == y1 && x1 < right {
		right = x1
	}
	if left < s.panelX0 {
		left = s.panelX0
	}
	if right > s.panelX1 {
		right = s.panelX1
	}
	if right <= left {
		return 0, 0, false
	}
	return left, right, true
}

// selectionView resalta solo las columnas seleccionadas.
func (m Model) selectionView(lines []uiLine) []uiLine {
	for y := range lines {
		left, right, ok := m.sel.rangeFor(y, lines[y])
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

// panelBounds encuentra el bloque contiguo del panel que contiene y.
func (m Model) panelBounds(y int) (int, int, bool) {
	if m.buf == nil || y < 0 || y >= len(m.buf.lines) {
		return 0, 0, false
	}
	panel := m.buf.lines[y].panel
	if panel == "" {
		return 0, 0, false
	}
	top, bottom := y, y
	for top > 0 && m.buf.lines[top-1].panel == panel {
		top--
	}
	for bottom+1 < len(m.buf.lines) && m.buf.lines[bottom+1].panel == panel {
		bottom++
	}
	return top, bottom, true
}

// cleanCopyLine quita relleno final y barras de panel, sin tocar la sangría
// real del contenido (importante en bloques de código).
func cleanCopyLine(s string) string {
	s = strings.TrimRight(s, " \t")
	for {
		trimmed := false
		for _, bar := range []string{"┃", "│", "▌", "▐"} {
			if strings.HasPrefix(s, bar) {
				s = strings.TrimPrefix(s, bar)
				s = strings.TrimPrefix(s, " ")
				trimmed = true
				break
			}
		}
		if !trimmed {
			break
		}
	}
	s = strings.TrimRight(s, "┃│▌▐")
	return strings.TrimRight(s, " \t")
}

// selectedText extrae el texto plano de la selección actual.
func (m Model) selectedText() string {
	if m.buf == nil || len(m.buf.lines) == 0 {
		return ""
	}
	y0, _, y1, _ := m.sel.normalized()
	if y0 < 0 {
		y0 = 0
	}
	if y1 >= len(m.buf.lines) {
		y1 = len(m.buf.lines) - 1
	}
	var out []string
	for y := y0; y <= y1; y++ {
		ln := m.buf.lines[y]
		if ln.panel != m.sel.panel {
			continue
		}
		left, right, ok := m.sel.rangeFor(y, ln)
		if !ok {
			if y > y0 && y < y1 {
				out = append(out, "")
			}
			continue
		}
		line := ansi.Strip(ansi.Cut(ln.text, left, right))
		out = append(out, cleanCopyLine(line))
	}
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return strings.Join(out, "\n")
}

// displayWidth mide runas como celdas de terminal.
func displayWidth(runes []rune) int {
	if len(runes) == 0 {
		return 0
	}
	return lipgloss.Width(string(runes))
}

// runeIndexAtColumn devuelve la runa que ocupa una columna visual.
func runeIndexAtColumn(runes []rune, col int) int {
	if col < 0 {
		return 0
	}
	x := 0
	for i, r := range runes {
		w := lipgloss.Width(string(r))
		if w < 1 {
			w = 1
		}
		if col < x+w {
			return i
		}
		x += w
	}
	return len(runes) - 1
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-'
}

// wordRange encuentra la palabra bajo una columna de la línea.
func wordRange(ln uiLine, x int) (int, int, bool) {
	if !ln.selectable || ln.selEnd <= ln.selStart {
		return 0, 0, false
	}
	if x < ln.selStart {
		x = ln.selStart
	}
	if x >= ln.selEnd {
		x = ln.selEnd - 1
	}
	text := ansi.Strip(ansi.Cut(ln.text, ln.selStart, ln.selEnd))
	runes := []rune(text)
	if len(runes) == 0 {
		return 0, 0, false
	}
	idx := runeIndexAtColumn(runes, x-ln.selStart)
	if idx < 0 || idx >= len(runes) {
		return 0, 0, false
	}
	if unicode.IsSpace(runes[idx]) {
		return 0, 0, false
	}
	start, end := idx, idx+1
	for start > 0 && isWordRune(runes[start-1]) {
		start--
	}
	for end < len(runes) && isWordRune(runes[end]) {
		end++
	}
	if start == end {
		return 0, 0, false
	}
	return ln.selStart + displayWidth(runes[:start]), ln.selStart + displayWidth(runes[:end]), true
}

func (m Model) selectWordAt(x, y int) Model {
	if m.buf == nil || y < 0 || y >= len(m.buf.lines) {
		return m
	}
	ln := m.buf.lines[y]
	if ln.panel != "chat" || !ln.selectable {
		return m
	}
	start, end, ok := wordRange(ln, x)
	if !ok {
		m.sel = selectState{}
		return m
	}
	top, bottom, ok := m.panelBounds(y)
	if !ok {
		return m
	}
	m.sel = selectState{
		active:      true,
		moved:       true,
		startX:      start,
		startY:      y,
		endX:        end,
		endY:        y,
		panel:       ln.panel,
		panelTop:    top,
		panelBottom: bottom,
		panelX0:     ln.panelX0,
		panelX1:     ln.panelX1,
	}
	return m.copySelection()
}

// selectMessageAt selecciona el cuerpo de un mensaje, sin su cabecera.
func (m Model) selectMessageAt(x, y int) Model {
	if m.buf == nil || y < 0 || y >= len(m.buf.lines) {
		return m
	}
	ln := m.buf.lines[y]
	if !ln.hasMsg {
		return m
	}
	first, last := -1, -1
	for i, row := range m.buf.lines {
		if row.hasMsg && row.msgIndex == ln.msgIndex && row.msgBody && row.selectable {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return m
	}
	top, bottom, ok := m.panelBounds(first)
	if !ok {
		return m
	}
	m.sel = selectState{
		active:      true,
		moved:       true,
		startX:      m.buf.lines[first].selStart,
		startY:      first,
		endX:        m.buf.lines[last].selEnd,
		endY:        last,
		panel:       ln.panel,
		panelTop:    top,
		panelBottom: bottom,
		panelX0:     ln.panelX0,
		panelX1:     ln.panelX1,
	}
	return m.copySelection()
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

// lastAssistant devuelve la última respuesta real del asistente.
func (m Model) lastAssistant() (Message, bool) {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		msg := m.msgs[i]
		if msg.Role != "assistant" || strings.TrimSpace(msg.Text) == "" {
			continue
		}
		if strings.HasPrefix(msg.Text, vaultTag) || strings.HasPrefix(msg.Text, toolTag) {
			continue
		}
		return msg, true
	}
	return Message{}, false
}

// copyLast copia la última respuesta completa y guarda los bloques de código
// en archivos aparte.
func (m Model) copyLast() Model {
	msg, ok := m.lastAssistant()
	if !ok {
		if text := m.selectedText(); strings.TrimSpace(text) != "" {
			return m.copyText(text, "selección")
		}
		m.toast, m.toastErr = "No hay respuesta para copiar", true
		return m
	}
	m = m.copyText(msg.Text, "última respuesta")
	if m.toastErr {
		return m
	}
	blocks := extractCodeBlocks(msg.Text)
	if len(blocks) == 0 {
		return m
	}
	if dir, err := saveCodeBlocks(blocks); err == nil {
		m.toast += fmt.Sprintf(" · %d bloque(s) en %s", len(blocks), shortPath(dir))
	}
	return m
}

// copyCommand atiende /copy, /copy code [n] y /copy files.
func (m Model) copyCommand(rest string) Model {
	arg := strings.ToLower(strings.TrimSpace(rest))
	switch {
	case arg == "":
		return m.copyLast()
	case arg == "files", arg == "archivos":
		return m.saveCodeFiles()
	case strings.HasPrefix(arg, "code"):
		return m.copyCode(strings.TrimSpace(strings.TrimPrefix(arg, "code")))
	default:
		m.toast, m.toastErr = "Uso: /copy · /copy code [n] · /copy files", true
		return m
	}
}

func (m Model) copyCode(which string) Model {
	msg, ok := m.lastAssistant()
	if !ok {
		m.toast, m.toastErr = "No hay respuesta para copiar", true
		return m
	}
	blocks := extractCodeBlocks(msg.Text)
	if len(blocks) == 0 {
		m.toast, m.toastErr = "La última respuesta no tiene bloques de código", true
		return m
	}
	idx := 0
	if which != "" {
		n, err := strconv.Atoi(which)
		if err != nil || n < 1 || n > len(blocks) {
			m.toast, m.toastErr = fmt.Sprintf("Hay %d bloque(s): /copy code 1..%d", len(blocks), len(blocks)), true
			return m
		}
		idx = n - 1
	} else if len(blocks) > 1 {
		m.toast, m.toastErr = fmt.Sprintf("Hay %d bloques: /copy code 1..%d", len(blocks), len(blocks)), true
		return m
	}
	label := fmt.Sprintf("bloque %d/%d", idx+1, len(blocks))
	if lang := strings.TrimSpace(blocks[idx].Lang); lang != "" {
		label += " · " + lang
	}
	return m.copyText(blocks[idx].Body, label)
}

func (m Model) saveCodeFiles() Model {
	msg, ok := m.lastAssistant()
	if !ok {
		m.toast, m.toastErr = "No hay respuesta para copiar", true
		return m
	}
	blocks := extractCodeBlocks(msg.Text)
	if len(blocks) == 0 {
		m.toast, m.toastErr = "La última respuesta no tiene bloques de código", true
		return m
	}
	dir, err := saveCodeBlocks(blocks)
	if err != nil {
		m.toast, m.toastErr = "No pude guardar los bloques: "+err.Error(), true
		return m
	}
	m.toast, m.toastErr = fmt.Sprintf("%d bloque(s) guardados en %s", len(blocks), shortPath(dir)), false
	m.sound(soundDone)
	return m
}

type codeBlock struct {
	Lang string
	Body string
}

// extractCodeBlocks separa los bloques ```lang ... ``` de una respuesta.
func extractCodeBlocks(text string) []codeBlock {
	var (
		blocks []codeBlock
		lang   string
		body   []string
		in     bool
	)
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") {
			if !in {
				in = true
				lang = strings.TrimSpace(strings.TrimPrefix(line, "```"))
				body = nil
			} else {
				blocks = append(blocks, codeBlock{Lang: lang, Body: strings.Join(body, "\n")})
				in = false
			}
			continue
		}
		if in {
			body = append(body, raw)
		}
	}
	if in {
		blocks = append(blocks, codeBlock{Lang: lang, Body: strings.Join(body, "\n")})
	}
	return blocks
}

func saveCodeBlocks(blocks []codeBlock) (string, error) {
	dir := filepath.Join(config.Home(), "copies")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	stamp := time.Now().Format("20060102-150405")
	for i, b := range blocks {
		name := fmt.Sprintf("%s-%02d.%s", stamp, i+1, codeExt(b.Lang))
		if err := os.WriteFile(filepath.Join(dir, name), []byte(b.Body), 0o600); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func codeExt(lang string) string {
	switch strings.ToLower(strings.TrimSpace(lang)) {
	case "go", "golang":
		return "go"
	case "python", "py":
		return "py"
	case "javascript", "js":
		return "js"
	case "typescript", "ts":
		return "ts"
	case "json":
		return "json"
	case "bash", "sh", "shell", "zsh":
		return "sh"
	case "powershell", "ps1", "pwsh":
		return "ps1"
	case "html":
		return "html"
	case "css":
		return "css"
	case "sql":
		return "sql"
	case "yaml", "yml":
		return "yaml"
	case "markdown", "md":
		return "md"
	case "rust", "rs":
		return "rs"
	case "java":
		return "java"
	case "c":
		return "c"
	case "cpp", "c++":
		return "cpp"
	default:
		return "txt"
	}
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
