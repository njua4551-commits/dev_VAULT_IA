package config

import (
	"net/url"
	"strings"
	"testing"
)

func TestCatalogoValido(t *testing.T) {
	seen := map[string]bool{}
	for _, p := range Presets {
		if p.ID == "" || strings.ContainsAny(p.ID, " /") || seen[p.ID] {
			t.Fatalf("id inválido o repetido: %q", p.ID)
		}
		seen[p.ID] = true
		u, err := url.Parse(p.BaseURL)
		if err != nil || u.Host == "" || p.Model == "" || p.Context <= 0 {
			t.Fatalf("%s incompleto: %+v", p.ID, p)
		}
		if !p.Local && u.Scheme != "https" {
			t.Fatalf("%s debe ser https", p.ID)
		}
		if !p.Local && p.KeyURL == "" {
			t.Fatalf("%s sin enlace para la clave", p.ID)
		}
	}
	for _, id := range []string{"openai", "claude", "gemini", "deepseek"} {
		if _, ok := PresetByID(id); !ok {
			t.Fatalf("falta %s", id)
		}
	}
}

func TestIsCloudURL(t *testing.T) {
	if !IsCloudURL("https://api.openai.com/v1") || !IsCloudURL("https://api.deepseek.com/v1/") {
		t.Fatal("nube conocida")
	}
	for _, u := range []string{"https://abc.ngrok-free.dev/v1", "http://localhost:11434/v1", "", "no es url"} {
		if IsCloudURL(u) {
			t.Fatalf("%q no es nube conocida", u)
		}
	}
}
