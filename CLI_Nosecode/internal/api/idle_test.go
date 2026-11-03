package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestStreamCortaSiElModeloSeQuedaInactivo(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fl := w.(http.Flusher)
		fmt.Fprint(w, "data: {\"choices\":[{\"delta\":{\"content\":\"hola\"}}]}\n\n")
		fl.Flush()
		select {
		case <-r.Context().Done():
		case <-time.After(2 * time.Second):
		}
	}))
	defer srv.Close()

	req := req(srv.URL + "/v1")
	req.IdleTimeout = 120 * time.Millisecond
	start := time.Now()
	text, last := collect(Stream(context.Background(), req))
	if text != "hola" {
		t.Fatalf("texto = %q", text)
	}
	if !errors.Is(last.Err, ErrModelIdle) {
		t.Fatalf("esperaba ErrModelIdle, llego %v", last.Err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("el corte tardo demasiado: %v", elapsed)
	}
}
