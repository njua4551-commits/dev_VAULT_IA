package images

import (
	"bytes"
	"fmt"
	"image"
	"strings"
)

// Thumbnail dibuja una imagen en la terminal con medios bloques (▀) a color
// verdadero: cada celda lleva dos píxeles (arriba y abajo). Funciona en
// cualquier terminal moderna sin protocolos especiales. Devuelve una cadena
// por fila y cuántas columnas ocupa (≤ cols).
func Thumbnail(data []byte, cols, maxRows int) ([]string, int, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, 0, fmt.Errorf("no pude leer la imagen: %w", err)
	}
	b := src.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 || cols < 1 || maxRows < 1 {
		return nil, 0, fmt.Errorf("tamaño inválido")
	}
	w := cols
	h := int(float64(sh)*float64(w)/float64(sw) + 0.5)
	if h < 1 {
		h = 1
	}
	if h > maxRows*2 { // limitar por alto manteniendo la proporción
		h = maxRows * 2
		w = int(float64(sw)*float64(h)/float64(sh) + 0.5)
		if w < 1 {
			w = 1
		}
	}
	px := func(x, y int) (uint8, uint8, uint8) {
		// promedio del bloque de la imagen original que cubre este píxel
		x0 := b.Min.X + x*sw/w
		x1 := b.Min.X + (x+1)*sw/w
		y0 := b.Min.Y + y*sh/h
		y1 := b.Min.Y + (y+1)*sh/h
		if x1 <= x0 {
			x1 = x0 + 1
		}
		if y1 <= y0 {
			y1 = y0 + 1
		}
		var r, g, bl, n uint64
		for yy := y0; yy < y1; yy++ {
			for xx := x0; xx < x1; xx++ {
				cr, cg, cb, ca := src.At(xx, yy).RGBA() // 16 bits, premultiplicado
				_ = ca                                  // sobre negro: el premultiplicado ya lo hace
				r += uint64(cr >> 8)
				g += uint64(cg >> 8)
				bl += uint64(cb >> 8)
				n++
			}
		}
		return uint8(r / n), uint8(g / n), uint8(bl / n)
	}
	var rows []string
	for y := 0; y < h; y += 2 {
		var sb strings.Builder
		for x := 0; x < w; x++ {
			tr, tg, tb := px(x, y)
			fmt.Fprintf(&sb, "\x1b[38;2;%d;%d;%dm", tr, tg, tb)
			if y+1 < h {
				br, bg, bb := px(x, y+1)
				fmt.Fprintf(&sb, "\x1b[48;2;%d;%d;%dm", br, bg, bb)
			} else {
				sb.WriteString("\x1b[49m")
			}
			sb.WriteString("▀")
		}
		sb.WriteString("\x1b[0m")
		rows = append(rows, sb.String())
	}
	return rows, w, nil
}
