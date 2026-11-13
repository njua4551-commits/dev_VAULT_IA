package tools

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"net"
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

func TestGrepOmiteSensibles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.txt"), []byte("TOKEN visible\n"), 0o644)
	_ = os.WriteFile(filepath.Join(dir, ".env"), []byte("TOKEN secreto\n"), 0o600)
	g := runGrep(context.Background(), args(map[string]any{"pattern": "TOKEN", "path": dir}))
	if g.Error || !strings.Contains(g.Text, "a.txt") || strings.Contains(g.Text, ".env") {
		t.Fatalf("grep incluyó un archivo sensible: %q", g.Text)
	}
	ctx := WithReadRoot(context.Background(), dir)
	g = runGrep(ctx, args(map[string]any{"pattern": "TOKEN", "path": dir}))
	if g.Error || strings.Contains(g.Text, ".env") {
		t.Fatalf("una raíz normal no debe habilitar .env: %q", g.Text)
	}

	home := t.TempDir()
	t.Setenv("HOME", home)
	ssh := filepath.Join(home, ".ssh")
	if err := os.MkdirAll(ssh, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ssh, "config"), []byte("Host nosecode\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx = WithReadRoot(context.Background(), ssh)
	g = runGrep(ctx, args(map[string]any{"pattern": "Host", "path": ssh}))
	if g.Error || !strings.Contains(g.Text, "Host nosecode") {
		t.Fatalf("la raíz sensible aprobada debería leerse: %q", g.Text)
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
		_, _ = w.Write([]byte(`<html><head><style>oculto</style></head><body>` +
			`<nav>menu-navegacion</nav><header>cabecera-ruido</header>` +
			`<aside>lateral-ruido</aside><form>formulario-ruido</form>` +
			`<svg><text>icono-ruido</text></svg><noscript>sin-js-ruido</noscript>` +
			`<h1>Hola</h1><script>malo()</script><p>Mundo &amp; compañía</p>` +
			`<footer>pie-ruido</footer></body></html>`))
	}))
	defer srv.Close()
	got := runWebFetch(WithWebLocal(context.Background()), args(map[string]any{"url": srv.URL}))
	if got.Error {
		t.Fatal(got.Text)
	}
	if !strings.Contains(got.Text, "Hola") || !strings.Contains(got.Text, "Mundo & compañía") ||
		strings.Contains(got.Text, "malo") || strings.Contains(got.Text, "oculto") {
		t.Fatalf("web = %q", got.Text)
	}
	for _, noise := range []string{"menu-navegacion", "cabecera-ruido", "lateral-ruido",
		"formulario-ruido", "icono-ruido", "sin-js-ruido", "pie-ruido"} {
		if strings.Contains(got.Text, noise) {
			t.Fatalf("no se limpio %q: %q", noise, got.Text)
		}
	}
}

func TestWebFetchBloqueaDestinosLocales(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("no debería llegar"))
	}))
	defer srv.Close()
	targets := []string{
		srv.URL,
		"http://localhost:1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/",
	}
	for _, target := range targets {
		got := runWebFetch(context.Background(), args(map[string]any{"url": target}))
		if !got.Error || !strings.Contains(got.Text, "bloque") {
			t.Fatalf("%s no se bloqueó: %+v", target, got)
		}
	}
	got := runWebFetch(WithWebLocal(context.Background()), args(map[string]any{"url": srv.URL}))
	if got.Error || !strings.Contains(got.Text, "no debería llegar") {
		t.Fatalf("webfetch_local debería permitir el servidor de prueba: %+v", got)
	}
}

func TestWebFetchRedireccionPrivadaBloqueada(t *testing.T) {
	client := webHTTPClient(context.Background())
	public, _ := http.NewRequest(http.MethodGet, "https://example.com/", nil)
	private, _ := http.NewRequest(http.MethodGet, "http://192.168.1.10/secreto", nil)
	err := client.CheckRedirect(private, []*http.Request{public})
	if err == nil || !strings.Contains(err.Error(), "bloque") {
		t.Fatalf("redirección privada = %v", err)
	}
	via := make([]*http.Request, webMaxRedirects)
	if err := client.CheckRedirect(private, via); err == nil || !strings.Contains(err.Error(), "redirecciones") {
		t.Fatalf("límite de redirecciones = %v", err)
	}
	open := webHTTPClient(WithWebLocal(context.Background()))
	if err := open.CheckRedirect(private, []*http.Request{public}); err != nil {
		t.Fatalf("webfetch_local debería permitir la redirección: %v", err)
	}
}

func TestCheckWebIP(t *testing.T) {
	blocked := []string{
		"127.0.0.1", "10.1.2.3", "172.16.0.1", "172.31.255.255",
		"192.168.1.1", "169.254.169.254", "0.0.0.0", "100.64.0.1",
		"255.255.255.255", "::1", "fc00::1", "fe80::1", "::ffff:127.0.0.1",
	}
	for _, raw := range blocked {
		if err := checkWebIP(net.ParseIP(raw)); err == nil {
			t.Fatalf("%s debería estar bloqueada", raw)
		}
	}
	allowed := []string{"8.8.8.8", "1.1.1.1", "172.32.0.1", "2606:4700:4700::1111"}
	for _, raw := range allowed {
		if err := checkWebIP(net.ParseIP(raw)); err != nil {
			t.Fatalf("%s debería permitirse: %v", raw, err)
		}
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
