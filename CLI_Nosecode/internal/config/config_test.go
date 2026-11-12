package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaults(t *testing.T) {
	c := Defaults()
	if c.Version != CurrentVersion {
		t.Fatalf("version = %d", c.Version)
	}
	if c.DefaultProvider != "vault" {
		t.Fatalf("provider por defecto = %q", c.DefaultProvider)
	}
	if _, ok := c.Providers["vault"]; !ok {
		t.Fatal("falta el proveedor vault")
	}
	if c.Permissions.Bash != "ask" {
		t.Fatalf("bash = %q", c.Permissions.Bash)
	}
	if !c.UI.Tokens {
		t.Fatal("el contador de tokens debe venir activo")
	}
}

func TestMaxStepsDefaultYLimite(t *testing.T) {
	c := Defaults()
	if c.Tools.MaxSteps != DefaultMaxSteps {
		t.Fatalf("max_steps por defecto = %d", c.Tools.MaxSteps)
	}
	if DefaultMaxSteps != 30 || MaxStepsLimit != 100 {
		t.Fatalf("constantes max_steps = %d/%d", DefaultMaxSteps, MaxStepsLimit)
	}
	c.Tools.MaxSteps = 0
	c.normalize()
	if c.Tools.MaxSteps != DefaultMaxSteps {
		t.Fatalf("max_steps tras normalizar = %d", c.Tools.MaxSteps)
	}
	c.Tools.MaxSteps = MaxStepsLimit + 50
	c.normalize()
	if c.Tools.MaxSteps != MaxStepsLimit {
		t.Fatalf("max_steps sin tope = %d", c.Tools.MaxSteps)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NOSECODE_HOME", home)
	t.Setenv("VAULT_HOME", filepath.Join(home, "vault-vacio"))

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cfg.Path); err != nil {
		t.Fatalf("no se creo el config: %v", err)
	}

	cfg.Providers["local"] = Provider{Type: "openai", BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen"}
	cfg.DefaultProvider = "local"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	got, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got.DefaultProvider != "local" {
		t.Fatalf("provider = %q", got.DefaultProvider)
	}
	if got.Providers["local"].BaseURL != "http://127.0.0.1:11434/v1" {
		t.Fatalf("base_url = %q", got.Providers["local"].BaseURL)
	}
}

func TestImportVault(t *testing.T) {
	home := t.TempDir()
	vault := filepath.Join(home, "vault")
	if err := os.MkdirAll(vault, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "config.json"),
		[]byte(`{"api_key":"sk-secreto-1234567890"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "state.json"),
		[]byte(`{"url":"https://abc.ngrok.app/","model":"Qwen3.8-27B"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv("VAULT_HOME", vault)
	t.Setenv("NOSECODE_HOME", filepath.Join(home, "nosecode"))

	p, ok := ImportVault()
	if !ok {
		t.Fatal("ImportVault no detecto el Vault")
	}
	if p.BaseURL != "https://abc.ngrok.app/v1" {
		t.Fatalf("base_url = %q", p.BaseURL)
	}
	if p.APIKey != "sk-secreto-1234567890" {
		t.Fatalf("api_key = %q", p.APIKey)
	}
	if p.Model != "Qwen3.8-27B" {
		t.Fatalf("model = %q", p.Model)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Providers["vault"].BaseURL != "https://abc.ngrok.app/v1" {
		t.Fatal("el config inicial no importo el Vault")
	}
}

func TestMaskKey(t *testing.T) {
	cases := map[string]string{
		"":                         "sin configurar",
		"abc":                      "•••",
		"api-key-1234567890abcdef": "api-••••••••cdef",
	}
	for in, want := range cases {
		if got := MaskKey(in); got != want {
			t.Fatalf("MaskKey(%q) = %q, queria %q", in, got, want)
		}
	}
}

func TestNormalizeBaseURL(t *testing.T) {
	if got := NormalizeBaseURL(" https://x.ngrok.app/ "); got != "https://x.ngrok.app" {
		t.Fatalf("got %q", got)
	}
}

func TestThemeMigration(t *testing.T) {
	home := t.TempDir()
	t.Setenv("NOSECODE_HOME", home)
	t.Setenv("VAULT_HOME", filepath.Join(home, "vault-vacio"))
	if err := os.MkdirAll(home, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(home, "config.json"),
		[]byte(`{"version":1,"ui":{"theme":"opencode-dark"}}`), 0o600); err != nil {
		t.Fatal(err)
	}

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.UI.Theme != "vercel" {
		t.Fatalf("tema = %q, queria vercel", cfg.UI.Theme)
	}
	if !cfg.UI.Sidebar {
		t.Fatal("la migracion deberia activar la barra lateral")
	}
	if cfg.Version != CurrentVersion {
		t.Fatalf("version = %d, queria %d", cfg.Version, CurrentVersion)
	}
}

func TestSyncVaultActualizaSoloVault(t *testing.T) {
	vault := t.TempDir()
	if err := os.WriteFile(filepath.Join(vault, "config.json"), []byte(`{"api_key":"k1"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(vault, "state.json"), []byte(`{"url":"https://nuevo.ngrok-free.dev","model":"Qwen3.8-27B"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("VAULT_HOME", vault)

	c := Defaults()
	c.Providers["vault"] = Provider{Type: "openai", BaseURL: "https://viejo.ngrok-free.dev/v1", APIKey: "k1", Model: "viejo"}
	c.Providers["otro"] = Provider{BaseURL: "https://otro/v1"}

	changed, url := c.SyncVault()
	if !changed || url != "https://nuevo.ngrok-free.dev/v1" {
		t.Fatalf("changed=%v url=%q", changed, url)
	}
	if got := c.Providers["vault"]; got.BaseURL != url || got.Model != "Qwen3.8-27B" {
		t.Fatalf("vault = %+v", got)
	}
	if got := c.Providers["otro"].BaseURL; got != "https://otro/v1" {
		t.Fatalf("otro proveedor cambio: %q", got)
	}
	if changed, _ := c.SyncVault(); changed {
		t.Fatal("la segunda llamada no debia cambiar nada")
	}
}
