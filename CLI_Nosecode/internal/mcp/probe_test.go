package mcp

import (
	"context"
	"os/exec"
	"strings"
	"testing"
	"time"

	"nosecode/internal/config"
)

const fakeServer = `
import sys, json
for line in sys.stdin:
    try: m = json.loads(line)
    except Exception: continue
    if "id" not in m: continue
    if m["method"] == "initialize":
        r = {"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"falso","version":"9.9"}}
    elif m["method"] == "tools/list":
        r = {"tools":[{"name":"uno","description":"d","inputSchema":{"type":"object"}},{"name":"dos"}]}
    else:
        r = {}
    sys.stdout.write(json.dumps({"jsonrpc":"2.0","id":m["id"],"result":r})+"\n"); sys.stdout.flush()
`

func TestProbeServidorFalso(t *testing.T) {
	py, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("sin python3")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := Probe(ctx, "falso", config.MCPServer{Type: "local", Command: []string{py, "-c", fakeServer}})
	if err != nil {
		t.Fatal(err)
	}
	if res.Info != "falso 9.9" || len(res.Tools) != 2 || res.Latency <= 0 {
		t.Fatalf("%+v", res)
	}
}

func TestProbeComandoInexistenteYStderr(t *testing.T) {
	_, err := Probe(context.Background(), "x", config.MCPServer{Type: "local", Command: []string{"no-existe-xyz"}})
	if err == nil || !strings.Contains(err.Error(), "no encuentro el comando") {
		t.Fatalf("%v", err)
	}
	sh, lerr := exec.LookPath("sh")
	if lerr != nil {
		t.Skip()
	}
	_, err = Probe(context.Background(), "x", config.MCPServer{Type: "local", Command: []string{sh, "-c", "echo boom >&2; exit 3"}})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("debía incluir stderr: %v", err)
	}
}
