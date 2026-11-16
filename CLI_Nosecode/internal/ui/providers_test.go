package ui

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"

	"nosecode/internal/config"
)

func TestCatalogoAbreFormularioRelleno(t *testing.T) {
	m := newTestModel(t)
	m = m.openCatalog()
	if m.screen != screenCatalog || len(m.catalogScreen()) == 0 {
		t.Fatal("catálogo")
	}
	for i, p := range config.Presets {
		if p.ID == "deepseek" {
			m.catalog.cursor = i
		}
	}
	nm, _ := m.catalogKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = nm.(Model)
	if m.screen != screenSettings || !m.settings.editing || len(m.settings.fields) < 5 {
		t.Fatalf("formulario: %v %v", m.screen, m.settings.editing)
	}
	f := m.settings.fields
	if f[0].Value() != "deepseek" || f[1].Value() != "https://api.deepseek.com/v1" || f[3].Value() != "deepseek-chat" || f[4].Value() != "128000" {
		t.Fatalf("campos: %q %q %q %q", f[0].Value(), f[1].Value(), f[3].Value(), f[4].Value())
	}
	if m.settings.field != 2 {
		t.Fatalf("debe enfocar la clave, está en %d", m.settings.field)
	}
	if f[2].EchoMode == 0 {
		t.Fatal("la clave debe ocultarse")
	}
	m.settings.fields[2].SetValue("sk-secreta")
	nm2, _ := m.saveEdit()
	m = nm2.(Model)
	p, ok := m.cfg.Providers["deepseek"]
	if !ok || p.APIKey != "sk-secreta" || p.Model != "deepseek-chat" {
		t.Fatalf("no guardó: %+v", p)
	}
}

func TestNubeNoRecibeParametrosDeLlamaCpp(t *testing.T) {
	m := newTestModel(t)
	m.cfg.Providers["openai"] = config.Provider{Type: "openai", BaseURL: "https://api.openai.com/v1", APIKey: "k", Model: "gpt"}
	m.cfg.DefaultProvider = "openai"
	m.contextWin = 100000
	m.cfg.UI.Thinking = "on"
	if m.localMaxTokens() != 0 || m.localThinking() != nil {
		t.Fatal("la nube no debe recibir max_tokens ni enable_thinking")
	}
	m.cfg.Providers["vault"] = config.Provider{Type: "openai", BaseURL: "https://abc.ngrok-free.dev/v1", APIKey: "k", Model: "q"}
	m.cfg.DefaultProvider = "vault"
	if m.localMaxTokens() == 0 || m.localThinking() == nil {
		t.Fatal("el Vault sí debe recibirlos")
	}
}

func TestProvidersSlash(t *testing.T) {
	m := newTestModel(t)
	if m = m.providersSlash(""); m.screen != screenCatalog {
		t.Fatal("sin argumento abre el catálogo")
	}
	m = newTestModel(t).providersSlash("gemini")
	if !m.settings.editing || m.settings.fields[1].Value() == "" {
		t.Fatal("gemini")
	}
	m = newTestModel(t).providersSlash("nope")
	if !m.toastErr {
		t.Fatal("desconocido")
	}
}
