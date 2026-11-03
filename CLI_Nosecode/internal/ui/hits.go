package ui

// Almacen de zonas clicables reconstruido en cada render.

type relHit struct {
	x      int
	w      int
	action string
	arg    string
}

type uiLine struct {
	text string
	hits []relHit

	// Metadatos para seleccion con raton. panel limita la seleccion a una zona
	// (chat, rail...). selectable indica que hay texto copiable entre selStart
	// y selEnd (columnas de pantalla).
	panel      string
	panelX0    int
	panelX1    int
	selectable bool
	selStart   int
	selEnd     int

	// Mensaje al que pertenece la linea (para doble/triple clic).
	hasMsg   bool
	msgIndex int
	msgBody  bool
	msgHead  bool
}

type hitBox struct {
	x, y, w, h int
	action     string
	arg        string
}

type hitStore struct {
	boxes []hitBox
}

func (h *hitStore) reset() {
	h.boxes = h.boxes[:0]
}

func (h *hitStore) Add(x, y, w int, action, arg string) {
	if w <= 0 {
		return
	}
	h.boxes = append(h.boxes, hitBox{x: x, y: y, w: w, h: 1, action: action, arg: arg})
}

func (h *hitStore) At(x, y int) (hitBox, bool) {
	for _, b := range h.boxes {
		if x >= b.x && x < b.x+b.w && y >= b.y && y < b.y+b.h {
			return b, true
		}
	}
	return hitBox{}, false
}

func sameHit(a, b hitBox) bool {
	return a.action == b.action && a.arg == b.arg
}

func (m Model) register(lines []uiLine) {
	m.hit.reset()
	for y, ln := range lines {
		for _, hb := range ln.hits {
			m.hit.Add(hb.x, y, hb.w, hb.action, hb.arg)
		}
	}
}
