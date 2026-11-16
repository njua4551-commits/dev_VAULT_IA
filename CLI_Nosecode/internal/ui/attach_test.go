package ui

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nosecode/internal/api"
)

func writePNG(t *testing.T, dir, name string) string {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 32))
	for y := 0; y < 32; y++ {
		for x := 0; x < 64; x++ {
			img.Set(x, y, color.RGBA{uint8(x * 4), uint8(y * 8), 120, 255})
		}
	}
	var b bytes.Buffer
	png.Encode(&b, img)
	p := filepath.Join(dir, name)
	if err := os.WriteFile(p, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSplitShellishYExtraerRutas(t *testing.T) {
	d := t.TempDir()
	t.Setenv("NOSECODE_HOME", d)
	p := writePNG(t, d, "mi captura.png")
	clean, paths := extractImagePaths(`qué ves aquí? '` + p + `' y esto`)
	if len(paths) != 1 || paths[0] != p || clean != "qué ves aquí? y esto" {
		t.Fatalf("clean=%q paths=%v", clean, paths)
	}
	esc := strings.ReplaceAll(p, " ", `\ `)
	if _, paths = extractImagePaths(esc); len(paths) != 1 {
		t.Fatalf("con \\espacio: %v", paths)
	}
	if _, paths = extractImagePaths("file://" + strings.ReplaceAll(p, " ", "%20")); len(paths) != 1 {
		t.Fatalf("file://: %v", paths)
	}
	if _, paths = extractImagePaths("/no/existe.png hola"); len(paths) != 0 {
		t.Fatal("no debe adjuntar archivos inexistentes")
	}
}

func TestAdjuntarEnviarYMiniatura(t *testing.T) {
	d := t.TempDir()
	t.Setenv("NOSECODE_HOME", d)
	src := writePNG(t, d, "a.png")
	m := newTestModel(t)
	m, err := m.attachFile(src)
	if err != nil || len(m.attachments) != 1 {
		t.Fatalf("attach: %v %v", err, m.attachments)
	}
	stored := m.attachments[0]
	if !strings.HasPrefix(stored, chatImagesDir()) {
		t.Fatalf("ruta: %s", stored)
	}
	// historial: las 2 últimas imágenes viajan; la tercera no
	msgs := []Message{
		{Role: "user", Text: "uno", Images: []string{stored}},
		{Role: "assistant", Text: "ok"},
		{Role: "user", Text: "dos", Images: []string{stored}},
		{Role: "assistant", Text: "ok"},
		{Role: "user", Text: "tres", Images: []string{stored}},
	}
	out := apiMessages(msgs, false)
	isParts := func(c api.ChatMessage) bool { _, ok := c.Content.([]api.ContentPart); return ok }
	if isParts(out[0]) || !isParts(out[2]) || !isParts(out[4]) {
		t.Fatalf("presupuesto de imágenes: %v %v %v", isParts(out[0]), isParts(out[2]), isParts(out[4]))
	}
	parts := out[4].Content.([]api.ContentPart)
	if len(parts) != 2 || parts[1].ImageURL == nil || !strings.HasPrefix(parts[1].ImageURL.URL, "data:image/jpeg;base64,") {
		t.Fatalf("partes: %+v", parts)
	}
	// miniatura y visor
	rows, w, ok := thumb(stored, thumbCols, thumbRows)
	if !ok || w == 0 || len(rows) == 0 || len(rows) > thumbRows {
		t.Fatalf("thumb: %d filas, w=%d", len(rows), w)
	}
	m.msgs = msgs
	m = m.openImageView(m.lastChatImages(), 0)
	if m.screen != screenImage || len(m.imageScreen()) == 0 {
		t.Fatal("visor")
	}
	if out := m.View(); out == "" {
		t.Fatal("vista vacía")
	}
}

func TestLimiteDeAdjuntos(t *testing.T) {
	d := t.TempDir()
	t.Setenv("NOSECODE_HOME", d)
	m := newTestModel(t)
	for i := 0; i < maxAttachments; i++ {
		var err error
		if m, err = m.attachFile(writePNG(t, d, string(rune('a'+i))+".png")); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := m.attachFile(writePNG(t, d, "z.png")); err == nil {
		t.Fatal("debe respetar el máximo")
	}
}

func TestOpenCommand(t *testing.T) {
	if c := openCommand("linux", "/a/b.jpg"); c[0] != "xdg-open" || c[1] != "/a/b.jpg" {
		t.Fatal(c)
	}
	if c := openCommand("darwin", "/a/b.jpg"); c[0] != "open" {
		t.Fatal(c)
	}
	if c := openCommand("windows", `C:\a b.jpg`); c[0] != "cmd" || c[len(c)-1] != `C:\a b.jpg` {
		t.Fatal(c)
	}
}
