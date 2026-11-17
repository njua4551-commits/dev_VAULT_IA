package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"nosecode/internal/config"
)

// servidor SSE antiguo mínimo: GET abre el flujo, POST /messages responde por el flujo.
func fakeSSEServer(t *testing.T) *httptest.Server {
	// un canal por conexión: si no, un manejador viejo podría robar eventos de una nueva
	var mu sync.Mutex
	sessions := map[string]chan string{}
	n := 0
	mux := http.NewServeMux()
	mux.HandleFunc("/sse", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "solo GET", http.StatusMethodNotAllowed)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		mu.Lock()
		n++
		id := fmt.Sprint(n)
		events := make(chan string, 16)
		sessions[id] = events
		mu.Unlock()
		fmt.Fprintf(w, "event: endpoint\ndata: /messages?s=%s\n\n", id)
		fl.Flush()
		for {
			select {
			case ev := <-events:
				fmt.Fprint(w, ev)
				fl.Flush()
			case <-r.Context().Done():
				return
			}
		}
	})
	mux.HandleFunc("/messages", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		mu.Lock()
		events := sessions[r.URL.Query().Get("s")]
		mu.Unlock()
		var m struct {
			ID     *int64 `json:"id"`
			Method string `json:"method"`
		}
		json.Unmarshal(raw, &m)
		w.WriteHeader(http.StatusAccepted)
		if m.ID == nil {
			return
		}
		var result string
		switch m.Method {
		case "initialize":
			result = `{"protocolVersion":"2024-11-05","capabilities":{"tools":{}},"serverInfo":{"name":"viejo","version":"0.1"}}`
		case "tools/list":
			result = `{"tools":[{"name":"ping","description":"p","inputSchema":{"type":"object"}}]}`
		default:
			result = `{}`
		}
		events <- fmt.Sprintf("event: message\ndata: {\"jsonrpc\":\"2.0\",\"id\":%d,\"result\":%s}\n\n", *m.ID, result)
	})
	return httptest.NewServer(mux)
}

func TestProbeTransporteSSEAntiguoConFallback(t *testing.T) {
	srv := fakeSSEServer(t)
	defer srv.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	// explícito
	res, err := Probe(ctx, "v", config.MCPServer{Type: "remote", Transport: "sse", URL: srv.URL + "/sse"})
	if err != nil || res.Info != "viejo 0.1" || len(res.Tools) != 1 {
		t.Fatalf("%v %+v", err, res)
	}
	// automático: el POST al /sse da 405 → cae a SSE
	res, err = Probe(ctx, "v", config.MCPServer{Type: "remote", URL: srv.URL + "/sse"})
	if err != nil || !strings.Contains(res.Info, "viejo") {
		t.Fatalf("fallback: %v %+v", err, res)
	}
}
