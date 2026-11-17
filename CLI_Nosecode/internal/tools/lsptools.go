package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"nosecode/internal/lsp"
)

func runDiagnostics(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Path string `json:"path"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	if !lsp.Default.Supported(path) {
		return errResult("no hay servidor LSP configurado para %s (config: lsp.servers)", filepath.Ext(path))
	}
	cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	c, err := lsp.Default.ClientFor(cctx, path)
	if err != nil {
		return errResult("no pude arrancar el servidor LSP: %v", err)
	}
	ds, err := c.Diagnostics(cctx, path, 10*time.Second)
	if err != nil {
		return errResult("%v", err)
	}
	text := lsp.FormatDiagnostics(path, ds, false, 40)
	if text == "" {
		text = "sin errores ni avisos en " + filepath.Base(path)
	}
	return Result{Text: text, Bytes: len(text)}
}

func runLSP(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Action string `json:"action"`
		Path   string `json:"path"`
		Line   int    `json:"line"`
		Column int    `json:"column"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	if !lsp.Default.Supported(path) {
		return errResult("no hay servidor LSP configurado para %s (config: lsp.servers)", filepath.Ext(path))
	}
	cctx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	c, err := lsp.Default.ClientFor(cctx, path)
	if err != nil {
		return errResult("no pude arrancar el servidor LSP: %v", err)
	}
	if a.Column < 1 {
		a.Column = 1
	}
	var text string
	switch a.Action {
	case "definition", "references":
		var locs []lsp.Location
		if a.Action == "definition" {
			locs, err = c.Definition(cctx, path, a.Line, a.Column)
		} else {
			locs, err = c.References(cctx, path, a.Line, a.Column)
		}
		if err != nil {
			return errResult("%v", err)
		}
		var b strings.Builder
		for i, l := range locs {
			if i >= 40 {
				fmt.Fprintf(&b, "… y %d más\n", len(locs)-40)
				break
			}
			fmt.Fprintf(&b, "%s:%d:%d\n", l.Path, l.Line, l.Col)
		}
		text = strings.TrimRight(b.String(), "\n")
		if text == "" {
			text = "(sin resultados)"
		}
	case "hover":
		text, err = c.Hover(cctx, path, a.Line, a.Column)
		if err != nil {
			return errResult("%v", err)
		}
		if strings.TrimSpace(text) == "" {
			text = "(sin información en esa posición)"
		}
	default:
		return errResult("acción desconocida %q (definition, references o hover)", a.Action)
	}
	text = truncateText(text, 6000)
	return Result{Text: text, Bytes: len(text)}
}
