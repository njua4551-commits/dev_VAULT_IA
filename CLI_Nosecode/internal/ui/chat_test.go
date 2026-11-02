package ui

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"nosecode/internal/config"

	tea "github.com/charmbracelet/bubbletea"
)

func sseTestServer(pieces []string, gap time.Duration) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer clave" {
			w.WriteHeader(401)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		for _, p := range pieces {
			b, _ := json.Marshal(map[string]any{"choices": []any{map[string]any{"delta": map[string]string{"content": p}}}})
			fmt.Fprintf(w, "data: %s\n\n", b)
			fl.Flush()
			time.Sleep(gap)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
}

func connected(t *testing.T, url, key string) Model {
	t.Helper()
	m := newTestModel(t)
	m.cfg.Providers["vault"] = config.Provider{Type: "openai", BaseURL: url + "/v1", APIKey: key, Model: "m"}
	m.cfg.DefaultProvider = "vault"
	return m
}

// pump ejecuta los comandos de Bubble Tea como lo haria el runtime hasta que no queden.
func pump(t *testing.T, m Model, cmd tea.Cmd) Model {
	t.Helper()
	for i := 0; cmd != nil && i < 500; i++ {
		nm, next := m.Update(cmd())
		m = nm.(Model)
		cmd = next
	}
	return m
}

func TestChatStreamingLlenaLaRespuesta(t *testing.T) {
	srv := sseTestServer([]string{"Hola ", "desde ", "la IA"}, 0)
	defer srv.Close()
	m := connected(t, srv.URL, "clave")
	m.input.SetValue("hola")
	nm, cmd := m.submit()
	m = nm.(Model)
	if !m.run.busy() || len(m.msgs) != 2 {
		t.Fatalf("debia quedar ocupado con 2 mensajes: busy=%v msgs=%d", m.run.busy(), len(m.msgs))
	}
	m = pump(t, m, cmd)
	if m.run.busy() {
		t.Fatal("debia terminar")
	}
	if got := m.msgs[1].Text; got != "Hola desde la IA" {
		t.Fatalf("respuesta = %q", got)
	}
	if m.View() == "" {
		t.Fatal("View vacio")
	}
	// se guardo en la sesion
	idx := m.activeIndex()
	if got := m.sessions[idx].Messages; len(got) != 2 || got[1].Text != "Hola desde la IA" {
		t.Fatalf("sesion = %+v", got)
	}
}

func TestChatClaveIncorrectaMuestraError(t *testing.T) {
	srv := sseTestServer([]string{"x"}, 0)
	defer srv.Close()
	m := connected(t, srv.URL, "mala")
	m.input.SetValue("hola")
	nm, cmd := m.submit()
	m = pump(t, nm.(Model), cmd)
	if !m.toastErr || !strings.Contains(m.msgs[1].Text, "API key incorrecta") {
		t.Fatalf("toast=%q err=%v msg=%q", m.toast, m.toastErr, m.msgs[1].Text)
	}
	if m.run.busy() {
		t.Fatal("no debia quedar ocupado")
	}
}

func TestChatEscCancela(t *testing.T) {
	srv := sseTestServer([]string{"a", "b", "c", "d", "e", "f", "g", "h", "i", "j"}, 200*time.Millisecond)
	defer srv.Close()
	m := connected(t, srv.URL, "clave")
	m.input.SetValue("hola")
	nm, cmd := m.submit()
	m = nm.(Model)

	// recibe el primer trozo
	nm, cmd = m.Update(cmd())
	m = nm.(Model)
	if m.msgs[1].Text != "a" {
		t.Fatalf("primer trozo = %q", m.msgs[1].Text)
	}
	// enter mientras responde no envia otro mensaje
	m.input.SetValue("otro")
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if len(m.msgs) != 2 {
		t.Fatalf("enter ocupado envio un mensaje: %d", len(m.msgs))
	}
	// esc cancela
	nm, _ = m.Update(tea.KeyMsg{Type: tea.KeyEsc})
	m = nm.(Model)
	if m.run.busy() || !strings.Contains(m.toast, "cancelada") {
		t.Fatalf("busy=%v toast=%q", m.run.busy(), m.toast)
	}
	// el comando pendiente devuelve un mensaje viejo: se ignora sin romper nada
	nm, next := m.Update(cmd())
	m = nm.(Model)
	if next != nil || m.msgs[1].Text != "a" {
		t.Fatalf("mensaje viejo no ignorado: next=%v text=%q", next != nil, m.msgs[1].Text)
	}
}

func TestSinProveedorSigueOffline(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("hola")
	nm, cmd := m.submit()
	if cmd != nil || len(nm.(Model).msgs) != 2 {
		t.Fatal("sin proveedor no debe lanzar red")
	}
}

func TestApiMessagesFiltraVaultYPlan(t *testing.T) {
	msgs := []Message{
		{Role: "user", Text: "hola"},
		{Role: "assistant", Text: vaultTag + "up\nlinea"},
		{Role: "assistant", Text: "ok"},
		{Role: "user", Text: "  "},
	}
	got := apiMessages(msgs, true)
	if len(got) != 3 || got[0].Role != "system" || got[1].Content != "hola" || got[2].Content != "ok" {
		t.Fatalf("got = %+v", got)
	}
}

func TestComandosVaultPorBarra(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		if py, err = exec.LookPath("python"); err != nil {
			t.Skip("sin Python")
		}
	}
	_ = py
	dir := t.TempDir()
	_ = os.MkdirAll(filepath.Join(dir, "client"), 0o755)
	script := filepath.Join(dir, "client", "vault.py")
	_ = os.WriteFile(script, []byte("import sys\nprint('fase '+sys.argv[1], flush=True)\nprint('listo')\n"), 0o644)
	t.Setenv("VAULT_SCRIPT", script)

	m := newTestModel(t)
	m.input.SetValue("/down")
	nm, cmd := m.submit()
	m = nm.(Model)
	if m.run.vault == nil || cmd == nil {
		t.Fatal("debia lanzar vault.py")
	}
	m = pump(t, m, cmd)
	last := m.msgs[len(m.msgs)-1].Text
	if m.run.busy() || !strings.HasPrefix(last, vaultTag+"down") || !strings.Contains(last, "fase down") || !strings.Contains(last, "listo") {
		t.Fatalf("busy=%v texto=%q", m.run.busy(), last)
	}
}

func TestVaultSinScriptDaMensajeClaro(t *testing.T) {
	t.Setenv("VAULT_SCRIPT", filepath.Join(t.TempDir(), "no-existe.py"))
	t.Setenv("HOME", t.TempDir())
	t.Setenv("USERPROFILE", t.TempDir())
	m := newTestModel(t)
	m.input.SetValue("/up")
	nm, cmd := m.submit()
	m = nm.(Model)
	if cmd != nil || !m.toastErr || !strings.Contains(m.msgs[len(m.msgs)-1].Text, "vault.py") {
		t.Fatalf("cmd=%v toast=%q", cmd != nil, m.toast)
	}
}
