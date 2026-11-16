package images

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
)

func pngOf(w, h int, c color.Color) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	return b.Bytes()
}

func TestThumbnailTamanoYColor(t *testing.T) {
	rows, w, err := Thumbnail(pngOf(100, 50, color.RGBA{255, 0, 0, 255}), 20, 10)
	if err != nil || w != 20 || len(rows) != 5 { // 20 columnas → 5 filas (celda 1:2)
		t.Fatalf("rows=%d w=%d err=%v", len(rows), w, err)
	}
	if !strings.Contains(rows[0], "255;0;0m") {
		t.Fatalf("color: %q", rows[0])
	}
}

func TestThumbnailLimitaPorAlto(t *testing.T) {
	rows, w, err := Thumbnail(pngOf(10, 200, color.RGBA{0, 0, 255, 255}), 40, 6)
	if err != nil || len(rows) > 6 || w >= 40 {
		t.Fatalf("rows=%d w=%d err=%v", len(rows), w, err)
	}
}

func TestThumbnailInvalida(t *testing.T) {
	if _, _, err := Thumbnail([]byte("no es una imagen"), 10, 5); err == nil {
		t.Fatal("debe fallar")
	}
}

// Mitad izquierda roja, derecha azul: el cuadrante debe separar los dos colores
// dentro de una misma celda (doble detalle horizontal que los medios bloques).
func TestThumbnailCuadrantesSeparanColores(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 2, 2))
	for y := 0; y < 2; y++ {
		img.Set(0, y, color.RGBA{255, 0, 0, 255})
		img.Set(1, y, color.RGBA{0, 0, 255, 255})
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	rows, w, err := Thumbnail(b.Bytes(), 1, 1)
	if err != nil || w != 1 || len(rows) != 1 {
		t.Fatalf("%v %d %d", err, w, len(rows))
	}
	r := rows[0]
	if !(strings.Contains(r, "▌") || strings.Contains(r, "▐")) || !strings.Contains(r, "255;0;0") || !strings.Contains(r, "0;0;255") {
		t.Fatalf("debía usar medio bloque vertical con rojo y azul: %q", r)
	}
}
