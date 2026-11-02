// Package api habla con endpoints compatibles con la API de OpenAI (llama-server)
// y controla el Dev Vault (encender/apagar la IA en Kaggle).
package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// ChatMessage es un turno enviado al modelo.
type ChatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolDef describe una herramienta para la API de OpenAI.
type ToolDef struct {
	Type     string      `json:"type"`
	Function FunctionDef `json:"function"`
}

// FunctionDef es la firma de una herramienta.
type FunctionDef struct {
	Name        string          `json:"name"`
	Description string          `json:"description,omitempty"`
	Parameters  json.RawMessage `json:"parameters,omitempty"`
}

// ToolCall es una llamada a herramienta pedida por el modelo.
type ToolCall struct {
	ID       string `json:"id,omitempty"`
	Type     string `json:"type,omitempty"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolCallDelta es un trozo de tool_call durante el streaming.
type ToolCallDelta struct {
	Index     int    `json:"index"`
	ID        string `json:"id,omitempty"`
	Type      string `json:"type,omitempty"`
	Name      string `json:"name,omitempty"`
	Arguments string `json:"arguments,omitempty"`
}

// ErrCanceled indica que el usuario cancelo la respuesta (esc).
var ErrCanceled = errors.New("cancelado")

// HTTPError es una respuesta HTTP no exitosa del endpoint o del tunel.
type HTTPError struct {
	Code int
	Body string
}

func (e *HTTPError) Error() string {
	switch {
	case e.Code == 401:
		return "API key incorrecta (401)"
	case e.Code == 400 || e.Code == 404 || e.Code == 422:
		return fmt.Sprintf("peticion rechazada (%d) %s", e.Code, e.Body)
	case e.Code == 402 || e.Code == 403 || e.Code == 429:
		return fmt.Sprintf("tunel sin cupo o bloqueado (%d)", e.Code)
	case e.Code >= 500:
		return fmt.Sprintf("tunel caido o IA no responde (%d)", e.Code)
	}
	return fmt.Sprintf("error HTTP %d %s", e.Code, e.Body)
}

// retryable dice si conviene probar otro tunel (la clave mala no se arregla cambiando de tunel).
func (e *HTTPError) retryable() bool {
	switch e.Code {
	case 402, 403, 429:
		return true
	}
	return e.Code >= 500
}

// Chunk es un trozo de la respuesta. Cuando Done es true el flujo termino
// (Err vale nil si fue bien, ErrCanceled si se cancelo, u otro error).
type Chunk struct {
	Text      string
	ToolCalls []ToolCallDelta
	Done      bool
	Err       error
	Base      string // URL base que respondio (util para avisar de un cambio de tunel)
}

// Request describe una conversacion a enviar.
type Request struct {
	Bases    []string // URLs base (terminan en /v1), por orden de preferencia
	APIKey   string
	Model    string
	Messages []ChatMessage
	Tools    []ToolDef
}

// Streamer entrega la respuesta trozo a trozo.
type Streamer struct {
	ch     chan Chunk
	cancel context.CancelFunc
}

// Next bloquea hasta el siguiente trozo. Tras Done no vuelve a llamarse.
func (s *Streamer) Next() Chunk {
	c, ok := <-s.ch
	if !ok {
		return Chunk{Done: true}
	}
	return c
}

// Cancel corta la peticion en curso.
func (s *Streamer) Cancel() { s.cancel() }

// Stream lanza la peticion en segundo plano. Prueba cada URL base en orden y
// pasa a la siguiente si hay error de conexion o de tunel/cupo ANTES de recibir texto.
func Stream(parent context.Context, req Request) *Streamer {
	ctx, cancel := context.WithCancel(parent)
	s := &Streamer{ch: make(chan Chunk, 64), cancel: cancel}
	go func() {
		defer close(s.ch)
		defer cancel()
		send := func(c Chunk) bool {
			select {
			case s.ch <- c:
				return true
			case <-ctx.Done():
				return false
			}
		}

		final := func(c Chunk) {
			select {
			case s.ch <- c:
			case <-time.After(2 * time.Second):
			}
		}

		var last error
		for _, base := range req.Bases {
			got, err := streamOne(ctx, base, req, send)
			if err == nil {
				send(Chunk{Done: true, Base: base})
				return
			}
			if ctx.Err() != nil {
				final(Chunk{Done: true, Err: ErrCanceled, Base: base})
				return
			}
			last = err
			var he *HTTPError
			if got { // ya llego texto: no se puede cambiar de tunel a mitad de respuesta
				final(Chunk{Done: true, Err: err, Base: base})
				return
			}
			if errors.As(err, &he) && !he.retryable() {
				final(Chunk{Done: true, Err: err, Base: base})
				return
			}
		}
		if last == nil {
			last = errors.New("no hay URL base configurada")
		}
		final(Chunk{Done: true, Err: last})
	}()
	return s
}

var httpClient = &http.Client{Timeout: 0} // sin limite global: el contexto controla la cancelacion

// streamOne hace una peticion SSE. got indica si llego algun texto.
func rawString(raw json.RawMessage) string {
	if len(raw) == 0 || string(raw) == "null" {
		return ""
	}
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return string(raw)
}

func streamOne(ctx context.Context, base string, r Request, send func(Chunk) bool) (got bool, err error) {
	payload := map[string]any{
		"model":    r.Model,
		"messages": r.Messages,
		"stream":   true,
	}
	if len(r.Tools) > 0 {
		payload["tools"] = r.Tools
		payload["tool_choice"] = "auto"
	}
	body, _ := json.Marshal(payload)
	url := strings.TrimRight(base, "/") + "/chat/completions"
	hr, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return false, err
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("Accept", "text/event-stream")
	hr.Header.Set("Authorization", "Bearer "+r.APIKey)
	hr.Header.Set("ngrok-skip-browser-warning", "1")
	hr.Header.Set("User-Agent", "nosecode")

	resp, err := httpClient.Do(hr)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 300))
		return false, &HTTPError{Code: resp.StatusCode, Body: strings.TrimSpace(string(b))}
	}

	sc := bufio.NewScanner(resp.Body)
	sc.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		payload := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if payload == "[DONE]" {
			return got, nil
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string          `json:"name"`
							Arguments json.RawMessage `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(payload), &ev) != nil || len(ev.Choices) == 0 {
			continue
		}
		d := ev.Choices[0].Delta
		if len(d.ToolCalls) > 0 {
			got = true
			calls := make([]ToolCallDelta, 0, len(d.ToolCalls))
			for _, tc := range d.ToolCalls {
				calls = append(calls, ToolCallDelta{
					Index:     tc.Index,
					ID:        tc.ID,
					Type:      tc.Type,
					Name:      tc.Function.Name,
					Arguments: rawString(tc.Function.Arguments),
				})
			}
			if !send(Chunk{ToolCalls: calls, Base: base}) {
				return got, ctx.Err()
			}
		}
		if t := d.Content; t != "" {
			got = true
			if !send(Chunk{Text: t, Base: base}) {
				return got, ctx.Err()
			}
		}
	}
	if err := sc.Err(); err != nil {
		return got, err
	}
	if ctx.Err() != nil {
		return got, ctx.Err()
	}
	return got, nil
}

// Bases devuelve las URL base a probar: primero la del proveedor y despues los
// otros tuneles guardados por el Dev Vault en state.json (ngrok -> tailscale).
// Cloudflare Quick Tunnel se omite porque no soporta streaming (SSE).
func Bases(primary, vaultHome string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(u string) {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		if u == "" {
			return
		}
		if !strings.HasSuffix(u, "/v1") {
			u += "/v1"
		}
		if !seen[u] {
			seen[u] = true
			out = append(out, u)
		}
	}
	add(primary)

	raw, err := os.ReadFile(filepath.Join(vaultHome, "state.json"))
	if err != nil {
		return out
	}
	var st struct {
		URL  string            `json:"url"`
		URLs map[string]string `json:"urls"`
	}
	if json.Unmarshal(raw, &st) != nil {
		return out
	}
	rank := map[string]int{"ngrok": 0, "tailscale": 1}
	names := make([]string, 0, len(st.URLs))
	for n := range st.URLs {
		if n == "cloudflared" {
			continue
		}
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		ri, okI := rank[names[i]]
		rj, okJ := rank[names[j]]
		if okI != okJ {
			return okI
		}
		if ri != rj {
			return ri < rj
		}
		return names[i] < names[j]
	})
	add(st.URL)
	for _, n := range names {
		add(st.URLs[n])
	}
	return out
}

// Ping comprueba /health de una URL base (sin /v1) con un tiempo maximo corto.
func Ping(ctx context.Context, base string) bool {
	root := strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, root+"/health", nil)
	if err != nil {
		return false
	}
	req.Header.Set("ngrok-skip-browser-warning", "1")
	resp, err := httpClient.Do(req)
	if err != nil {
		return false
	}
	resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
