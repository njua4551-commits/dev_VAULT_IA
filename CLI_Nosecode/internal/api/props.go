package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"
)

// PropsContext lee /props de llama-server y devuelve n_ctx (0 si no está).
// Prueba las URL base en orden con un timeout corto para no bloquear.
func PropsContext(ctx context.Context, bases []string, apiKey string) int {
	for _, base := range bases {
		root := strings.TrimSuffix(strings.TrimRight(base, "/"), "/v1")
		if root == "" {
			continue
		}
		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, root+"/props", nil)
		if err != nil {
			cancel()
			continue
		}
		if apiKey != "" {
			req.Header.Set("Authorization", "Bearer "+apiKey)
		}
		req.Header.Set("ngrok-skip-browser-warning", "1")
		req.Header.Set("User-Agent", "nosecode")
		resp, err := httpClient.Do(req)
		if err != nil {
			cancel()
			continue
		}
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512<<10))
		resp.Body.Close()
		cancel()
		if resp.StatusCode != http.StatusOK {
			continue
		}
		var p struct {
			NCtx                      int `json:"n_ctx"`
			DefaultGenerationSettings struct {
				NCtx int `json:"n_ctx"`
			} `json:"default_generation_settings"`
		}
		if json.Unmarshal(body, &p) != nil {
			continue
		}
		if p.DefaultGenerationSettings.NCtx > 0 {
			return p.DefaultGenerationSettings.NCtx
		}
		if p.NCtx > 0 {
			return p.NCtx
		}
	}
	return 0
}

// Complete ejecuta una petición sin herramientas y devuelve todo el texto.
// Se usa para resumir (/compact); respeta el mismo streaming y los tuneles.
func Complete(ctx context.Context, req Request) (string, error) {
	req.Tools = nil
	s := Stream(ctx, req)
	var b strings.Builder
	for {
		c := s.Next()
		if c.Text != "" {
			b.WriteString(c.Text)
		}
		if c.Done {
			return b.String(), c.Err
		}
	}
}
