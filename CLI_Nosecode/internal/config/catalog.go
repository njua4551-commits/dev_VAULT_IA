package config

import (
	"net/url"
	"strings"
)

// Preset describe un proveedor conocido con API compatible con OpenAI. Los
// modelos cambian a menudo: el valor es solo un punto de partida editable.
type Preset struct {
	ID      string // nombre del proveedor en la config (sin espacios)
	Name    string
	BaseURL string
	Model   string
	Context int
	KeyURL  string // dónde conseguir la clave
	Free    bool   // tiene plan gratis (con límites)
	Local   bool   // corre en tu equipo
	Note    string
}

// Presets es el catálogo que se ofrece en Ajustes → Proveedores y en /providers.
var Presets = []Preset{
	{ID: "gemini", Name: "Google Gemini", BaseURL: "https://generativelanguage.googleapis.com/v1beta/openai", Model: "gemini-2.5-flash",
		Context: 1000000, KeyURL: "https://aistudio.google.com/apikey", Free: true,
		Note: "plan gratis con límites; contexto enorme"},
	{ID: "claude", Name: "Anthropic Claude", BaseURL: "https://api.anthropic.com/v1", Model: "claude-sonnet-5-5",
		Context: 200000, KeyURL: "https://platform.claude.com/", Note: "de pago; capa de compatibilidad OpenAI"},
	{ID: "openai", Name: "OpenAI (ChatGPT)", BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini",
		Context: 128000, KeyURL: "https://platform.openai.com/api-keys", Note: "de pago"},
	{ID: "deepseek", Name: "DeepSeek", BaseURL: "https://api.deepseek.com/v1", Model: "deepseek-chat",
		Context: 128000, KeyURL: "https://platform.deepseek.com/api_keys", Note: "muy barato"},
	{ID: "openrouter", Name: "OpenRouter", BaseURL: "https://openrouter.ai/api/v1", Model: "openrouter/auto",
		Context: 128000, KeyURL: "https://openrouter.ai/keys", Free: true,
		Note: "un solo acceso a muchos modelos; los que acaban en :free son gratis"},
	{ID: "groq", Name: "Groq", BaseURL: "https://api.groq.com/openai/v1", Model: "llama-3.3-70b-versatile",
		Context: 128000, KeyURL: "https://console.groq.com/keys", Free: true,
		Note: "gratis y muy rápido; límite bajo de tokens por minuto"},
	{ID: "cerebras", Name: "Cerebras", BaseURL: "https://api.cerebras.ai/v1", Model: "llama-3.3-70b",
		Context: 8192, KeyURL: "https://cloud.cerebras.ai/", Free: true,
		Note: "gratis; el contexto gratis es corto (~8K)"},
	{ID: "github", Name: "GitHub Models", BaseURL: "https://models.github.ai/inference", Model: "openai/gpt-4.1",
		Context: 128000, KeyURL: "https://github.com/settings/personal-access-tokens", Free: true,
		Note: "gratis con límites; usa un token de GitHub con permiso models"},
	{ID: "mistral", Name: "Mistral", BaseURL: "https://api.mistral.ai/v1", Model: "mistral-large-latest",
		Context: 128000, KeyURL: "https://console.mistral.ai/api-keys", Free: true, Note: "nivel Experiment gratis"},
	{ID: "xai", Name: "xAI Grok", BaseURL: "https://api.x.ai/v1", Model: "grok-code-fast-1",
		Context: 256000, KeyURL: "https://console.x.ai/", Note: "de pago"},
	{ID: "nvidia", Name: "NVIDIA build", BaseURL: "https://integrate.api.nvidia.com/v1", Model: "meta/llama-3.3-70b-instruct",
		Context: 128000, KeyURL: "https://build.nvidia.com/", Free: true, Note: "créditos gratis al registrarte"},
	{ID: "ollama", Name: "Ollama (local)", BaseURL: "http://localhost:11434/v1", Model: "qwen2.5-coder",
		Context: 32768, Free: true, Local: true, Note: "en tu PC; la clave puede ser cualquier texto"},
	{ID: "lmstudio", Name: "LM Studio (local)", BaseURL: "http://localhost:1234/v1", Model: "local-model",
		Context: 32768, Free: true, Local: true, Note: "en tu PC; la clave puede ser cualquier texto"},
}

// PresetByID busca un preset por su id.
func PresetByID(id string) (Preset, bool) {
	for _, p := range Presets {
		if p.ID == strings.ToLower(strings.TrimSpace(id)) {
			return p, true
		}
	}
	return Preset{}, false
}

// IsCloudURL dice si la URL es de un servicio en la nube conocido. Esos
// servicios son estrictos con los parámetros: no se les mandan extras de
// llama.cpp (chat_template_kwargs) ni max_tokens calculado.
func IsCloudURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	for _, p := range Presets {
		if p.Local {
			continue
		}
		if pu, err := url.Parse(p.BaseURL); err == nil && strings.EqualFold(pu.Hostname(), host) {
			return true
		}
	}
	return false
}
