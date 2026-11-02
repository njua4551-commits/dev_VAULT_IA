// Package mcp implementa el cliente mínimo del Model Context Protocol:
// servidores locales por stdio y remotos por HTTP (Streamable HTTP).
package mcp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"sort"
	"strings"
	"sync"
	"time"

	"nosecode/internal/config"
)

const protocolVersion = "2024-11-05"

// Tool es una herramienta expuesta por un servidor MCP.
type Tool struct {
	Server      string
	Name        string
	Wire        string
	Description string
	Schema      json.RawMessage
}

// Image es una imagen devuelta por una herramienta.
type Image struct {
	MIME string
	Data string // base64
}

// Result es el resultado de tools/call.
type Result struct {
	Text   string
	Images []Image
	Error  bool
}

// Manager mantiene las conexiones y el catálogo de herramientas.
type Manager struct {
	mu      sync.RWMutex
	conns   []*conn
	tools   []Tool
	byWire  map[string]*conn
	errors  []string
	servers []string
}

type conn struct {
	name string
	cfg  config.MCPServer
	tr   transport
	byID map[string]string // nombre original -> wire
}

type transport interface {
	Call(ctx context.Context, method string, params any, out any) error
	Notify(ctx context.Context, method string, params any) error
	Close() error
}

// ConnectAll arranca en paralelo los servidores habilitados. Los errores no
// abortan el resto: se devuelven para mostrarlos en la interfaz.
func ConnectAll(ctx context.Context, servers map[string]config.MCPServer) (*Manager, []string) {
	m := &Manager{byWire: map[string]*conn{}}
	names := make([]string, 0, len(servers))
	for name, s := range servers {
		if !s.ImportedServerEnabled() {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	type res struct {
		name  string
		conn  *conn
		tools []Tool
		err   error
	}
	ch := make(chan res, len(names))
	for _, name := range names {
		name, cfg := name, servers[name]
		go func() {
			c, tools, err := connectOne(ctx, name, cfg)
			ch <- res{name: name, conn: c, tools: tools, err: err}
		}()
	}

	for range names {
		r := <-ch
		if r.err != nil {
			m.errors = append(m.errors, r.name+": "+r.err.Error())
			continue
		}
		m.conns = append(m.conns, r.conn)
		m.servers = append(m.servers, r.name)
		seen := map[string]bool{}
		for _, t := range m.tools {
			seen[t.Wire] = true
		}
		for _, t := range r.tools {
			wire := t.Wire
			for n := 2; seen[wire]; n++ {
				suffix := fmt.Sprintf("_%d", n)
				wire = truncate(t.Wire, 64-len(suffix)) + suffix
			}
			seen[wire] = true
			t.Wire = wire
			t.Server = r.name
			m.tools = append(m.tools, t)
			m.byWire[wire] = r.conn
			r.conn.byID[t.Name] = wire
		}
	}
	sort.Slice(m.tools, func(i, j int) bool { return m.tools[i].Wire < m.tools[j].Wire })
	return m, m.errors
}

func connectOne(ctx context.Context, name string, cfg config.MCPServer) (*conn, []Tool, error) {
	var (
		tr  transport
		err error
	)
	switch strings.ToLower(cfg.Type) {
	case "remote", "http", "streamable-http":
		if strings.TrimSpace(cfg.URL) == "" {
			return nil, nil, errors.New("falta url")
		}
		tr = newHTTPTransport(cfg)
	case "local", "stdio", "":
		tr, err = newStdioTransport(ctx, cfg)
	default:
		return nil, nil, fmt.Errorf("tipo MCP desconocido: %s", cfg.Type)
	}
	if err != nil {
		return nil, nil, err
	}
	c := &conn{name: name, cfg: cfg, tr: tr, byID: map[string]string{}}
	initCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := c.initialize(initCtx); err != nil {
		tr.Close()
		return nil, nil, err
	}
	listCtx, cancelList := context.WithTimeout(ctx, 15*time.Second)
	defer cancelList()
	tools, err := c.listTools(listCtx)
	if err != nil {
		tr.Close()
		return nil, nil, err
	}
	return c, tools, nil
}

func (c *conn) initialize(ctx context.Context) error {
	params := map[string]any{
		"protocolVersion": protocolVersion,
		"capabilities":    map[string]any{},
		"clientInfo": map[string]string{
			"name":    "nosecode",
			"version": "0.4.0",
		},
	}
	var out json.RawMessage
	if err := c.tr.Call(ctx, "initialize", params, &out); err != nil {
		return fmt.Errorf("initialize: %w", err)
	}
	if err := c.tr.Notify(ctx, "notifications/initialized", map[string]any{}); err != nil {
		return fmt.Errorf("initialized: %w", err)
	}
	return nil
}

func (c *conn) listTools(ctx context.Context) ([]Tool, error) {
	var out []Tool
	cursor := ""
	for pages := 0; pages < 20; pages++ {
		params := map[string]any{}
		if cursor != "" {
			params["cursor"] = cursor
		}
		var page struct {
			Tools []struct {
				Name        string          `json:"name"`
				Description string          `json:"description"`
				InputSchema json.RawMessage `json:"inputSchema"`
			} `json:"tools"`
			NextCursor string `json:"nextCursor"`
		}
		if err := c.tr.Call(ctx, "tools/list", params, &page); err != nil {
			return nil, fmt.Errorf("tools/list: %w", err)
		}
		for _, t := range page.Tools {
			if strings.TrimSpace(t.Name) == "" {
				continue
			}
			schema := t.InputSchema
			if len(schema) == 0 || string(schema) == "null" {
				schema = json.RawMessage(`{"type":"object","properties":{}}`)
			}
			desc := strings.TrimSpace(t.Description)
			if desc == "" {
				desc = "Herramienta MCP " + c.name + "." + t.Name
			}
			out = append(out, Tool{
				Server:      c.name,
				Name:        t.Name,
				Wire:        wireName(c.name, t.Name),
				Description: truncate(desc, 900),
				Schema:      schema,
			})
		}
		if page.NextCursor == "" {
			break
		}
		cursor = page.NextCursor
	}
	if len(out) == 0 {
		return nil, errors.New("el servidor no expone herramientas")
	}
	return out, nil
}

func (c *conn) call(ctx context.Context, wire string, args json.RawMessage) (Result, error) {
	name := ""
	for orig, w := range c.byID {
		if w == wire {
			name = orig
			break
		}
	}
	if name == "" {
		name = wire
	}
	if len(args) == 0 || string(args) == "null" {
		args = json.RawMessage(`{}`)
	}
	var out struct {
		Content []struct {
			Type     string `json:"type"`
			Text     string `json:"text"`
			Data     string `json:"data"`
			MIMEType string `json:"mimeType"`
		} `json:"content"`
		IsError    bool            `json:"isError"`
		Structured json.RawMessage `json:"structuredContent"`
	}
	params := map[string]any{"name": name, "arguments": args}
	if err := c.tr.Call(ctx, "tools/call", params, &out); err != nil {
		return Result{}, err
	}
	res := Result{Error: out.IsError}
	var texts []string
	for _, b := range out.Content {
		switch b.Type {
		case "text":
			if strings.TrimSpace(b.Text) != "" {
				texts = append(texts, b.Text)
			}
		case "image":
			if b.Data != "" {
				mime := b.MIMEType
				if mime == "" {
					mime = "image/png"
				}
				res.Images = append(res.Images, Image{MIME: mime, Data: b.Data})
				texts = append(texts, fmt.Sprintf("[imagen %s recibida]", mime))
			}
		case "resource":
			texts = append(texts, "[recurso MCP recibido]")
		}
	}
	if len(texts) == 0 && len(out.Structured) > 0 && string(out.Structured) != "null" {
		texts = append(texts, string(out.Structured))
	}
	res.Text = strings.TrimSpace(strings.Join(texts, "\n"))
	if res.Text == "" {
		res.Text = "(sin contenido)"
	}
	res.Text = truncate(res.Text, 24000)
	return res, nil
}

// Tools devuelve el catálogo actual.
func (m *Manager) Tools() []Tool {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Tool, len(m.tools))
	copy(out, m.tools)
	return out
}

// Servers devuelve los nombres conectados.
func (m *Manager) Servers() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.servers))
	copy(out, m.servers)
	return out
}

// Errors devuelve los fallos de conexión.
func (m *Manager) Errors() []string {
	if m == nil {
		return nil
	}
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]string, len(m.errors))
	copy(out, m.errors)
	return out
}

// Call ejecuta una herramienta por su nombre wire.
func (m *Manager) Call(ctx context.Context, wire string, args json.RawMessage) (Result, error) {
	if m == nil {
		return Result{}, errors.New("MCP no conectado")
	}
	m.mu.RLock()
	c := m.byWire[wire]
	m.mu.RUnlock()
	if c == nil {
		return Result{}, fmt.Errorf("herramienta MCP desconocida: %s", wire)
	}
	timeout := duration(c.cfg.Timeout, 120*time.Second)
	if timeout > 90*time.Second {
		timeout = 90 * time.Second
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return c.call(cctx, wire, args)
}

// Close termina todos los servidores.
func (m *Manager) Close() {
	if m == nil {
		return
	}
	m.mu.Lock()
	conns := m.conns
	m.conns = nil
	m.tools = nil
	m.byWire = map[string]*conn{}
	m.mu.Unlock()
	for _, c := range conns {
		_ = c.tr.Close()
	}
}

func wireName(server, tool string) string {
	clean := func(s string) string {
		var b strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' || r == '-' {
				b.WriteRune(r)
			} else {
				b.WriteByte('_')
			}
		}
		return strings.Trim(b.String(), "_")
	}
	name := clean(server) + "__" + clean(tool)
	return truncate(name, 64)
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

func duration(ms int, def time.Duration) time.Duration {
	if ms <= 0 {
		return def
	}
	return time.Duration(ms) * time.Millisecond
}

// ---------------------------------------------------------------- stdio ---

type stdioTransport struct {
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	writeMu sync.Mutex
	mu      sync.Mutex
	nextID  int64
	pending map[int64]chan rpcResponse
	closed  chan struct{}
	err     error
	stderr  *tailBuffer
}

type rpcResponse struct {
	ID     json.RawMessage `json:"id"`
	Result json.RawMessage `json:"result"`
	Error  *rpcError       `json:"error"`
	Method string          `json:"method"`
}

type rpcError struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func (e *rpcError) Error() string {
	if e == nil {
		return "error MCP"
	}
	if e.Message == "" {
		return fmt.Sprintf("error MCP %d", e.Code)
	}
	return fmt.Sprintf("MCP %d: %s", e.Code, e.Message)
}

func newStdioTransport(ctx context.Context, cfg config.MCPServer) (transport, error) {
	if len(cfg.Command) == 0 {
		return nil, errors.New("falta command")
	}
	cmd := exec.Command(cfg.Command[0], cfg.Command[1:]...)
	if cfg.Cwd != "" {
		cmd.Dir = cfg.Cwd
	}
	env := os.Environ()
	for k, v := range cfg.Environment {
		env = append(env, k+"="+v)
	}
	cmd.Env = env

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	t := &stdioTransport{
		cmd:     cmd,
		stdin:   stdin,
		pending: map[int64]chan rpcResponse{},
		closed:  make(chan struct{}),
		stderr:  &tailBuffer{max: 4096},
	}
	go io.Copy(t.stderr, stderr)
	go t.readLoop(stdout)
	return t, nil
}

func (t *stdioTransport) readLoop(r io.Reader) {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var msg rpcResponse
		if json.Unmarshal(line, &msg) != nil {
			continue
		}
		if msg.Method != "" && len(msg.ID) == 0 {
			continue // notificación
		}
		var id int64
		if json.Unmarshal(msg.ID, &id) != nil {
			continue
		}
		t.mu.Lock()
		ch := t.pending[id]
		delete(t.pending, id)
		t.mu.Unlock()
		if ch != nil {
			ch <- msg
		}
	}
	t.mu.Lock()
	if t.err == nil {
		t.err = sc.Err()
		if t.err == nil {
			t.err = errors.New("el servidor MCP cerró la salida")
		}
	}
	for id, ch := range t.pending {
		delete(t.pending, id)
		close(ch)
	}
	t.mu.Unlock()
	close(t.closed)
}

func (t *stdioTransport) Call(ctx context.Context, method string, params any, out any) error {
	t.mu.Lock()
	t.nextID++
	id := t.nextID
	ch := make(chan rpcResponse, 1)
	t.pending[id] = ch
	t.mu.Unlock()

	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	raw, _ := json.Marshal(msg)
	t.writeMu.Lock()
	_, err := t.stdin.Write(append(raw, '\n'))
	t.writeMu.Unlock()
	if err != nil {
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return err
	}

	select {
	case <-ctx.Done():
		t.mu.Lock()
		delete(t.pending, id)
		t.mu.Unlock()
		return ctx.Err()
	case <-t.closed:
		return t.closedErr()
	case resp, ok := <-ch:
		if !ok {
			return t.closedErr()
		}
		if resp.Error != nil {
			return resp.Error
		}
		if out != nil && len(resp.Result) > 0 {
			if err := json.Unmarshal(resp.Result, out); err != nil {
				return err
			}
		}
		return nil
	}
}

func (t *stdioTransport) Notify(ctx context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	raw, _ := json.Marshal(msg)
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	_, err := t.stdin.Write(append(raw, '\n'))
	return err
}

func (t *stdioTransport) Close() error {
	t.writeMu.Lock()
	_ = t.stdin.Close()
	t.writeMu.Unlock()
	select {
	case <-t.closed:
	case <-time.After(2 * time.Second):
	}
	if t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
	}
	_ = t.cmd.Wait()
	return nil
}

func (t *stdioTransport) closedErr() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.err != nil {
		return fmt.Errorf("%w: %s", t.err, strings.TrimSpace(t.stderr.String()))
	}
	return errors.New("servidor MCP cerrado")
}

type tailBuffer struct {
	mu  sync.Mutex
	max int
	buf []byte
}

func (b *tailBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

func (b *tailBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

// ----------------------------------------------------------------- http ---

type httpTransport struct {
	cfg     config.MCPServer
	client  *http.Client
	mu      sync.Mutex
	nextID  int64
	session string
}

func newHTTPTransport(cfg config.MCPServer) transport {
	return &httpTransport{
		cfg:    cfg,
		client: &http.Client{Timeout: 0},
	}
}

func (t *httpTransport) Call(ctx context.Context, method string, params any, out any) error {
	t.mu.Lock()
	t.nextID++
	id := t.nextID
	t.mu.Unlock()
	msg := map[string]any{"jsonrpc": "2.0", "id": id, "method": method}
	if params != nil {
		msg["params"] = params
	}
	raw, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if t.session != "" {
		req.Header.Set("Mcp-Session-Id", t.session)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if sid := resp.Header.Get("Mcp-Session-Id"); sid != "" {
		t.session = sid
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 500))
		return fmt.Errorf("MCP HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	if resp.StatusCode == http.StatusAccepted {
		return nil
	}
	ct := resp.Header.Get("Content-Type")
	if strings.Contains(ct, "text/event-stream") {
		return readSSE(resp.Body, id, out)
	}
	var rpc rpcResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpc); err != nil {
		return err
	}
	if rpc.Error != nil {
		return rpc.Error
	}
	if out != nil && len(rpc.Result) > 0 {
		return json.Unmarshal(rpc.Result, out)
	}
	return nil
}

func readSSE(r io.Reader, id int64, out any) error {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "" || payload == "[DONE]" {
			continue
		}
		var rpc rpcResponse
		if json.Unmarshal([]byte(payload), &rpc) != nil {
			continue
		}
		var rid int64
		if json.Unmarshal(rpc.ID, &rid) != nil || rid != id {
			continue
		}
		if rpc.Error != nil {
			return rpc.Error
		}
		if out != nil && len(rpc.Result) > 0 {
			return json.Unmarshal(rpc.Result, out)
		}
		return nil
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return errors.New("respuesta SSE de MCP sin resultado")
}

func (t *httpTransport) Notify(ctx context.Context, method string, params any) error {
	msg := map[string]any{"jsonrpc": "2.0", "method": method}
	if params != nil {
		msg["params"] = params
	}
	raw, _ := json.Marshal(msg)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, t.cfg.URL, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json, text/event-stream")
	req.Header.Set("MCP-Protocol-Version", protocolVersion)
	for k, v := range t.cfg.Headers {
		req.Header.Set(k, v)
	}
	if t.session != "" {
		req.Header.Set("Mcp-Session-Id", t.session)
	}
	resp, err := t.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("MCP HTTP %d", resp.StatusCode)
	}
	return nil
}

func (t *httpTransport) Close() error { return nil }

// DecodeImage convierte una imagen MCP a bytes.
func DecodeImage(img Image) ([]byte, error) {
	return base64.StdEncoding.DecodeString(img.Data)
}
