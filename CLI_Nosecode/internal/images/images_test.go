package images

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

func TestPrepareReduceYConvierteAJPEG(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 2000, 1000))
	for y := 0; y < 1000; y++ {
		for x := 0; x < 2000; x++ {
			src.SetRGBA(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 80, A: 255})
		}
	}
	var raw bytes.Buffer
	if err := png.Encode(&raw, src); err != nil {
		t.Fatal(err)
	}
	mime, data, err := Prepare(raw.Bytes(), "image/png")
	if err != nil {
		t.Fatal(err)
	}
	if mime != "image/jpeg" {
		t.Fatalf("mime = %q", mime)
	}
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	if b := img.Bounds(); b.Dx() != MaxDimension || b.Dy() != MaxDimension/2 {
		t.Fatalf("tamano = %dx%d", b.Dx(), b.Dy())
	}
}

func TestPrepareImagenInvalida(t *testing.T) {
	if _, _, err := Prepare([]byte("no soy una imagen"), "image/png"); err == nil {
		t.Fatal("esperaba error")
	}
}
