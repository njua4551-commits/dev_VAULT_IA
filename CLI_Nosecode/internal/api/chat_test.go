package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func sseServer(t *testing.T, key string, pieces []string, gap time.Duration) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer "+key {
			w.WriteHeader(401)
			return
		}
		var body struct {
			Stream   bool          `json:"stream"`
			Model    string        `json:"model"`
			Messages []ChatMessage `json:"messages"`
		}
		_ = json.NewDecoder(r.Body).Decode(&body)
		if !body.Stream || body.Model != "m" || len(body.Messages) != 1 {
			w.WriteHeader(400)
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

func collect(s *Streamer) (string, Chunk) {
	var sb strings.Builder
	for {
		c := s.Next()
		sb.WriteString(c.Text)
		if c.Done {
			return sb.String(), c
		}
	}
}

func req(bases ...string) Request {
	return Request{Bases: bases, APIKey: "k", Model: "m", Messages: []ChatMessage{{Role: "user", Content: "hola"}}}
}

func TestStreamEntregaTrozos(t *testing.T) {
	srv := sseServer(t, "k", []string{"Hola ", "desde ", "la IA"}, 0)
	defer srv.Close()
	text, last := collect(Stream(context.Background(), req(srv.URL+"/v1")))
	if last.Err != nil || text != "Hola desde la IA" {
		t.Fatalf("text=%q err=%v", text, last.Err)
	}
}

func TestStreamCancelar(t *testing.T) {
	srv := sseServer(t, "k", []string{"a", "b", "c", "d", "e", "f", "g", "h"}, 150*time.Millisecond)
	defer srv.Close()
	s := Stream(context.Background(), req(srv.URL+"/v1"))
	first := s.Next()
	if first.Text != "a" {
		t.Fatalf("primer trozo = %+v", first)
	}
	start := time.Now()
	s.Cancel()
	var last Chunk
	for {
		last = s.Next()
		if last.Done {
			break
		}
	}
	if last.Err != ErrCanceled {
		t.Fatalf("esperaba ErrCanceled, llego %v", last.Err)
	}
	if time.Since(start) > 900*time.Millisecond {
		t.Fatalf("cancelar tardo demasiado: %v", time.Since(start))
	}
}

func TestStream401NoCambiaDeTunel(t *testing.T) {
	bad := sseServer(t, "otra", nil, 0)
	good := sseServer(t, "k", []string{"x"}, 0)
	defer bad.Close()
	defer good.Close()
	_, last := collect(Stream(context.Background(), req(bad.URL+"/v1", good.URL+"/v1")))
	he, ok := last.Err.(*HTTPError)
	if !ok || he.Code != 401 || !strings.Contains(he.Error(), "API key incorrecta") {
		t.Fatalf("err = %v", last.Err)
	}
}

func TestStreamCambiaDeTunelSiHay5xxOCaida(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
	quota := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(429) }))
	good := sseServer(t, "k", []string{"ok"}, 0)
	dead := httptest.NewServer(http.NotFoundHandler())
	deadURL := dead.URL
	dead.Close() // puerto cerrado: error de conexion
	defer down.Close()
	defer quota.Close()
	defer good.Close()
	text, last := collect(Stream(context.Background(), req(deadURL+"/v1", down.URL+"/v1", quota.URL+"/v1", good.URL+"/v1")))
	if last.Err != nil || text != "ok" || last.Base != good.URL+"/v1" {
		t.Fatalf("text=%q last=%+v", text, last)
	}
}

func TestStreamTodosFallan(t *testing.T) {
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(502) }))
	defer down.Close()
	_, last := collect(Stream(context.Background(), req(down.URL+"/v1")))
	if last.Err == nil || !strings.Contains(last.Err.Error(), "tunel caido") {
		t.Fatalf("err = %v", last.Err)
	}
	_, last = collect(Stream(context.Background(), req()))
	if last.Err == nil {
		t.Fatal("sin bases debia fallar")
	}
}

func TestBasesPrioridadYSinCloudflare(t *testing.T) {
	dir := t.TempDir()
	state := `{"url":"https://a.ngrok-free.dev","urls":{"cloudflared":"https://x.trycloudflare.com","tailscale":"https://k.ts.net","ngrok":"https://a.ngrok-free.dev"}}`
	if err := os.WriteFile(filepath.Join(dir, "state.json"), []byte(state), 0o600); err != nil {
		t.Fatal(err)
	}
	got := Bases("https://a.ngrok-free.dev/v1", dir)
	want := []string{"https://a.ngrok-free.dev/v1", "https://k.ts.net/v1"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("got %v want %v", got, want)
	}
	if g := Bases("https://p.dev", t.TempDir()); len(g) != 1 || g[0] != "https://p.dev/v1" {
		t.Fatalf("sin state.json: %v", g)
	}
}

func TestPing(t *testing.T) {
	ok := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" {
			w.WriteHeader(200)
			return
		}
		w.WriteHeader(404)
	}))
	defer ok.Close()
	if !Ping(context.Background(), ok.URL+"/v1") {
		t.Fatal("debia estar vivo")
	}
	u := ok.URL
	ok.Close()
	if Ping(context.Background(), u) {
		t.Fatal("cerrado debia dar false")
	}
}
