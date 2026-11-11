package api

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStreamParsesTimings(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hola\"}}]}\n\n")
		fl.Flush()
		fmt.Fprint(w, "data: {\"choices\":[],\"timings\":{\"prompt_n\":100,\"predicted_n\":20}}\n\n")
		fl.Flush()
		fmt.Fprint(w, "data: [DONE]\n\n")
		fl.Flush()
	}))
	defer srv.Close()

	s := Stream(context.Background(), Request{
		Bases:    []string{srv.URL + "/v1"},
		APIKey:   "k",
		Model:    "m",
		Messages: []ChatMessage{{Role: "user", Content: "x"}},
	})
	var text string
	var usage *Usage
	for {
		c := s.Next()
		text += c.Text
		if c.Usage != nil {
			usage = c.Usage
		}
		if c.Done {
			if c.Err != nil {
				t.Fatal(c.Err)
			}
			break
		}
	}
	if text != "hola" {
		t.Fatalf("texto = %q", text)
	}
	if usage == nil || usage.PromptTokens != 100 || usage.CompletionTokens != 20 || usage.TotalTokens != 120 {
		t.Fatalf("usage = %+v", usage)
	}
}

func TestPropsContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/props" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			w.WriteHeader(401)
			return
		}
		_, _ = io.WriteString(w, `{"default_generation_settings":{"n_ctx":196608}}`)
	}))
	defer srv.Close()
	if got := PropsContext(context.Background(), []string{srv.URL + "/v1"}, "k"); got != 196608 {
		t.Fatalf("n_ctx = %d", got)
	}
}

func TestIsContextOverflow(t *testing.T) {
	err := &HTTPError{Code: 400, Body: `{"error":{"message":"the request exceeds the available context size"}}`}
	if !IsContextOverflow(err) {
		t.Fatal("deberia detectar el 400 de contexto")
	}
	if !strings.Contains(err.Error(), "/compact") {
		t.Fatalf("mensaje = %q", err.Error())
	}
	if IsContextOverflow(&HTTPError{Code: 400, Body: "bad request"}) {
		t.Fatal("no deberia confundir un 400 normal")
	}
}
