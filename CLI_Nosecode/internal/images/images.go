// Package images prepara capturas para enviarlas al modelo: reduce el tamaño,
// las aplana sobre blanco y las convierte a JPEG para gastar menos contexto.
package images

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	_ "image/png"
	"math"
)

// MaxDimension es el lado máximo de una imagen enviada al modelo.
const MaxDimension = 1280

// Prepare decodifica una imagen, la reduce si hace falta y devuelve JPEG.
func Prepare(data []byte, mime string) (string, []byte, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return "", nil, fmt.Errorf("formato de imagen no soportado (%s): %w", mime, err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if w <= 0 || h <= 0 {
		return "", nil, fmt.Errorf("imagen vacia")
	}
	scale := 1.0
	if w > MaxDimension || h > MaxDimension {
		scale = math.Min(float64(MaxDimension)/float64(w), float64(MaxDimension)/float64(h))
	}
	nw := max(1, int(math.Round(float64(w)*scale)))
	nh := max(1, int(math.Round(float64(h)*scale)))
	dst := image.NewRGBA(image.Rect(0, 0, nw, nh))
	scaleBilinear(src, dst)

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, dst, &jpeg.Options{Quality: 82}); err != nil {
		return "", nil, err
	}
	return "image/jpeg", buf.Bytes(), nil
}

func scaleBilinear(src image.Image, dst *image.RGBA) {
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	dw, dh := dst.Bounds().Dx(), dst.Bounds().Dy()
	for y := 0; y < dh; y++ {
		fy := (float64(y)+0.5)*float64(sh)/float64(dh) - 0.5
		y0 := int(math.Floor(fy))
		wy := fy - float64(y0)
		if y0 < 0 {
			y0, wy = 0, 0
		}
		y1 := min(y0+1, sh-1)
		for x := 0; x < dw; x++ {
			fx := (float64(x)+0.5)*float64(sw)/float64(dw) - 0.5
			x0 := int(math.Floor(fx))
			wx := fx - float64(x0)
			if x0 < 0 {
				x0, wx = 0, 0
			}
			x1 := min(x0+1, sw-1)

			r00, g00, b00, a00 := src.At(sb.Min.X+x0, sb.Min.Y+y0).RGBA()
			r10, g10, b10, a10 := src.At(sb.Min.X+x1, sb.Min.Y+y0).RGBA()
			r01, g01, b01, a01 := src.At(sb.Min.X+x0, sb.Min.Y+y1).RGBA()
			r11, g11, b11, a11 := src.At(sb.Min.X+x1, sb.Min.Y+y1).RGBA()

			blend := func(c00, c10, c01, c11 uint32) uint32 {
				top := float64(c00)*(1-wx) + float64(c10)*wx
				bottom := float64(c01)*(1-wx) + float64(c11)*wx
				return uint32(top*(1-wy) + bottom*wy)
			}
			r := blend(r00, r10, r01, r11)
			g := blend(g00, g10, g01, g11)
			b := blend(b00, b10, b01, b11)
			a := blend(a00, a10, a01, a11)

			// Los canales vienen premultiplicados; añadir el blanco restante.
			const white = 0xffff
			ir := uint8(min(0xff, (r+(white-a))/0x100))
			ig := uint8(min(0xff, (g+(white-a))/0x100))
			ib := uint8(min(0xff, (b+(white-a))/0x100))
			dst.SetRGBA(x, y, color.RGBA{R: ir, G: ig, B: ib, A: 0xff})
		}
	}
}
