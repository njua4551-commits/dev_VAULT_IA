package debuglog

import (
	"os"
	"strings"
	"testing"
)

func TestLogfEscribeEnHome(t *testing.T) {
	t.Setenv("NOSECODE_HOME", t.TempDir())
	Logf("prueba %d", 7)
	data, err := os.ReadFile(Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "prueba 7") {
		t.Fatalf("log = %q", string(data))
	}
}
