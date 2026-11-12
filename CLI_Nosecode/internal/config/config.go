// Package config carga y guarda la configuracion local de Nosecode.
//
// Vive en %USERPROFILE%\.nosecode\config.json (Windows) o
// ~/.nosecode/config.json (Linux/macOS). Se puede sobreescribir con NOSECODE_HOME.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const (
	CurrentVersion       = 2
	DefaultContextWindow = 131072
	DefaultCompactKeep   = 6
	DefaultMaxSteps      = 30
	MaxStepsLimit        = 100
)

// ToolSettings controla que herramientas se envían al modelo. Las nativas
// siempre están activas; navegador y MCP se activan a mano con /tools.
type ToolSettings struct {
	Browser     bool `json:"browser"`
	MCP         bool `json:"mcp"`
	CompactKeep int  `json:"compact_keep"`
	MaxSteps    int  `json:"max_steps"`
}

// Provider describe un endpoint compatible con la API de OpenAI.
type Provider struct {
	Type          string `json:"type"`
	BaseURL       string `json:"base_url"`
	APIKey        string `json:"api_key"`
	Model         string `json:"model"`
	ContextWindow int    `json:"context_window"`
}

// Permissions define que puede hacer el agente sin preguntar.
// Valores: "ask", "allow", "deny".
type Permissions struct {
	Bash     string `json:"bash"`
	Edit     string `json:"edit"`
	Webfetch string `json:"webfetch"`
	MCP      string `json:"mcp"`
}

// MCPServer describe un servidor MCP local (stdio) o remoto (HTTP).
type MCPServer struct {
	Type        string            `json:"type,omitempty"` // local | remote
	Command     []string          `json:"command,omitempty"`
	Cwd         string            `json:"cwd,omitempty"`
	Environment map[string]string `json:"environment,omitempty"`
	URL         string            `json:"url,omitempty"`
	Headers     map[string]string `json:"headers,omitempty"`
	Enabled     *bool             `json:"enabled,omitempty"`
	Timeout     int               `json:"timeout,omitempty"` // milisegundos
}

// MCP guarda los servidores de herramientas del agente.
type MCP struct {
	Enabled bool                 `json:"enabled"`
	Servers map[string]MCPServer `json:"servers"`
}

// UI guarda preferencias visuales.
type UI struct {
	Theme      string `json:"theme"`
	Sidebar    bool   `json:"sidebar"`
	Animations bool   `json:"animations"`
	Tokens     bool   `json:"tokens"`
	Mute       bool   `json:"mute"`
	Mode       string `json:"mode"`
}

// Config es el archivo completo.
type Config struct {
	Version         int                 `json:"version"`
	DefaultProvider string              `json:"default_provider"`
	Providers       map[string]Provider `json:"providers"`
	Permissions     Permissions         `json:"permissions"`
	UI              UI                  `json:"ui"`
	Tools           ToolSettings        `json:"tools"`
	MCP             MCP                 `json:"mcp"`

	// Path es la ruta desde la que se cargo (no se escribe en el JSON).
	Path string `json:"-"`
}

// Home devuelve la carpeta de datos de Nosecode.
func Home() string {
	if h := strings.TrimSpace(os.Getenv("NOSECODE_HOME")); h != "" {
		return h
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return ".nosecode"
	}
	return filepath.Join(dir, ".nosecode")
}

// File es la ruta del config.json.
func File() string { return filepath.Join(Home(), "config.json") }

// SessionsDir es la carpeta donde viviran las sesiones (Fase 2).
func SessionsDir() string { return filepath.Join(Home(), "sessions") }

// VaultHome devuelve la carpeta del cliente Dev Vault.
func VaultHome() string {
	if h := strings.TrimSpace(os.Getenv("VAULT_HOME")); h != "" {
		return h
	}
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, ".dev-vault")
}

// Defaults devuelve la configuracion de fabrica, sin tocar el disco.
func Defaults() *Config {
	return &Config{
		Version:         CurrentVersion,
		DefaultProvider: "vault",
		Providers: map[string]Provider{
			"vault": {Type: "openai"},
		},
		Permissions: Permissions{Bash: "ask", Edit: "ask", Webfetch: "ask", MCP: "ask"},
		Tools:       ToolSettings{CompactKeep: DefaultCompactKeep, MaxSteps: DefaultMaxSteps},
		MCP:         MCP{Servers: map[string]MCPServer{}},
		UI: UI{
			Theme:      "vercel",
			Sidebar:    true,
			Animations: true,
			Tokens:     true,
			Mute:       false,
			Mode:       "build",
		},
		Path: File(),
	}
}

// Load lee la configuracion. Si no existe la crea, importando el proveedor
// del Vault cuando esta disponible.
func Load() (*Config, error) {
	cfg := Defaults()
	raw, err := os.ReadFile(cfg.Path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("leyendo %s: %w", cfg.Path, err)
		}
		if p, ok := ImportVault(); ok {
			cfg.Providers["vault"] = p
		}
		if err := cfg.Save(); err != nil {
			return nil, fmt.Errorf("creando %s: %w", cfg.Path, err)
		}
		return cfg, nil
	}

	if err := json.Unmarshal(raw, cfg); err != nil {
		return nil, fmt.Errorf("%s no es JSON valido: %w", cfg.Path, err)
	}
	cfg.Path = File()
	cfg.normalize()
	return cfg, nil
}

// Save escribe la configuracion con permisos restringidos.
func (c *Config) Save() error {
	if c.Path == "" {
		c.Path = File()
	}
	if err := os.MkdirAll(filepath.Dir(c.Path), 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(c, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	if err := os.WriteFile(c.Path, b, 0o600); err != nil {
		return err
	}
	_ = os.Chmod(c.Path, 0o600)
	return nil
}

func (c *Config) normalize() {
	if c.Version < 2 {
		// Migracion a la interfaz nueva: tema Vercel y barra lateral visible.
		if c.UI.Theme == "" || c.UI.Theme == "opencode-dark" || c.UI.Theme == "nosecode-dark" {
			c.UI.Theme = "vercel"
		}
		c.UI.Sidebar = true
		c.Version = CurrentVersion
	}
	if c.Version == 0 {
		c.Version = CurrentVersion
	}
	if c.Providers == nil {
		c.Providers = map[string]Provider{}
	}
	if c.DefaultProvider == "" {
		c.DefaultProvider = "vault"
	}
	if _, ok := c.Providers[c.DefaultProvider]; !ok {
		c.Providers[c.DefaultProvider] = Provider{Type: "openai"}
	}
	for name, p := range c.Providers {
		if p.Type == "" {
			p.Type = "openai"
			c.Providers[name] = p
		}
	}
	if c.UI.Theme == "" || c.UI.Theme == "opencode-dark" {
		c.UI.Theme = "vercel"
	}
	if c.Permissions.Bash == "" {
		c.Permissions.Bash = "ask"
	}
	if c.Permissions.Edit == "" {
		c.Permissions.Edit = "ask"
	}
	if c.Permissions.Webfetch == "" {
		c.Permissions.Webfetch = "ask"
	}
	if c.Permissions.MCP == "" {
		c.Permissions.MCP = "ask"
	}
	if c.Tools.CompactKeep <= 0 {
		c.Tools.CompactKeep = DefaultCompactKeep
	}
	if c.Tools.MaxSteps <= 0 {
		c.Tools.MaxSteps = DefaultMaxSteps
	}
	if c.Tools.MaxSteps > MaxStepsLimit {
		c.Tools.MaxSteps = MaxStepsLimit
	}
	if c.MCP.Servers == nil {
		c.MCP.Servers = map[string]MCPServer{}
	}
	for name, s := range c.MCP.Servers {
		if s.Type == "" {
			if s.URL != "" {
				s.Type = "remote"
			} else {
				s.Type = "local"
			}
			c.MCP.Servers[name] = s
		}
	}
	if c.UI.Mode != "plan" && c.UI.Mode != "build" {
		c.UI.Mode = "build"
	}
}

// Active devuelve el proveedor por defecto (o el primero disponible).
func (c *Config) Active() (string, Provider) {
	if p, ok := c.Providers[c.DefaultProvider]; ok {
		return c.DefaultProvider, p
	}
	for name, p := range c.Providers {
		return name, p
	}
	return "vault", Provider{Type: "openai"}
}

// Provider busca un proveedor por nombre.
func (c *Config) Provider(name string) (Provider, bool) {
	p, ok := c.Providers[name]
	return p, ok
}

// NormalizeBaseURL limpia espacios y barras finales.
func NormalizeBaseURL(s string) string {
	s = strings.TrimSpace(s)
	s = strings.TrimRight(s, "/")
	return s
}

// MaskKey oculta una API key dejando pistas de su longitud.
func MaskKey(k string) string {
	k = strings.TrimSpace(k)
	if k == "" {
		return "sin configurar"
	}
	if len(k) <= 8 {
		return strings.Repeat("•", len(k))
	}
	return k[:4] + strings.Repeat("•", 8) + k[len(k)-4:]
}

// ImportVault lee ~/.dev-vault/config.json y state.json para precargar el
// proveedor del Vault (URL de ngrok + API key + modelo).
func ImportVault() (Provider, bool) {
	dir := VaultHome()
	if dir == "" {
		return Provider{}, false
	}

	var (
		p  = Provider{Type: "openai"}
		ok bool
	)

	if raw, err := os.ReadFile(filepath.Join(dir, "config.json")); err == nil {
		var vc struct {
			APIKey string `json:"api_key"`
		}
		if json.Unmarshal(raw, &vc) == nil && strings.TrimSpace(vc.APIKey) != "" {
			p.APIKey = strings.TrimSpace(vc.APIKey)
			ok = true
		}
	}

	if raw, err := os.ReadFile(filepath.Join(dir, "state.json")); err == nil {
		var vs struct {
			URL   string `json:"url"`
			Model string `json:"model"`
		}
		if json.Unmarshal(raw, &vs) == nil {
			if u := NormalizeBaseURL(vs.URL); u != "" {
				p.BaseURL = u + "/v1"
				ok = true
			}
			if m := strings.TrimSpace(vs.Model); m != "" {
				p.Model = m
			}
		}
	}

	if !ok {
		return Provider{}, false
	}
	return p, true
}

// SyncVault relee ~/.dev-vault/state.json y actualiza SOLO el proveedor vault.
// Devuelve true y la URL nueva si hubo cambios.
func (c *Config) SyncVault() (bool, string) {
	p, ok := ImportVault()
	if !ok || strings.TrimSpace(p.BaseURL) == "" {
		return false, ""
	}
	cur, exists := c.Providers["vault"]
	if !exists {
		return false, ""
	}

	base := NormalizeBaseURL(p.BaseURL)
	changed := NormalizeBaseURL(cur.BaseURL) != base
	cur.BaseURL = base
	if p.APIKey != "" && p.APIKey != cur.APIKey {
		cur.APIKey = p.APIKey
		changed = true
	}
	if p.Model != "" && p.Model != cur.Model {
		cur.Model = p.Model
		changed = true
	}
	if cur.Type == "" {
		cur.Type = "openai"
		changed = true
	}
	if !changed {
		return false, ""
	}
	c.Providers["vault"] = cur
	return true, base
}

// OpenCodeConfig devuelve la ruta del opencode.json global.
func OpenCodeConfig() string {
	dir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, ".config", "opencode", "opencode.json")
}

// ImportMCP copia los servidores MCP configurados en OpenCode al config de
// Nosecode. No toca el archivo de OpenCode.
func (c *Config) ImportMCP() (int, string, error) {
	path := OpenCodeConfig()
	raw, err := os.ReadFile(path)
	if err != nil {
		return 0, path, fmt.Errorf("no encontré %s: %w", path, err)
	}
	var root struct {
		MCP struct {
			Servers map[string]MCPServer `json:"servers"`
		} `json:"mcp"`
		MCPServers map[string]MCPServer `json:"mcpServers"`
	}
	if err := json.Unmarshal(raw, &root); err != nil {
		return 0, path, fmt.Errorf("opencode.json no es JSON válido: %w", err)
	}
	servers := root.MCP.Servers
	if len(servers) == 0 {
		servers = root.MCPServers
	}
	if len(servers) == 0 {
		return 0, path, errors.New("no hay servidores MCP en opencode.json")
	}
	if c.MCP.Servers == nil {
		c.MCP.Servers = map[string]MCPServer{}
	}
	for name, s := range servers {
		if s.Type == "" {
			if s.URL != "" {
				s.Type = "remote"
			} else {
				s.Type = "local"
			}
		}
		c.MCP.Servers[name] = s
	}
	c.MCP.Enabled = true
	if err := c.Save(); err != nil {
		return 0, path, err
	}
	return len(servers), path, nil
}

// ImportedServerEnabled indica si un servidor importado debe arrancar.
func (s MCPServer) ImportedServerEnabled() bool {
	return s.Enabled == nil || *s.Enabled
}
