package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"nosecode/internal/config"
)

// sseTransport es el transporte HTTP+SSE antiguo de MCP (2024-11-05): un GET
// abre un flujo de eventos; el evento «endpoint» indica a qué URL hay que hacer
// POST, y las respuestas llegan como eventos «message» por ese mismo flujo.
type sseTransport struct {
	cfg      config.MCPServer
	client   *http.Client
	cancel   context.CancelFunc
	endpoint string

	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcResponse
	closed  bool
	readErr error
}

func newSSETransport(ctx context.Context, cfg config.MCPServer) (transport, error) {
	streamCtx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	t := &sseTransport{cfg: cfg, client: &http.Client{Timeout: 0}, cancel: cancel, pending: map[int64]chan rpcResponse{}}
	req, err := http.NewRequestWithContext(streamCtx, http.MethodGet, cfg.URL, nil)
	if err != nil {
		cancel()
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	for k, v := range cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		resp.Body.Close()
		cancel()
		return nil, fmt.Errorf("MCP SSE HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	ready := make(chan error, 1)
	go t.readLoop(resp.Body, ready)
	select {
	case err := <-ready:
		if err != nil {
			cancel()
			return nil, err
		}
	case <-ctx.Done():
		cancel()
		return nil, ctx.Err()
	case <-time.After(15 * time.Second):
		cancel()
		return nil, errors.New("el servidor SSE no envió el evento endpoint")
	}
	return t, nil
}

func (t *sseTransport) readLoop(body io.ReadCloser, ready chan<- error) {
	defer body.Close()
	sc := bufio.NewScanner(body)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	event, signaled := "message", false
	var data []string
	flush := func() {
		if len(data) == 0 {
			event = "message"
			return
		}
		payload := strings.Join(data, "\n")
		data = nil
		ev := event
		event = "message"
		switch ev {
		case "endpoint":
			base, err := url.Parse(t.cfg.URL)
			ref, err2 := url.Parse(strings.TrimSpace(payload))
			if err != nil || err2 != nil {
				if !signaled {
					signaled = true
					ready <- errors.New("endpoint SSE no válido: " + payload)
				}
				return
			}
			t.endpoint = base.ResolveReference(ref).String()
			if !signaled {
				signaled = true
				ready <- nil
			}
		default:
			var rpc rpcResponse
			if json.Unmarshal([]byte(payload), &rpc) != nil || len(rpc.ID) == 0 || rpc.Method != "" {
				return
			}
			var id int64
			if json.Unmarshal(rpc.ID, &id) != nil {
				return
			}
			t.mu.Lock()
			ch := t.pending[id]
			delete(t.pending, id)
			t.mu.Unlock()
			if ch != nil {
				ch <- rpc
			}
		}
	}
	for sc.Scan() {
		line := strings.TrimRight(sc.Text(), "\r")
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			data = append(data, strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	err := sc.Err()
	if err == nil {
		err = errors.New("el servidor cerró el flujo SSE")
	}
	t.mu.Lock()
	t.readErr = err
	t.closed = true
	for id, ch := range t.pending {
		close(ch)
		delete(t.pending, id)
	}
	t.mu.Unlock()
	if !signaled {
		ready <- err
	}
}

func (t *sseTransport) post(ctx context.Context, msg map[string]any) error {
	raw, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.endpoint, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return fmt.Errorf("MCP SSE POST %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

func (t *sseTransport) Call(ctx context.Context, method string, params any, out any) error {
	t.mu.Lock()
	if t.closed {
		err := t.readErr
		t.mu.Unlock()
		return err
	}
	t.nextID++
	id := t.nextID
	ch := make(chan rpcResponse, 1)
	t.pending[id] = ch
	t.mu.Unlock()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	if err := t.post(ctx, msg); err != nil {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return err
	}
	select {
	case rpc, ok := <-ch:
		if !ok {
			return errors.New("se cerró el flujo SSE antes de responder")
		}
		if rpc.Error != nil {
			return rpc.Error
		}
		if out != nil && len(rpc.Result) > 0 {
			return json.Unmarshal(rpc.Result, out)
		}
		return nil
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return ctx.Err()
	}
}

func (t *sseTransport) Notify(ctx context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	return t.post(ctx, msg)
}

func (t *sseTransport) Close() error {
	t.cancel()
	return nil
}
