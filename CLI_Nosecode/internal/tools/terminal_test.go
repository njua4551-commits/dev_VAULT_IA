//go:build !windows

package tools

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func term(t *testing.T, args map[string]any) Result {
	t.Helper()
	raw, _ := json.Marshal(args)
	return runTerminal(context.Background(), raw)
}

func termCleanup(t *testing.T) {
	t.Cleanup(func() { CloseTerminals(); KillJobs() })
}

func idOf(t *testing.T, r Result) int {
	t.Helper()
	var id int
	if _, err := fmtSscan(r.Text, &id); err != nil {
		t.Fatalf("sin id en %q", r.Text)
	}
	return id
}

func fmtSscan(text string, id *int) (int, error) {
	i := strings.Index(text, "[terminal ")
	if i < 0 {
		return 0, context.Canceled
	}
	rest := text[i+len("[terminal "):]
	n := 0
	for n < len(rest) && rest[n] >= '0' && rest[n] <= '9' {
		*id = *id*10 + int(rest[n]-'0')
		n++
	}
	return n, nil
}

func TestVTBasico(t *testing.T) {
	v := newVT(20, 5, nil)
	v.Write([]byte("hola\r\nmundo\x1b[2;3Hxy\x1b[K"))
	lines, row, col, _, _ := v.Screen()
	if lines[0] != "hola" || lines[1] != "muxy" || row != 2 || col != 5 {
		t.Fatalf("%q %d,%d", lines, row, col)
	}
	// UTF-8 partido en dos escrituras y carácter ancho
	v = newVT(10, 3, nil)
	v.Write([]byte{0xc3})
	v.Write([]byte{0xb1, 'a', 0xe4, 0xb8})
	v.Write([]byte{0xad, 'b'})
	if l, _, _, _, _ := v.Screen(); l[0] != "ña中b" {
		t.Fatalf("%q", l[0])
	}
}

func TestVTScrollYHistorial(t *testing.T) {
	v := newVT(10, 3, nil)
	v.Write([]byte("1\r\n2\r\n3\r\n4\r\n5"))
	lines, _, _, _, _ := v.Screen()
	if strings.Join(lines, "|") != "3|4|5" {
		t.Fatalf("%q", lines)
	}
	if h := v.History(0); strings.Join(h, "|") != "1|2" {
		t.Fatalf("%q", h)
	}
}

func TestVTPantallaAlternaYRespuestas(t *testing.T) {
	var got []byte
	v := newVT(10, 3, func(b []byte) { got = append(got, b...) })
	v.Write([]byte("principal\x1b[?1049h\x1b[Halterna"))
	if l, _, _, alt, _ := v.Screen(); !alt || l[0] != "alterna" {
		t.Fatalf("%q alt=%v", l, alt)
	}
	v.Write([]byte("\x1b[?1049l\x1b[6n"))
	if l, _, _, alt, _ := v.Screen(); alt || l[0] != "principal" {
		t.Fatalf("%q alt=%v", l, alt)
	}
	if string(got) != "\x1b[1;10R" {
		t.Fatalf("respuesta DSR: %q", got)
	}
}

func TestTerminalPythonInteractivo(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("sin python3")
	}
	termCleanup(t)
	r := term(t, map[string]any{"action": "open", "command": py + " -q -i", "until": ">>>", "wait_ms": 8000})
	if r.Error || !strings.Contains(r.Text, ">>>") {
		t.Fatalf("%+v", r)
	}
	id := idOf(t, r)
	r = term(t, map[string]any{"action": "send", "id": id, "text": "x = 6*7\nprint('r=', x)\n", "until": "r= 42", "wait_ms": 5000})
	if !strings.Contains(r.Text, "r= 42") {
		t.Fatalf("%s", r.Text)
	}
	r = term(t, map[string]any{"action": "send", "id": id, "keys": []string{"Ctrl-D"}, "wait_ms": 5000})
	if !strings.Contains(r.Text, "terminó") {
		t.Fatalf("debía terminar: %s", r.Text)
	}
}

func TestTerminalPreguntaYLista(t *testing.T) {
	termCleanup(t)
	r := term(t, map[string]any{"action": "open", "command": `read -p "¿Continuar? [y/N] " a; echo "resp=$a"; read -p "Nombre: " n; echo "hola $n"`, "until": "Continuar", "wait_ms": 5000})
	if !strings.Contains(r.Text, "¿Continuar? [y/N]") {
		t.Fatalf("%s", r.Text)
	}
	id := idOf(t, r)
	r = term(t, map[string]any{"action": "send", "id": id, "text": "y\n", "until": "Nombre:", "wait_ms": 5000})
	if !strings.Contains(r.Text, "resp=y") {
		t.Fatalf("%s", r.Text)
	}
	r = term(t, map[string]any{"action": "send", "id": id, "text": "Ana", "keys": []string{"Enter"}, "until": "hola Ana", "wait_ms": 5000})
	if !strings.Contains(r.Text, "hola Ana") {
		t.Fatalf("%s", r.Text)
	}
	time.Sleep(500 * time.Millisecond)
	if r = term(t, map[string]any{"action": "read", "id": id}); !strings.Contains(r.Text, "terminó (código 0)") {
		t.Fatalf("%s", r.Text)
	}
	if l := term(t, map[string]any{"action": "list"}); !strings.Contains(l.Text, "terminó") {
		t.Fatal(l.Text)
	}
	if r := term(t, map[string]any{"action": "close", "id": id}); r.Error {
		t.Fatal(r.Text)
	}
	if l := term(t, map[string]any{"action": "list"}); !strings.Contains(l.Text, "No hay") {
		t.Fatal(l.Text)
	}
}

func TestTerminalShellEnTerminalYCerrar(t *testing.T) {
	termCleanup(t)
	r := term(t, map[string]any{"action": "open", "wait_ms": 4000, "until": "$"})
	id := idOf(t, r)
	r = term(t, map[string]any{"action": "send", "id": id, "text": "cd /tmp && echo $((20+22)); sleep 300\n", "until": "42", "wait_ms": 5000})
	if !strings.Contains(r.Text, "42") {
		t.Fatalf("%s", r.Text)
	}
	if LiveCount() < 1 {
		t.Fatal("la terminal debe contar como proceso vivo")
	}
	term(t, map[string]any{"action": "close", "id": id})
	for i := 0; i < 100 && LiveCount() != 0; i++ {
		time.Sleep(50 * time.Millisecond)
	}
	if LiveCount() != 0 {
		t.Fatalf("quedan %d procesos", LiveCount())
	}
}

func TestTerminalNoEscribeContrasenas(t *testing.T) {
	termCleanup(t)
	r := term(t, map[string]any{"action": "open", "command": `printf 'Password: '; sleep 30`, "until": "Password", "wait_ms": 5000})
	id := idOf(t, r)
	r = term(t, map[string]any{"action": "send", "id": id, "text": "secreto\n"})
	if !r.Error || !strings.Contains(r.Text, "contraseña") {
		t.Fatalf("debía negarse: %+v", r)
	}
	if strings.Contains(term(t, map[string]any{"action": "read", "id": id}).Text, "secreto") {
		t.Fatal("no debía escribirse")
	}
}

func TestTerminalTeclasYErrores(t *testing.T) {
	for k, want := range map[string]string{"Enter": "\r", "Ctrl-C": "\x03", "ctrl-d": "\x04", "Alt-x": "\x1bx", "Up": "\x1b[A", "q": "q", "F5": "\x1b[15~"} {
		if got, ok := keyBytes(k); !ok || got != want {
			t.Errorf("%s → %q (%v)", k, got, ok)
		}
	}
	if _, ok := keyBytes("Hiper"); ok {
		t.Error("debía rechazar")
	}
	termCleanup(t)
	if r := term(t, map[string]any{"action": "send", "id": 99, "text": "x"}); !r.Error {
		t.Fatal("id inexistente debía fallar")
	}
}

func TestTerminalHistorialYTamano(t *testing.T) {
	termCleanup(t)
	r := term(t, map[string]any{"action": "open", "command": `seq 1 100; sleep 30`, "rows": 10, "cols": 40, "until": "100", "wait_ms": 5000})
	id := idOf(t, r)
	if !strings.Contains(r.Text, "100") || strings.Contains(r.Text, "\n50\n") || !strings.Contains(r.Text, "history:true") {
		t.Fatalf("%s", r.Text)
	}
	if h := term(t, map[string]any{"action": "read", "id": id, "history": true}); !strings.Contains(h.Text, "\n50\n") {
		t.Fatalf("sin historial: %s", h.Text)
	}
	if r := term(t, map[string]any{"action": "resize", "id": id, "cols": 80, "rows": 20}); !strings.Contains(r.Text, "80x20") {
		t.Fatal(r.Text)
	}
}

func TestTerminalDentroDelSandbox(t *testing.T) {
	if _, err := exec.LookPath("bwrap"); err != nil {
		t.Skip("sin bwrap")
	}
	proj, outside := t.TempDir(), t.TempDir()
	t.Chdir(proj)
	defer SetSandbox(SandboxConfig{})
	ResetSandboxProbe()
	SetSandbox(SandboxConfig{Mode: "on"})
	if _, active, note := SandboxState(); !active {
		t.Skipf("bwrap no puede aislar aquí: %s", note)
	}
	termCleanup(t)
	cmd := `echo dentro > a.txt && echo OK1; echo fuera > ` + outside + `/b.txt; echo FIN`
	r := term(t, map[string]any{"action": "open", "command": cmd, "until": "FIN", "wait_ms": 8000})
	// El mensaje varía con el locale («Read-only file system», «No such file»,
	// «No existe el fichero…»); lo importante es que el proyecto funcione y
	// que el archivo de fuera no se cree.
	if !strings.Contains(r.Text, "OK1") {
		t.Fatalf("el sandbox debía permitir escribir en el proyecto: %s", r.Text)
	}
	if _, err := os.Stat(filepath.Join(outside, "b.txt")); err == nil {
		t.Fatalf("el archivo de fuera no debía crearse: %s", r.Text)
	}
}
