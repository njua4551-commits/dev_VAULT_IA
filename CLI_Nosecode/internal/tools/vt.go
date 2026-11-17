package tools

import (
	"strconv"
	"strings"
	"sync"
	"unicode/utf8"

	"github.com/mattn/go-runewidth"
)

// vt es un emulador de terminal mínimo (suficiente para que el modelo «vea» la
// pantalla de programas interactivos: consolas, instaladores, editores simples).
// No guarda colores ni atributos: solo texto y posición del cursor.

const (
	vtMaxScrollback = 1000
	vtCont          = rune(-1) // segunda celda de un carácter ancho
)

type vt struct {
	mu         sync.Mutex
	cols, rows int
	main, alt  [][]rune
	cells      [][]rune // apunta a main o alt
	onAlt      bool
	cx, cy     int
	wrapNext   bool
	top, bot   int // región de scroll (0-based, inclusiva)
	savedX     int
	savedY     int
	hideCursor bool
	history    []string
	title      string
	reply      func([]byte)

	state  int
	params []byte
	inter  []byte
	osc    []byte
	pend   []byte // bytes UTF-8 incompletos
}

const (
	vtGround = iota
	vtEsc
	vtEscInter // ESC ( B etc.: consume un byte
	vtCSI
	vtOSC
	vtOSCEsc
	vtStr // DCS/APC/PM/SOS: ignorar hasta ST
	vtStrEsc
)

func newVT(cols, rows int, reply func([]byte)) *vt {
	v := &vt{cols: cols, rows: rows, reply: reply}
	v.main, v.alt = blankGrid(cols, rows), blankGrid(cols, rows)
	v.cells = v.main
	v.bot = rows - 1
	return v
}

func blankGrid(cols, rows int) [][]rune {
	g := make([][]rune, rows)
	for i := range g {
		g[i] = make([]rune, cols)
	}
	return g
}

// Resize cambia el tamaño conservando lo que cabe.
func (v *vt) Resize(cols, rows int) {
	v.mu.Lock()
	defer v.mu.Unlock()
	conv := func(g [][]rune) [][]rune {
		n := blankGrid(cols, rows)
		for y := 0; y < rows && y < len(g); y++ {
			copy(n[y], g[y])
		}
		return n
	}
	v.main, v.alt = conv(v.main), conv(v.alt)
	if v.onAlt {
		v.cells = v.alt
	} else {
		v.cells = v.main
	}
	v.cols, v.rows = cols, rows
	v.top, v.bot = 0, rows-1
	v.cx, v.cy = min(v.cx, cols-1), min(v.cy, rows-1)
	v.wrapNext = false
}

// Write interpreta los bytes que emite el programa.
func (v *vt) Write(p []byte) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if len(v.pend) > 0 {
		p = append(v.pend, p...)
		v.pend = nil
	}
	for i := 0; i < len(p); {
		b := p[i]
		if b >= 0x80 && v.state == vtGround {
			if !utf8.FullRune(p[i:]) {
				v.pend = append([]byte(nil), p[i:]...)
				return
			}
			r, n := utf8.DecodeRune(p[i:])
			i += n
			v.put(r)
			continue
		}
		i++
		v.feed(b)
	}
}

func (v *vt) feed(b byte) {
	switch v.state {
	case vtGround:
		v.ground(b)
	case vtEsc:
		v.escape(b)
	case vtEscInter:
		v.state = vtGround
	case vtCSI:
		v.csiByte(b)
	case vtOSC:
		switch b {
		case 0x07:
			v.endOSC()
		case 0x1b:
			v.state = vtOSCEsc
		default:
			if len(v.osc) < 512 {
				v.osc = append(v.osc, b)
			}
		}
	case vtOSCEsc:
		v.endOSC()
		if b != '\\' {
			v.state = vtEsc
			v.escape(b)
		}
	case vtStr:
		if b == 0x1b {
			v.state = vtStrEsc
		} else if b == 0x07 {
			v.state = vtGround
		}
	case vtStrEsc:
		v.state = vtGround
		if b != '\\' {
			v.state = vtEsc
			v.escape(b)
		}
	}
}

func (v *vt) endOSC() {
	s := string(v.osc)
	if strings.HasPrefix(s, "0;") || strings.HasPrefix(s, "2;") {
		v.title = s[2:]
	}
	v.osc = v.osc[:0]
	v.state = vtGround
}

func (v *vt) ground(b byte) {
	switch b {
	case 0x1b:
		v.state = vtEsc
	case '\r':
		v.cx, v.wrapNext = 0, false
	case '\n', 0x0b, 0x0c:
		v.lineFeed()
	case '\b':
		if v.cx > 0 {
			v.cx--
		}
		v.wrapNext = false
	case '\t':
		v.cx = min(((v.cx/8)+1)*8, v.cols-1)
		v.wrapNext = false
	default:
		if b >= 0x20 && b < 0x7f {
			v.put(rune(b))
		}
	}
}

func (v *vt) escape(b byte) {
	v.state = vtGround
	switch b {
	case '[':
		v.state = vtCSI
		v.params, v.inter = v.params[:0], v.inter[:0]
	case ']':
		v.state = vtOSC
		v.osc = v.osc[:0]
	case 'P', 'X', '^', '_':
		v.state = vtStr
	case '(', ')', '*', '+', '#', '%':
		v.state = vtEscInter
	case '7':
		v.savedX, v.savedY = v.cx, v.cy
	case '8':
		v.cx, v.cy, v.wrapNext = v.savedX, v.savedY, false
	case 'M':
		v.reverseIndex()
	case 'D':
		v.lineFeed()
	case 'E':
		v.cx = 0
		v.lineFeed()
	case 'c':
		v.reset()
	}
}

func (v *vt) reset() {
	v.main, v.alt = blankGrid(v.cols, v.rows), blankGrid(v.cols, v.rows)
	v.cells, v.onAlt = v.main, false
	v.cx, v.cy, v.top, v.bot, v.wrapNext = 0, 0, 0, v.rows-1, false
}

func (v *vt) csiByte(b byte) {
	switch {
	case b >= 0x30 && b <= 0x3f:
		v.params = append(v.params, b)
	case b >= 0x20 && b <= 0x2f:
		v.inter = append(v.inter, b)
	case b >= 0x40 && b <= 0x7e:
		v.state = vtGround
		v.csi(b)
	default:
		v.state = vtGround // secuencia inválida
	}
}

func (v *vt) args() (prefix byte, nums []int) {
	s := string(v.params)
	if s != "" && (s[0] == '?' || s[0] == '>' || s[0] == '=' || s[0] == '<') {
		prefix, s = s[0], s[1:]
	}
	for _, f := range strings.FieldsFunc(s, func(r rune) bool { return r == ';' || r == ':' }) {
		n, _ := strconv.Atoi(f)
		nums = append(nums, n)
	}
	if strings.Contains(s, ";;") || strings.HasPrefix(s, ";") {
		// parámetros vacíos intermedios cuentan como 0
		nums = nil
		for _, f := range strings.Split(s, ";") {
			n, _ := strconv.Atoi(f)
			nums = append(nums, n)
		}
	}
	return
}

func argOr(nums []int, i, def int) int {
	if i < len(nums) && nums[i] > 0 {
		return nums[i]
	}
	return def
}

func (v *vt) csi(final byte) {
	prefix, nums := v.args()
	n := argOr(nums, 0, 1)
	switch final {
	case 'A':
		v.moveY(-n)
	case 'B', 'e':
		v.moveY(n)
	case 'C', 'a':
		v.cx, v.wrapNext = min(v.cx+n, v.cols-1), false
	case 'D':
		v.cx, v.wrapNext = max(v.cx-n, 0), false
	case 'E':
		v.cx = 0
		v.moveY(n)
	case 'F':
		v.cx = 0
		v.moveY(-n)
	case 'G', '`':
		v.cx, v.wrapNext = clampInt(n-1, 0, v.cols-1), false
	case 'd':
		v.cy, v.wrapNext = clampInt(n-1, 0, v.rows-1), false
	case 'H', 'f':
		v.cy, v.cx, v.wrapNext = clampInt(argOr(nums, 0, 1)-1, 0, v.rows-1), clampInt(argOr(nums, 1, 1)-1, 0, v.cols-1), false
	case 'J':
		v.eraseDisplay(firstOr(nums, 0))
	case 'K':
		v.eraseLine(firstOr(nums, 0))
	case 'L':
		v.insertLines(n)
	case 'M':
		v.deleteLines(n)
	case '@':
		row := v.cells[v.cy]
		n = min(n, v.cols-v.cx)
		copy(row[v.cx+n:], row[v.cx:v.cols-n])
		for i := v.cx; i < v.cx+n; i++ {
			row[i] = 0
		}
	case 'P':
		row := v.cells[v.cy]
		n = min(n, v.cols-v.cx)
		copy(row[v.cx:], row[v.cx+n:])
		for i := v.cols - n; i < v.cols; i++ {
			row[i] = 0
		}
	case 'X':
		for i := v.cx; i < min(v.cx+n, v.cols); i++ {
			v.cells[v.cy][i] = 0
		}
	case 'S':
		for i := 0; i < n; i++ {
			v.scrollUp()
		}
	case 'T':
		for i := 0; i < n; i++ {
			v.scrollDown()
		}
	case 'r':
		t, b := argOr(nums, 0, 1)-1, argOr(nums, 1, v.rows)-1
		if t >= 0 && b < v.rows && t < b {
			v.top, v.bot = t, b
		} else {
			v.top, v.bot = 0, v.rows-1
		}
		v.cx, v.cy, v.wrapNext = 0, 0, false
	case 's':
		if prefix == 0 {
			v.savedX, v.savedY = v.cx, v.cy
		}
	case 'u':
		v.cx, v.cy, v.wrapNext = v.savedX, v.savedY, false
	case 'h', 'l':
		if prefix == '?' {
			for _, m := range nums {
				v.mode(m, final == 'h')
			}
		}
	case 'n':
		if prefix == 0 && v.reply != nil {
			switch firstOr(nums, 0) {
			case 5:
				v.reply([]byte("\x1b[0n"))
			case 6:
				v.reply([]byte("\x1b[" + strconv.Itoa(v.cy+1) + ";" + strconv.Itoa(v.cx+1) + "R"))
			}
		}
	case 'c':
		if v.reply != nil {
			if prefix == '>' {
				v.reply([]byte("\x1b[>0;0;0c"))
			} else if prefix == 0 {
				v.reply([]byte("\x1b[?62;c"))
			}
		}
	case 't':
		if v.reply != nil && firstOr(nums, 0) == 18 {
			v.reply([]byte("\x1b[8;" + strconv.Itoa(v.rows) + ";" + strconv.Itoa(v.cols) + "t"))
		}
	}
}

func firstOr(nums []int, def int) int {
	if len(nums) == 0 {
		return def
	}
	return nums[0]
}

func clampInt(x, lo, hi int) int { return max(lo, min(x, hi)) }

func (v *vt) mode(m int, on bool) {
	switch m {
	case 25:
		v.hideCursor = !on
	case 47, 1047, 1049:
		if on == v.onAlt {
			return
		}
		if on {
			if m == 1049 {
				v.savedX, v.savedY = v.cx, v.cy
			}
			v.alt = blankGrid(v.cols, v.rows)
			v.cells, v.onAlt = v.alt, true
			v.top, v.bot = 0, v.rows-1
		} else {
			v.cells, v.onAlt = v.main, false
			v.top, v.bot = 0, v.rows-1
			if m == 1049 {
				v.cx, v.cy = v.savedX, v.savedY
			}
		}
		v.wrapNext = false
	}
}

func (v *vt) moveY(d int) {
	v.wrapNext = false
	if d < 0 {
		lim := 0
		if v.cy >= v.top {
			lim = v.top
		}
		v.cy = max(v.cy+d, lim)
		return
	}
	lim := v.rows - 1
	if v.cy <= v.bot {
		lim = v.bot
	}
	v.cy = min(v.cy+d, lim)
}

func (v *vt) put(r rune) {
	w := runewidth.RuneWidth(r)
	if w == 0 {
		return // combinantes: se ignoran
	}
	if v.wrapNext || v.cx+w > v.cols {
		v.cx = 0
		v.lineFeed()
	}
	v.wrapNext = false
	row := v.cells[v.cy]
	row[v.cx] = r
	if w == 2 && v.cx+1 < v.cols {
		row[v.cx+1] = vtCont
	}
	v.cx += w
	if v.cx >= v.cols {
		v.cx = v.cols - 1
		v.wrapNext = true
	}
}

func (v *vt) lineFeed() {
	v.wrapNext = false
	if v.cy == v.bot {
		v.scrollUp()
	} else if v.cy < v.rows-1 {
		v.cy++
	}
}

func (v *vt) reverseIndex() {
	v.wrapNext = false
	if v.cy == v.top {
		v.scrollDown()
	} else if v.cy > 0 {
		v.cy--
	}
}

func (v *vt) scrollUp() {
	if !v.onAlt && v.top == 0 {
		v.history = append(v.history, rowText(v.cells[0]))
		if len(v.history) > vtMaxScrollback {
			v.history = v.history[len(v.history)-vtMaxScrollback:]
		}
	}
	copy(v.cells[v.top:v.bot], v.cells[v.top+1:v.bot+1])
	v.cells[v.bot] = make([]rune, v.cols)
}

func (v *vt) scrollDown() {
	copy(v.cells[v.top+1:v.bot+1], v.cells[v.top:v.bot])
	v.cells[v.top] = make([]rune, v.cols)
}

func (v *vt) insertLines(n int) {
	if v.cy < v.top || v.cy > v.bot {
		return
	}
	for i := 0; i < n; i++ {
		copy(v.cells[v.cy+1:v.bot+1], v.cells[v.cy:v.bot])
		v.cells[v.cy] = make([]rune, v.cols)
	}
}

func (v *vt) deleteLines(n int) {
	if v.cy < v.top || v.cy > v.bot {
		return
	}
	for i := 0; i < n; i++ {
		copy(v.cells[v.cy:v.bot], v.cells[v.cy+1:v.bot+1])
		v.cells[v.bot] = make([]rune, v.cols)
	}
}

func (v *vt) eraseDisplay(mode int) {
	switch mode {
	case 0:
		v.eraseLine(0)
		for y := v.cy + 1; y < v.rows; y++ {
			v.cells[y] = make([]rune, v.cols)
		}
	case 1:
		for y := 0; y < v.cy; y++ {
			v.cells[y] = make([]rune, v.cols)
		}
		v.eraseLine(1)
	case 2:
		for y := 0; y < v.rows; y++ {
			v.cells[y] = make([]rune, v.cols)
		}
	case 3:
		v.history = nil
	}
}

func (v *vt) eraseLine(mode int) {
	row := v.cells[v.cy]
	switch mode {
	case 0:
		for i := v.cx; i < v.cols; i++ {
			row[i] = 0
		}
	case 1:
		for i := 0; i <= v.cx && i < v.cols; i++ {
			row[i] = 0
		}
	case 2:
		for i := range row {
			row[i] = 0
		}
	}
}

func rowText(row []rune) string {
	var b strings.Builder
	for _, r := range row {
		switch {
		case r == vtCont:
		case r == 0:
			b.WriteByte(' ')
		default:
			b.WriteRune(r)
		}
	}
	return strings.TrimRight(b.String(), " ")
}

// Screen devuelve las líneas visibles (sin espacios finales ni líneas vacías
// al final, salvo hasta la fila del cursor) y la posición del cursor (1-based).
func (v *vt) Screen() (lines []string, row, col int, alt, cursorHidden bool) {
	v.mu.Lock()
	defer v.mu.Unlock()
	lines = make([]string, v.rows)
	for y := range lines {
		lines[y] = rowText(v.cells[y])
	}
	end := len(lines)
	for end > 0 && lines[end-1] == "" && end-1 > v.cy {
		end--
	}
	return lines[:end], v.cy + 1, v.cx + 1, v.onAlt, v.hideCursor
}

// History devuelve las últimas n líneas que salieron de la pantalla.
func (v *vt) History(n int) []string {
	v.mu.Lock()
	defer v.mu.Unlock()
	h := v.history
	if n > 0 && len(h) > n {
		h = h[len(h)-n:]
	}
	return append([]string(nil), h...)
}

// Text devuelve historial + pantalla como texto (para buscar con «until»).
func (v *vt) Text() string {
	lines, _, _, _, _ := v.Screen()
	v.mu.Lock()
	h := append([]string(nil), v.history...)
	v.mu.Unlock()
	if len(h) > 200 {
		h = h[len(h)-200:]
	}
	return strings.Join(append(h, lines...), "\n")
}
