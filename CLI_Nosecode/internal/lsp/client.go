// Package lsp es un cliente mínimo del Language Server Protocol: lo justo para
// que el agente vea errores de compilación/tipos tras editar y pueda saltar a
// definiciones, referencias y documentación sin adivinar con grep.
package lsp

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Diagnostic es un aviso del servidor sobre un archivo.
type Diagnostic struct {
	Line     int // 1-based
	Col      int // 1-based
	Severity int // 1 error, 2 warning, 3 info, 4 hint
	Message  string
	Source   string
}

// Location es una posición en un archivo (1-based).
type Location struct {
	Path string
	Line int
	Col  int
}

type rpcMsg struct {
	ID     json.RawMessage `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error,omitempty"`
}

// Client habla con un servidor LSP por stdio.
type Client struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	wmu    sync.Mutex
	root   string
	nextID int64

	mu      sync.Mutex
	pending map[int64]chan rpcMsg
	diags   map[string][]Diagnostic
	diagSeq map[string]int
	diagCh  chan struct{} // se cierra y recrea cuando llega un publishDiagnostics
	opened  map[string]int
	closed  bool
}

// Start arranca el servidor y hace el handshake initialize.
func Start(ctx context.Context, root string, command []string) (*Client, error) {
	if len(command) == 0 {
		return nil, errors.New("comando LSP vacío")
	}
	if _, err := exec.LookPath(command[0]); err != nil {
		return nil, fmt.Errorf("no encuentro %q en el PATH", command[0])
	}
	cmd := exec.Command(command[0], command[1:]...)
	cmd.Dir = root
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &Client{cmd: cmd, stdin: stdin, root: root, pending: map[int64]chan rpcMsg{},
		diags: map[string][]Diagnostic{}, diagSeq: map[string]int{}, diagCh: make(chan struct{}), opened: map[string]int{}}
	go c.readLoop(bufio.NewReader(stdout))
	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	params := map[string]any{
		"processId": os.Getpid(),
		"rootUri":   fileURI(root),
		"capabilities": map[string]any{
			"textDocument": map[string]any{
				"publishDiagnostics": map[string]any{},
				"synchronization":    map[string]any{"didSave": false},
				"definition":         map[string]any{},
				"references":         map[string]any{},
				"hover":              map[string]any{"contentFormat": []string{"plaintext", "markdown"}},
			},
			"workspace": map[string]any{"configuration": true},
		},
		"workspaceFolders": []map[string]any{{"uri": fileURI(root), "name": filepath.Base(root)}},
	}
	if _, err := c.call(initCtx, "initialize", params); err != nil {
		c.Close()
		return nil, fmt.Errorf("initialize: %w", err)
	}
	_ = c.notify("initialized", map[string]any{})
	return c, nil
}

func fileURI(path string) string {
	p := filepath.ToSlash(path)
	if runtime.GOOS == "windows" {
		p = "/" + p
	}
	return (&url.URL{Scheme: "file", Path: p}).String()
}

func uriPath(uri string) string {
	u, err := url.Parse(uri)
	if err != nil {
		return uri
	}
	p := u.Path
	if runtime.GOOS == "windows" {
		p = strings.TrimPrefix(p, "/")
	}
	return filepath.FromSlash(p)
}

func (c *Client) send(v any) error {
	body, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err = fmt.Fprintf(c.stdin, "Content-Length: %d\r\n\r\n%s", len(body), body)
	return err
}

func (c *Client) notify(method string, params any) error {
	return c.send(map[string]any{"jsonrpc": "2.0", "method": method, "params": params})
}

func (c *Client) call(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errors.New("servidor LSP cerrado")
	}
	c.nextID++
	id := c.nextID
	ch := make(chan rpcMsg, 1)
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.send(map[string]any{"jsonrpc": "2.0", "id": id, "method": method, "params": params}); err != nil {
		return nil, err
	}
	select {
	case m, ok := <-ch:
		if !ok {
			return nil, errors.New("el servidor LSP se cerró")
		}
		if m.Error != nil {
			return nil, errors.New(m.Error.Message)
		}
		return m.Result, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

func (c *Client) readLoop(r *bufio.Reader) {
	defer func() {
		c.mu.Lock()
		c.closed = true
		for id, ch := range c.pending {
			close(ch)
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()
	for {
		length := -1
		for {
			line, err := r.ReadString('\n')
			if err != nil {
				return
			}
			line = strings.TrimRight(line, "\r\n")
			if line == "" {
				break
			}
			if k, v, ok := strings.Cut(line, ":"); ok && strings.EqualFold(strings.TrimSpace(k), "content-length") {
				length, _ = strconv.Atoi(strings.TrimSpace(v))
			}
		}
		if length < 0 {
			continue
		}
		buf := make([]byte, length)
		if _, err := io.ReadFull(r, buf); err != nil {
			return
		}
		var m rpcMsg
		if json.Unmarshal(buf, &m) != nil {
			continue
		}
		switch {
		case m.Method != "" && len(m.ID) > 0: // petición del servidor: responder algo inocuo
			var result any
			if m.Method == "workspace/configuration" {
				var p struct {
					Items []json.RawMessage `json:"items"`
				}
				_ = json.Unmarshal(m.Params, &p)
				out := make([]any, len(p.Items))
				result = out
			}
			_ = c.send(map[string]any{"jsonrpc": "2.0", "id": json.RawMessage(m.ID), "result": result})
		case m.Method == "textDocument/publishDiagnostics":
			c.onDiagnostics(m.Params)
		case m.Method == "" && len(m.ID) > 0:
			var id int64
			if json.Unmarshal(m.ID, &id) == nil {
				c.mu.Lock()
				ch := c.pending[id]
				delete(c.pending, id)
				c.mu.Unlock()
				if ch != nil {
					ch <- m
				}
			}
		}
	}
}

func (c *Client) onDiagnostics(raw json.RawMessage) {
	var p struct {
		URI         string `json:"uri"`
		Diagnostics []struct {
			Range struct {
				Start struct{ Line, Character int } `json:"start"`
			} `json:"range"`
			Severity int    `json:"severity"`
			Message  string `json:"message"`
			Source   string `json:"source"`
		} `json:"diagnostics"`
	}
	if json.Unmarshal(raw, &p) != nil {
		return
	}
	list := make([]Diagnostic, 0, len(p.Diagnostics))
	for _, d := range p.Diagnostics {
		sev := d.Severity
		if sev == 0 {
			sev = 1
		}
		list = append(list, Diagnostic{Line: d.Range.Start.Line + 1, Col: d.Range.Start.Character + 1, Severity: sev, Message: d.Message, Source: d.Source})
	}
	key := uriPath(p.URI)
	c.mu.Lock()
	c.diags[key] = list
	c.diagSeq[key]++
	close(c.diagCh)
	c.diagCh = make(chan struct{})
	c.mu.Unlock()
}

// sync abre o actualiza el documento con el contenido actual del disco.
func (c *Client) sync(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	c.mu.Lock()
	ver, was := c.opened[path]
	ver++
	c.opened[path] = ver
	c.mu.Unlock()
	if !was {
		return c.notify("textDocument/didOpen", map[string]any{"textDocument": map[string]any{
			"uri": fileURI(path), "languageId": languageID(path), "version": ver, "text": string(data)}})
	}
	return c.notify("textDocument/didChange", map[string]any{
		"textDocument":   map[string]any{"uri": fileURI(path), "version": ver},
		"contentChanges": []map[string]any{{"text": string(data)}}})
}

func languageID(path string) string {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".go":
		return "go"
	case ".py":
		return "python"
	case ".ts":
		return "typescript"
	case ".tsx":
		return "typescriptreact"
	case ".js", ".mjs":
		return "javascript"
	case ".jsx":
		return "javascriptreact"
	case ".rs":
		return "rust"
	}
	return "plaintext"
}

// Diagnostics sincroniza el archivo y espera hasta wait a que el servidor
// publique sus diagnósticos; devuelve los últimos recibidos.
func (c *Client) Diagnostics(ctx context.Context, path string, wait time.Duration) ([]Diagnostic, error) {
	path, _ = filepath.Abs(path)
	c.mu.Lock()
	before := c.diagSeq[path]
	c.mu.Unlock()
	if err := c.sync(path); err != nil {
		return nil, err
	}
	deadline := time.NewTimer(wait)
	defer deadline.Stop()
	settle := (*time.Timer)(nil)
	for {
		c.mu.Lock()
		seq, ch, closed := c.diagSeq[path], c.diagCh, c.closed
		c.mu.Unlock()
		if seq > before && settle == nil {
			settle = time.NewTimer(250 * time.Millisecond) // por si llega una segunda publicación más completa
			defer settle.Stop()
		}
		var settled <-chan time.Time
		if settle != nil {
			settled = settle.C
		}
		if closed {
			break
		}
		select {
		case <-ch:
		case <-settled:
			goto done
		case <-deadline.C:
			goto done
		case <-ctx.Done():
			return nil, ctx.Err()
		}
	}
done:
	c.mu.Lock()
	out := append([]Diagnostic(nil), c.diags[path]...)
	c.mu.Unlock()
	return out, nil
}

func (c *Client) position(path string, line, col int) map[string]any {
	if line < 1 {
		line = 1
	}
	if col < 1 {
		col = 1
	}
	return map[string]any{"textDocument": map[string]any{"uri": fileURI(path)}, "position": map[string]any{"line": line - 1, "character": col - 1}}
}

func parseLocations(raw json.RawMessage) []Location {
	if len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return nil
	}
	type loc struct {
		URI       string `json:"uri"`
		TargetURI string `json:"targetUri"`
		Range     *struct {
			Start struct{ Line, Character int } `json:"start"`
		} `json:"range"`
		TargetRange *struct {
			Start struct{ Line, Character int } `json:"start"`
		} `json:"targetSelectionRange"`
	}
	var many []loc
	if json.Unmarshal(raw, &many) != nil {
		var one loc
		if json.Unmarshal(raw, &one) != nil {
			return nil
		}
		many = []loc{one}
	}
	var out []Location
	for _, l := range many {
		uri, line, ch := l.URI, 0, 0
		if l.Range != nil {
			line, ch = l.Range.Start.Line, l.Range.Start.Character
		}
		if uri == "" {
			uri = l.TargetURI
			if l.TargetRange != nil {
				line, ch = l.TargetRange.Start.Line, l.TargetRange.Start.Character
			}
		}
		out = append(out, Location{Path: uriPath(uri), Line: line + 1, Col: ch + 1})
	}
	return out
}

// Definition devuelve dónde se define el símbolo en esa posición.
func (c *Client) Definition(ctx context.Context, path string, line, col int) ([]Location, error) {
	path, _ = filepath.Abs(path)
	if err := c.sync(path); err != nil {
		return nil, err
	}
	raw, err := c.call(ctx, "textDocument/definition", c.position(path, line, col))
	return parseLocations(raw), err
}

// References devuelve los usos del símbolo.
func (c *Client) References(ctx context.Context, path string, line, col int) ([]Location, error) {
	path, _ = filepath.Abs(path)
	if err := c.sync(path); err != nil {
		return nil, err
	}
	p := c.position(path, line, col)
	p["context"] = map[string]any{"includeDeclaration": true}
	raw, err := c.call(ctx, "textDocument/references", p)
	return parseLocations(raw), err
}

// Hover devuelve la documentación/tipo del símbolo.
func (c *Client) Hover(ctx context.Context, path string, line, col int) (string, error) {
	path, _ = filepath.Abs(path)
	if err := c.sync(path); err != nil {
		return "", err
	}
	raw, err := c.call(ctx, "textDocument/hover", c.position(path, line, col))
	if err != nil || len(bytes.TrimSpace(raw)) == 0 || string(bytes.TrimSpace(raw)) == "null" {
		return "", err
	}
	var h struct {
		Contents json.RawMessage `json:"contents"`
	}
	if json.Unmarshal(raw, &h) != nil {
		return "", nil
	}
	return hoverText(h.Contents), nil
}

func hoverText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var obj struct {
		Value string `json:"value"`
	}
	if json.Unmarshal(raw, &obj) == nil && obj.Value != "" {
		return obj.Value
	}
	var arr []json.RawMessage
	if json.Unmarshal(raw, &arr) == nil {
		var parts []string
		for _, a := range arr {
			if t := hoverText(a); t != "" {
				parts = append(parts, t)
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// Close termina el servidor.
func (c *Client) Close() {
	c.mu.Lock()
	already := c.closed
	c.closed = true
	c.mu.Unlock()
	if !already {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		_, _ = c.call(ctx, "shutdown", nil)
		cancel()
		_ = c.notify("exit", nil)
	}
	_ = c.stdin.Close()
	done := make(chan struct{})
	go func() { _ = c.cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		_ = c.cmd.Process.Kill()
		<-done
	}
}
