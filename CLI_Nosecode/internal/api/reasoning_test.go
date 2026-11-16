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
