package lsp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// DefaultServers asocia extensiones con el servidor LSP habitual.
var DefaultServers = map[string][]string{
	".go":  {"gopls"},
	".py":  {"pyright-langserver", "--stdio"},
	".ts":  {"typescript-language-server", "--stdio"},
	".tsx": {"typescript-language-server", "--stdio"},
	".js":  {"typescript-language-server", "--stdio"},
	".jsx": {"typescript-language-server", "--stdio"},
	".rs":  {"rust-analyzer"},
}

var rootMarkers = []string{"go.mod", "pyproject.toml", "package.json", "Cargo.toml", ".git"}

// Manager arranca los servidores bajo demanda y los reutiliza.
type Manager struct {
	mu      sync.Mutex
	Enabled bool
	Servers map[string][]string // extensión → comando (sobrescribe DefaultServers)
	clients map[string]*Client  // clave: comando + raíz
	failed  map[string]string   // clave → motivo (no se reintenta en la sesión)
}

// Default es el gestor global que usan las herramientas.
var Default = &Manager{Enabled: true}

func (m *Manager) command(ext string) []string {
	if c, ok := m.Servers[ext]; ok {
		return c
	}
	return DefaultServers[ext]
}

// Supported dice si hay un servidor configurado para ese archivo.
func (m *Manager) Supported(path string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.Enabled && len(m.command(strings.ToLower(filepath.Ext(path)))) > 0
}

// Root busca la raíz del proyecto subiendo desde el archivo.
func Root(path string) string {
	dir := filepath.Dir(path)
	for d := dir; ; d = filepath.Dir(d) {
		for _, mk := range rootMarkers {
			if _, err := os.Stat(filepath.Join(d, mk)); err == nil {
				return d
			}
		}
		if filepath.Dir(d) == d {
			return dir
		}
	}
}

// ClientFor devuelve (arrancando si hace falta) el cliente del archivo.
func (m *Manager) ClientFor(ctx context.Context, path string) (*Client, error) {
	path, _ = filepath.Abs(path)
	m.mu.Lock()
	if !m.Enabled {
		m.mu.Unlock()
		return nil, fmt.Errorf("LSP desactivado")
	}
	cmd := m.command(strings.ToLower(filepath.Ext(path)))
	if len(cmd) == 0 {
		m.mu.Unlock()
		return nil, fmt.Errorf("no hay servidor LSP para %s", filepath.Ext(path))
	}
	root := Root(path)
	key := strings.Join(cmd, " ") + "@" + root
	if c, ok := m.clients[key]; ok {
		m.mu.Unlock()
		return c, nil
	}
	if why, bad := m.failed[key]; bad {
		m.mu.Unlock()
		return nil, fmt.Errorf("%s", why)
	}
	m.mu.Unlock()
	c, err := Start(ctx, root, cmd)
	m.mu.Lock()
	defer m.mu.Unlock()
	if err != nil {
		if m.failed == nil {
			m.failed = map[string]string{}
		}
		m.failed[key] = err.Error()
		return nil, err
	}
	if m.clients == nil {
		m.clients = map[string]*Client{}
	}
	m.clients[key] = c
	return c, nil
}

// CloseAll apaga todos los servidores.
func (m *Manager) CloseAll() {
	m.mu.Lock()
	list := make([]*Client, 0, len(m.clients))
	for _, c := range m.clients {
		list = append(list, c)
	}
	m.clients = nil
	m.mu.Unlock()
	for _, c := range list {
		c.Close()
	}
}

// FormatDiagnostics resume los diagnósticos (errores primero, máx. max líneas).
func FormatDiagnostics(path string, ds []Diagnostic, onlyErrors bool, max int) string {
	var sel []Diagnostic
	for _, d := range ds {
		if !onlyErrors || d.Severity == 1 {
			sel = append(sel, d)
		}
	}
	sort.SliceStable(sel, func(i, j int) bool {
		if sel[i].Severity != sel[j].Severity {
			return sel[i].Severity < sel[j].Severity
		}
		return sel[i].Line < sel[j].Line
	})
	var b strings.Builder
	for i, d := range sel {
		if i >= max {
			fmt.Fprintf(&b, "… y %d más\n", len(sel)-max)
			break
		}
		sev := map[int]string{1: "error", 2: "aviso", 3: "info", 4: "pista"}[d.Severity]
		fmt.Fprintf(&b, "%s:%d:%d %s: %s\n", filepath.Base(path), d.Line, d.Col, sev, strings.Join(strings.Fields(d.Message), " "))
	}
	return strings.TrimRight(b.String(), "\n")
}

// PostEditNotes devuelve los errores del servidor LSP para los archivos
// recién editados (vacío si no hay servidor, no hay errores o tarda demasiado).
func (m *Manager) PostEditNotes(files []string) string {
	var out strings.Builder
	for _, f := range files {
		if !m.Supported(f) {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		c, err := m.ClientFor(ctx, f)
		if err != nil {
			cancel()
			continue
		}
		ds, err := c.Diagnostics(ctx, f, 6*time.Second)
		cancel()
		if err != nil {
			continue
		}
		if txt := FormatDiagnostics(f, ds, true, 8); txt != "" {
			out.WriteString("\n[LSP: errores tras editar " + filepath.Base(f) + "]\n" + txt)
		}
	}
	return out.String()
}
