package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func args(v any) json.RawMessage {
	b, _ := json.Marshal(v)
	return b
}

func TestReadWriteEditUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nota.txt")
	if got := runWriteFile(context.Background(), args(map[string]any{"path": path, "content": "uno\ndos\ntres\n"})); got.Error {
		t.Fatal(got.Text)
	}
	got := runReadFile(context.Background(), args(map[string]any{"path": path, "start": 2, "end": 3}))
	if got.Error || !strings.Contains(got.Text, "2: dos") || !strings.Contains(got.Text, "3: tres") {
		t.Fatalf("read = %q", got.Text)
	}
	got = runEditFile(context.Background(), args(map[string]any{"path": path, "old": "dos", "new": "DOS"}))
	if got.Error {
		t.Fatal(got.Text)
	}
	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "DOS") {
		t.Fatalf("archivo = %q", data)
	}
	got = runEditFile(context.Background(), args(map[string]any{"path": path, "old": "uno", "new": "1"}))
	if got.Error {
		t.Fatal(got.Text)
	}
	got = runEditFile(context.Background(), args(map[string]any{"path": path, "old": "o", "new": "0"}))
	if !got.Error || !strings.Contains(got.Text, "aparece") {
		t.Fatalf("esperaba fallo por no ser único: %q", got.Text)
	}
}

func TestListDirAndGrep(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	_ = os.WriteFile(filepath.Join(dir, "sub", "a.go"), []byte("package x\n// TODO: algo\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.txt"), []byte("hola TODO\n"), 0o644)

	list := runListDir(context.Background(), args(map[string]any{"path": dir}))
	if list.Error || !strings.Contains(list.Text, "sub") || !strings.Contains(list.Text, "b.txt") {
		t.Fatalf("list = %q", list.Text)
	}
	g := runGrep(context.Background(), args(map[string]any{"pattern": "TODO", "path": dir, "glob": "*.go"}))
	if g.Error || !strings.Contains(g.Text, "a.go:2") || strings.Contains(g.Text, "b.txt") {
		t.Fatalf("grep = %q", g.Text)
	}
}

func TestBashRun(t *testing.T) {
	got := runBash(context.Background(), args(map[string]any{"command": "echo nosecode"}))
	if got.Error || !strings.Contains(got.Text, "nosecode") {
		t.Fatalf("bash = %q", got.Text)
	}
}

func TestRemember(t *testing.T) {
	path := filepath.Join(t.TempDir(), "NOSECODE.md")
	t.Setenv("NOSECODE_MEMORY", path)
	if got := runRemember(context.Background(), args(map[string]any{"text": "usar 131072 por defecto"})); got.Error {
		t.Fatal(got.Text)
	}
	if got := runRemember(context.Background(), args(map[string]any{"text": "probar /compact"})); got.Error {
		t.Fatal(got.Text)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	text := string(data)
	if !strings.Contains(text, "# Memoria de NOSECODE") ||
		!strings.Contains(text, "- usar 131072 por defecto") ||
		!strings.Contains(text, "- probar /compact") {
		t.Fatalf("memoria = %q", text)
	}
}

func TestWebFetchHTML(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(`<html><head><style>oculto</style></head><body><h1>Hola</h1><script>malo()</script><p>Mundo &amp; compañía</p></body></html>`))
	}))
	defer srv.Close()
	got := runWebFetch(context.Background(), args(map[string]any{"url": srv.URL}))
	if got.Error {
		t.Fatal(got.Text)
	}
	if !strings.Contains(got.Text, "Hola") || !strings.Contains(got.Text, "Mundo & compañía") ||
		strings.Contains(got.Text, "malo") || strings.Contains(got.Text, "oculto") {
		t.Fatalf("web = %q", got.Text)
	}
}

func TestViewImage(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 3, 2))
	img.Set(0, 0, color.RGBA{R: 255, A: 255})
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "x.png")
	if err := os.WriteFile(path, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runViewImage(context.Background(), args(map[string]any{"path": path}))
	if got.Error || len(got.Images) != 1 || got.Images[0].MIME != "image/png" {
		t.Fatalf("view = %+v", got)
	}
	if _, err := base64.StdEncoding.DecodeString(got.Images[0].Data); err != nil {
		t.Fatal(err)
	}
}
