package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestStreamRazonamientoYParametros(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"reasoning_content\":\"pienso\"}}]}\n\n")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hola\"}}]}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	off := false
	s := Stream(context.Background(), Request{Bases: []string{srv.URL}, Model: "m", MaxTokens: 777, Thinking: &off,
		Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	var think, text string
	for {
		c := s.Next()
		think += c.Reasoning
		text += c.Text
		if c.Done {
			if c.Err != nil {
				t.Fatal(c.Err)
			}
			break
		}
	}
	if think != "pienso" || text != "hola" {
		t.Fatalf("think=%q text=%q", think, text)
	}
	if body["max_tokens"].(float64) != 777 {
		t.Fatalf("max_tokens: %v", body["max_tokens"])
	}
	kw := body["chat_template_kwargs"].(map[string]any)
	if kw["enable_thinking"].(bool) != false {
		t.Fatalf("enable_thinking: %v", kw)
	}
}

func TestUsageConCacheNoSalta(t *testing.T) {
	var body map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		json.Unmarshal(raw, &body)
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hola\"}}]}\n\n")
		// llama.cpp con caché: solo 500 tokens procesados de nuevo, 9000 reutilizados
		fmt.Fprint(w, "data: {\"choices\":[],\"timings\":{\"prompt_n\":500,\"cache_n\":9000,\"predicted_n\":20}}\n\n")
		fmt.Fprint(w, "data: [DONE]\n\n")
	}))
	defer srv.Close()
	s := Stream(context.Background(), Request{Bases: []string{srv.URL}, Model: "m", Messages: []ChatMessage{{Role: "user", Content: "x"}}})
	total := 0
	for {
		c := s.Next()
		if c.Usage != nil {
			total = c.Usage.TotalTokens
		}
		if c.Done {
			break
		}
	}
	if total != 9520 {
		t.Fatalf("total=%d, quería 9520 (500+9000+20)", total)
	}
	if so, ok := body["stream_options"].(map[string]any); !ok || so["include_usage"] != true {
		t.Fatalf("stream_options: %v", body["stream_options"])
	}
}
