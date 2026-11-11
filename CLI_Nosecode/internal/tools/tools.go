// Package tools implementa las herramientas nativas de Nosecode. No depende de
// MCP: son funciones locales con esquemas minimos para gastar poco contexto.
package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Image es una imagen devuelta por una herramienta (base64).
type Image struct {
	MIME string
	Data string
}

// Result es el resultado de ejecutar una herramienta.
type Result struct {
	Text   string
	Images []Image
	Error  bool
}

// Tool describe una herramienta nativa.
type Tool struct {
	Name        string
	Description string
	Parameters  json.RawMessage
	Run         func(context.Context, json.RawMessage) Result
}

var (
	builtinOrder []string
	registry     = map[string]Tool{}
)

func init() {
	list := []Tool{
		{
			Name:        "bash",
			Description: "Ejecuta un comando en PowerShell (Windows) o bash/sh (Linux); timeout 60 s y salida máx. 8 KB.",
			Parameters:  schema(`{"type":"object","properties":{"command":{"type":"string","description":"Comando a ejecutar"}},"required":["command"],"additionalProperties":false}`),
			Run:         runBash,
		},
		{
			Name:        "read_file",
			Description: "Lee un archivo por rango de líneas (start/end, 1-based); máx. 400 líneas.",
			Parameters:  schema(`{"type":"object","properties":{"path":{"type":"string"},"start":{"type":"integer"},"end":{"type":"integer"}},"required":["path"],"additionalProperties":false}`),
			Run:         runReadFile,
		},
		{
			Name:        "write_file",
			Description: "Escribe un archivo completo, creando sus carpetas si faltan.",
			Parameters:  schema(`{"type":"object","properties":{"path":{"type":"string"},"content":{"type":"string"}},"required":["path","content"],"additionalProperties":false}`),
			Run:         runWriteFile,
		},
		{
			Name:        "edit_file",
			Description: "Reemplaza texto exacto una sola vez; falla si no existe o aparece más de una vez.",
			Parameters:  schema(`{"type":"object","properties":{"path":{"type":"string"},"old":{"type":"string"},"new":{"type":"string"}},"required":["path","old","new"],"additionalProperties":false}`),
			Run:         runEditFile,
		},
		{
			Name:        "list_dir",
			Description: "Lista el contenido de una carpeta (máx. 200 entradas).",
			Parameters:  schema(`{"type":"object","properties":{"path":{"type":"string"}},"additionalProperties":false}`),
			Run:         runListDir,
		},
		{
			Name:        "grep",
			Description: "Busca una expresión regular en archivos de texto y devuelve coincidencias.",
			Parameters:  schema(`{"type":"object","properties":{"pattern":{"type":"string"},"path":{"type":"string"},"glob":{"type":"string"},"ignore_case":{"type":"boolean"}},"required":["pattern"],"additionalProperties":false}`),
			Run:         runGrep,
		},
		{
			Name:        "screenshot",
			Description: "Captura la pantalla y la envía al modelo reducida a 1024 px.",
			Parameters:  schema(`{"type":"object","properties":{},"additionalProperties":false}`),
			Run:         runScreenshot,
		},
		{
			Name:        "view_image",
			Description: "Lee una imagen del disco y la envía al modelo reducida a 1024 px.",
			Parameters:  schema(`{"type":"object","properties":{"path":{"type":"string"}},"required":["path"],"additionalProperties":false}`),
			Run:         runViewImage,
		},
		{
			Name:        "web_fetch",
			Description: "Descarga una URL y devuelve texto plano (HTML convertido, máx. 20 KB).",
			Parameters:  schema(`{"type":"object","properties":{"url":{"type":"string"},"max_bytes":{"type":"integer"}},"required":["url"],"additionalProperties":false}`),
			Run:         runWebFetch,
		},
		{
			Name:        "remember",
			Description: "Añade una nota a NOSECODE.md para recordarla en próximas sesiones.",
			Parameters:  schema(`{"type":"object","properties":{"text":{"type":"string"}},"required":["text"],"additionalProperties":false}`),
			Run:         runRemember,
		},
	}
	builtinOrder = make([]string, 0, len(list))
	for _, t := range list {
		builtinOrder = append(builtinOrder, t.Name)
		registry[t.Name] = t
	}
}

func schema(s string) json.RawMessage { return json.RawMessage(s) }

// Builtins devuelve las herramientas nativas en orden estable.
func Builtins() []Tool {
	out := make([]Tool, 0, len(builtinOrder))
	for _, name := range builtinOrder {
		out = append(out, registry[name])
	}
	return out
}

// Lookup busca una herramienta nativa por nombre.
func Lookup(name string) (Tool, bool) {
	t, ok := registry[name]
	return t, ok
}

// Names devuelve los nombres nativos en orden.
func Names() []string { return append([]string(nil), builtinOrder...) }

func decode(args json.RawMessage, v any) error {
	if len(bytes.TrimSpace(args)) == 0 {
		args = []byte("{}")
	}
	return json.Unmarshal(args, v)
}

func errResult(format string, args ...any) Result {
	return Result{Text: "Error: " + fmt.Sprintf(format, args...), Error: true}
}

func resolvePath(p string) (string, error) {
	p = strings.TrimSpace(p)
	if p == "" {
		return "", fmt.Errorf("la ruta está vacía")
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("no pude obtener la carpeta del usuario: %w", err)
		}
		if p == "~" {
			return home, nil
		}
		p = filepath.Join(home, p[2:])
	}
	return filepath.Clean(p), nil
}

// TruncateText recorta texto sin partir un carácter UTF-8.
func TruncateText(s string, max int) string { return truncateText(s, max) }

func truncateText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	cut := max
	for cut > 0 && cut < len(s) && (s[cut]&0xc0) == 0x80 {
		cut--
	}
	return s[:cut] + "\n[recortado]"
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".nosecode-edit-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, path)
}
