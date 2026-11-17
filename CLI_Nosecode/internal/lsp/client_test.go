package lsp

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const fakeLSP = `
import sys, json
def read():
    n = 0
    while True:
        line = sys.stdin.buffer.readline()
        if not line: return None
        line = line.decode().strip()
        if line == "": break
        if line.lower().startswith("content-length"): n = int(line.split(":")[1])
    return json.loads(sys.stdin.buffer.read(n))
def send(o):
    b = json.dumps(o).encode()
    sys.stdout.buffer.write(b"Content-Length: %d\r\n\r\n" % len(b) + b); sys.stdout.buffer.flush()
while True:
    m = read()
    if m is None: break
    meth = m.get("method"); i = m.get("id")
    if meth == "initialize":
        # el servidor también pide configuración: el cliente debe contestar sin bloquearse
        send({"jsonrpc":"2.0","id":99,"method":"workspace/configuration","params":{"items":[{}]}})
        send({"jsonrpc":"2.0","id":i,"result":{"capabilities":{}}})
    elif meth == "textDocument/didOpen" or meth == "textDocument/didChange":
        uri = m["params"]["textDocument"]["uri"]
        send({"jsonrpc":"2.0","method":"textDocument/publishDiagnostics","params":{"uri":uri,"diagnostics":[
            {"range":{"start":{"line":2,"character":4}},"severity":1,"message":"undefined: foo","source":"fake"},
            {"range":{"start":{"line":0,"character":0}},"severity":2,"message":"unused"}]}})
    elif meth == "textDocument/definition":
        uri = m["params"]["textDocument"]["uri"]
        send({"jsonrpc":"2.0","id":i,"result":[{"uri":uri,"range":{"start":{"line":9,"character":1}}}]})
    elif meth == "textDocument/references":
        uri = m["params"]["textDocument"]["uri"]
        send({"jsonrpc":"2.0","id":i,"result":[{"uri":uri,"range":{"start":{"line":1,"character":2}}},{"uri":uri,"range":{"start":{"line":5,"character":0}}}]})
    elif meth == "textDocument/hover":
        send({"jsonrpc":"2.0","id":i,"result":{"contents":{"kind":"markdown","value":"func Hola()"}}})
    elif meth == "shutdown":
        send({"jsonrpc":"2.0","id":i,"result":None})
    elif meth == "exit":
        break
`

func TestClienteConServidorFalso(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("sin python3")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "a.go")
	os.WriteFile(file, []byte("package a\n\nfoo()\n"), 0o644)
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	c, err := Start(ctx, dir, []string{py, "-c", fakeLSP})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ds, err := c.Diagnostics(ctx, file, 5*time.Second)
	if err != nil || len(ds) != 2 {
		t.Fatalf("%v %+v", err, ds)
	}
	txt := FormatDiagnostics(file, ds, true, 8)
	if txt != "a.go:3:5 error: undefined: foo" {
		t.Fatalf("%q", txt)
	}
	defs, err := c.Definition(ctx, file, 3, 1)
	if err != nil || len(defs) != 1 || defs[0].Line != 10 || defs[0].Col != 2 || !strings.HasSuffix(defs[0].Path, "a.go") {
		t.Fatalf("%v %+v", err, defs)
	}
	refs, _ := c.References(ctx, file, 3, 1)
	if len(refs) != 2 || refs[1].Line != 6 {
		t.Fatalf("%+v", refs)
	}
	if h, _ := c.Hover(ctx, file, 3, 1); h != "func Hola()" {
		t.Fatalf("%q", h)
	}
}

func TestManagerSinServidorYRaiz(t *testing.T) {
	m := &Manager{Enabled: true, Servers: map[string][]string{".zzz": {"no-existe-lsp-xyz"}}}
	if m.Supported("x.txt") || !m.Supported("a.zzz") || !m.Supported("a.go") {
		t.Fatal("Supported")
	}
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module x"), 0o644)
	os.MkdirAll(filepath.Join(dir, "sub", "deep"), 0o755)
	if got := Root(filepath.Join(dir, "sub", "deep", "f.go")); got != dir {
		t.Fatalf("root=%s", got)
	}
	if _, err := m.ClientFor(context.Background(), filepath.Join(dir, "a.zzz")); err == nil || !strings.Contains(err.Error(), "no encuentro") {
		t.Fatalf("%v", err)
	}
	if notes := m.PostEditNotes([]string{filepath.Join(dir, "a.zzz")}); notes != "" {
		t.Fatal("sin servidor no debe añadir nada")
	}
	m.Enabled = false
	if m.Supported("a.go") {
		t.Fatal("desactivado")
	}
}

func TestPyrightReal(t *testing.T) {
	if _, err := exec.LookPath("pyright-langserver"); err != nil {
		t.Skip("sin pyright-langserver")
	}
	dir := t.TempDir()
	file := filepath.Join(dir, "m.py")
	os.WriteFile(file, []byte("def f(x: int) -> int:\n    return x\n\nf(1)\nundefined_name\n"), 0o644)
	m := &Manager{Enabled: true}
	defer m.CloseAll()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	c, err := m.ClientFor(ctx, file)
	if err != nil {
		t.Fatal(err)
	}
	ds, err := c.Diagnostics(ctx, file, 20*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(FormatDiagnostics(file, ds, true, 10), "undefined_name") {
		t.Fatalf("pyright debía marcar undefined_name: %+v", ds)
	}
	defs, err := c.Definition(ctx, file, 4, 1)
	if err != nil || len(defs) == 0 || defs[0].Line != 1 {
		t.Fatalf("definición de f: %v %+v", err, defs)
	}
}
