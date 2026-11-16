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
	if err != nil || w != 20 || len(rows) != 5 { // 20x10 px → 5 filas
		t.Fatalf("rows=%d w=%d err=%v", len(rows), w, err)
	}
	if !strings.Contains(rows[0], "\x1b[38;2;255;0;0m") || !strings.Contains(rows[0], "▀") {
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
