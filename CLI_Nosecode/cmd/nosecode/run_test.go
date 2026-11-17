package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"nosecode/internal/config"
)

// fakeModel responde primero con una llamada a write_file y luego con texto.
func fakeModel(t *testing.T, target string) *httptest.Server {
	turn := 0
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		turn++
		if turn == 1 {
			args, _ := json.Marshal(map[string]string{"path": target, "content": "hola\n"})
			a, _ := json.Marshal(string(args))
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"write_file\",\"arguments\":%s}}]}}]}\n\n", a)
		} else {
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"listo: archivo creado\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
}

func testCfg(url string) *config.Config {
	t := config.Defaults()
	p := t.Providers["vault"]
	p.BaseURL, p.APIKey, p.Model = url, "k", "m"
	t.Providers["vault"] = p
	return t
}

func TestRunConYesEscribeYResponde(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	srv := fakeModel(t, target)
	defer srv.Close()
	var out, errb bytes.Buffer
	code := runCommand(testCfg(srv.URL), []string{"--yes", "--cwd", dir, "crea out.txt"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	data, err := os.ReadFile(target)
	if err != nil || string(data) != "hola\n" {
		t.Fatalf("no se escribió: %v %q", err, data)
	}
	if !strings.Contains(out.String(), "listo: archivo creado") {
		t.Fatalf("salida: %q", out.String())
	}
}

func TestRunSinYesDeniegaEdicionYJSON(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	srv := fakeModel(t, target)
	defer srv.Close()
	var out, errb bytes.Buffer
	code := runCommand(testCfg(srv.URL), []string{"--json", "--cwd", dir, "crea out.txt"}, strings.NewReader(""), &out, &errb)
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if _, err := os.Stat(target); err == nil {
		t.Fatal("sin --yes no debía escribir")
	}
	s := out.String()
	if !strings.Contains(s, `"Denegado: editar necesita aprobación`) || !strings.Contains(s, `"type":"final"`) {
		t.Fatalf("json: %s", s)
	}
}

func TestRunReadOnlyYUso(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "out.txt")
	srv := fakeModel(t, target)
	defer srv.Close()
	var out, errb bytes.Buffer
	runCommand(testCfg(srv.URL), []string{"--yes", "--read-only", "--cwd", dir, "x"}, strings.NewReader(""), &out, &errb)
	if _, err := os.Stat(target); err == nil {
		t.Fatal("--read-only no debe escribir ni con --yes")
	}
	if code := runCommand(testCfg(srv.URL), nil, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("sin tarea debía salir 2, salió %d", code)
	}
	if code := runCommand(testCfg(srv.URL), []string{"--raro", "x"}, strings.NewReader(""), &out, &errb); code != 2 {
		t.Fatalf("opción desconocida debía salir 2, salió %d", code)
	}
}

func TestRunSubagenteTask(t *testing.T) {
	turn := 0
	var bodies []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		bodies = append(bodies, string(raw))
		w.Header().Set("Content-Type", "text/event-stream")
		turn++
		switch turn {
		case 1:
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"t1\",\"function\":{\"name\":\"task\",\"arguments\":\"{\\\"prompt\\\":\\\"busca main\\\"}\"}}]}}]}\n\n")
		case 2:
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"HALLAZGO: main.go:10\"}}]}\n\n")
		default:
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"resumen final\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	code := runCommand(testCfg(srv.URL), []string{"--read-only", "investiga"}, strings.NewReader(""), &out, &errb)
	if code != 0 || !strings.Contains(out.String(), "resumen final") || turn != 3 {
		t.Fatalf("code=%d turn=%d out=%q err=%s", code, turn, out.String(), errb.String())
	}
	if !strings.Contains(bodies[2], "HALLAZGO: main.go:10") {
		t.Fatal("el padre debía recibir el resumen del subagente como resultado de task")
	}
	if strings.Contains(bodies[1], `"name":"write_file"`) || strings.Contains(bodies[1], `"name":"task"`) {
		t.Fatal("el subagente no debe tener herramientas de escritura ni task")
	}
}

func TestRunEditaYRecibeErroresLSP(t *testing.T) {
	if _, err := exec.LookPath("pyright-langserver"); err != nil {
		t.Skip("sin pyright-langserver")
	}
	dir := t.TempDir()
	target := filepath.Join(dir, "m.py")
	turn := 0
	var second string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		w.Header().Set("Content-Type", "text/event-stream")
		turn++
		if turn == 1 {
			args, _ := json.Marshal(map[string]string{"path": target, "content": "x: int = 1\nnombre_que_no_existe\n"})
			a, _ := json.Marshal(string(args))
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":{\"tool_calls\":[{\"index\":0,\"id\":\"c1\",\"function\":{\"name\":\"write_file\",\"arguments\":%s}}]}}]}\n\n", a)
		} else {
			second = string(raw)
			fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"ok\"}}]}\n\n")
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	var out, errb bytes.Buffer
	code := runCommand(testCfg(srv.URL), []string{"--yes", "--cwd", dir, "crea m.py"}, strings.NewReader(""), &out, &errb)
	if code != 0 || !strings.Contains(second, "LSP: errores tras editar m.py") || !strings.Contains(second, "nombre_que_no_existe") {
		t.Fatalf("code=%d el modelo debía recibir el error LSP tras editar: %s", code, second)
	}
}
