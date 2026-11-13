package ui

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"nosecode/internal/api"
)

func TestRedactSecrets(t *testing.T) {
	cmd := `TOKEN=abc123 curl --password hunter2 ` +
		`-H "Authorization: Bearer eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.firma" ` +
		`https://usuario:clave-secreta@example.com/api?api_key=xyz ` +
		`&& echo ghp_abcdefghijklmnopqrstuvwxyz0123456789 AKIAIOSFODNN7EXAMPLE`
	got := redactSecrets(cmd)
	for _, secret := range []string{
		"abc123", "hunter2", "eyJhbGciOiJIUzI1NiJ9", "clave-secreta",
		"api_key=xyz", "ghp_abcdefghijklmnopqrstuvwxyz0123456789", "AKIAIOSFODNN7EXAMPLE",
	} {
		if strings.Contains(got, secret) {
			t.Fatalf("no se redactó %q: %s", secret, got)
		}
	}
	for _, keep := range []string{"curl", "example.com", "echo", "Bearer ***", "TOKEN=***"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("se perdió %q: %s", keep, got)
		}
	}
}

func TestBashCommandYConfirmacion(t *testing.T) {
	var call api.ToolCall
	call.Function.Name = "bash"
	call.Function.Arguments = `{"command":"echo \"hola mundo\" && ls -la"}`
	cmd, ok := bashCommand(call)
	if !ok || cmd != `echo "hola mundo" && ls -la` {
		t.Fatalf("comando = %q, ok=%v", cmd, ok)
	}
	m := newTestModel(t)
	m.width, m.height = 100, 40
	m.tool.calls = []api.ToolCall{call}
	var lines []string
	for _, ln := range m.toolScreen() {
		lines = append(lines, ln.text)
	}
	screen := strings.Join(lines, "\n")
	if !strings.Contains(screen, "Comando") || !strings.Contains(screen, `echo "hola mundo" && ls -la`) {
		t.Fatalf("el modal no muestra el comando completo:\n%s", screen)
	}
}

func TestLogBashCallsRedacta(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("NOSECODE_HOME", dir)
	var call api.ToolCall
	call.Function.Name = "bash"
	call.Function.Arguments = `{"command":"TOKEN=secreto123 echo listo"}`
	logBashCalls("exec", []api.ToolCall{call})
	data, err := os.ReadFile(filepath.Join(dir, "debug.log"))
	if err != nil {
		t.Fatal(err)
	}
	log := string(data)
	if !strings.Contains(log, "bash exec") || !strings.Contains(log, "echo listo") {
		t.Fatalf("no se registró el comando: %q", log)
	}
	if strings.Contains(log, "secreto123") {
		t.Fatalf("el log guardó el secreto: %q", log)
	}
}
