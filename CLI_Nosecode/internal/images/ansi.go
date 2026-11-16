package images

import (
	"bytes"
	"fmt"
	"image"
	"strings"
)

// Thumbnail dibuja una imagen en la terminal con bloques de cuadrante
// (▘▝▀▖▌▞▛▗▚▐▜▄▙▟): cada celda lleva 2×2 subpíxeles con 2 colores a color
// verdadero. Tiene el doble de detalle horizontal que los medios bloques y
// funciona en cualquier terminal moderna. Devuelve una cadena por fila y las
// columnas que ocupa (≤ cols).
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
	// una celda mide 1 de ancho por ~2 de alto: filas = cols·(sh/sw)/2
	w := cols
	h := int(float64(sh)*float64(w)/float64(sw)/2 + 0.5)
	if h < 1 {
		h = 1
	}
	if h > maxRows {
		h = maxRows
		w = int(float64(sw)*float64(h)*2/float64(sh) + 0.5)
		if w < 1 {
			w = 1
		}
	}
	gw, gh := w*2, h*2 // rejilla de subpíxeles
	grid := make([][3]float64, gw*gh)
	for y := 0; y < gh; y++ {
		y0 := b.Min.Y + y*sh/gh
		y1 := b.Min.Y + (y+1)*sh/gh
		if y1 <= y0 {
			y1 = y0 + 1
		}
		for x := 0; x < gw; x++ {
			x0 := b.Min.X + x*sw/gw
			x1 := b.Min.X + (x+1)*sw/gw
			if x1 <= x0 {
				x1 = x0 + 1
			}
			var r, g, bl, n float64
			for yy := y0; yy < y1; yy++ {
				for xx := x0; xx < x1; xx++ {
					cr, cg, cb, _ := src.At(xx, yy).RGBA() // premultiplicado: transparente = negro
					r += float64(cr >> 8)
					g += float64(cg >> 8)
					bl += float64(cb >> 8)
					n++
				}
			}
			grid[y*gw+x] = [3]float64{r / n, g / n, bl / n}
		}
	}

	// máscara: bit0=arriba-izq, bit1=arriba-der, bit2=abajo-izq, bit3=abajo-der
	chars := [...]string{"", "▘", "▝", "▀", "▖", "▌", "▞", "▛", "▗", "▚", "▐", "▜", "▄", "▙", "▟"}
	var rows []string
	for cy := 0; cy < h; cy++ {
		var sb strings.Builder
		lastFg, lastBg := "", ""
		for cx := 0; cx < w; cx++ {
			px := [4][3]float64{
				grid[(cy*2)*gw+cx*2], grid[(cy*2)*gw+cx*2+1],
				grid[(cy*2+1)*gw+cx*2], grid[(cy*2+1)*gw+cx*2+1],
			}
			bestMask, bestErr := 1, -1.0
			var bestFg, bestBg [3]float64
			for mask := 1; mask <= 14; mask++ {
				var fg, bg [3]float64
				nf, nb := 0, 0
				for i := 0; i < 4; i++ {
					if mask&(1<<i) != 0 {
						for c := 0; c < 3; c++ {
							fg[c] += px[i][c]
						}
						nf++
					} else {
						for c := 0; c < 3; c++ {
							bg[c] += px[i][c]
						}
						nb++
					}
				}
				for c := 0; c < 3; c++ {
					fg[c] /= float64(nf)
					bg[c] /= float64(nb)
				}
				e := 0.0
				for i := 0; i < 4; i++ {
					ref := bg
					if mask&(1<<i) != 0 {
						ref = fg
					}
					for c := 0; c < 3; c++ {
						d := px[i][c] - ref[c]
						e += d * d
					}
				}
				if bestErr < 0 || e < bestErr {
					bestErr, bestMask, bestFg, bestBg = e, mask, fg, bg
				}
			}
			fgS := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", int(bestFg[0]+.5), int(bestFg[1]+.5), int(bestFg[2]+.5))
			bgS := fmt.Sprintf("\x1b[48;2;%d;%d;%dm", int(bestBg[0]+.5), int(bestBg[1]+.5), int(bestBg[2]+.5))
			if fgS != lastFg {
				sb.WriteString(fgS)
				lastFg = fgS
			}
			if bgS != lastBg {
				sb.WriteString(bgS)
				lastBg = bgS
			}
			sb.WriteString(chars[bestMask])
		}
		sb.WriteString("\x1b[0m")
		rows = append(rows, sb.String())
	}
	return rows, w, nil
}
