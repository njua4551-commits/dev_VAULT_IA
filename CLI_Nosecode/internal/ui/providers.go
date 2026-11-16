package ui

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"

	"nosecode/internal/config"
)

// Catálogo de proveedores conocidos (ChatGPT, Claude, Gemini, DeepSeek…).
// Elegir uno abre el formulario de proveedor ya relleno; solo falta la clave.

type catalogState struct {
	cursor int
}

// cloudProvider: el proveedor activo es un servicio en la nube conocido.
func (m Model) cloudProvider() bool {
	_, p := m.cfg.Active()
	return config.IsCloudURL(p.BaseURL)
}

// localParams: max_tokens calculado y enable_thinking son para llama.cpp; los
// servicios en la nube son estrictos y a veces los rechazan.
func (m Model) localMaxTokens() int {
	if m.cloudProvider() {
		return 0
	}
	return m.maxOutputTokens()
}

func (m Model) localThinking() *bool {
	if m.cloudProvider() {
		return nil
	}
	return m.thinkingSetting()
}

func (m Model) openCatalog() Model {
	m.catalog = catalogState{}
	m.screen = screenCatalog
	return m
}

func (m Model) catalogKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	n := len(config.Presets)
	switch k.String() {
	case "esc", "q":
		m.screen = screenChat
		return m, nil
	case "up", "k":
		m.catalog.cursor = (m.catalog.cursor + n - 1) % n
	case "down", "j", "tab":
		m.catalog.cursor = (m.catalog.cursor + 1) % n
	case "enter":
		return m.startEditPreset(config.Presets[m.catalog.cursor]), nil
	}
	return m, nil
}

// startEditPreset abre el formulario de proveedor con los datos del preset.
func (m Model) startEditPreset(p config.Preset) Model {
	m = m.openSettings(0)
	name := p.ID
	_, exists := m.cfg.Providers[name]
	m = m.startEdit(name, !exists)
	set := func(i int, v string) {
		if i < len(m.settings.fields) {
			m.settings.fields[i].SetValue(v)
		}
	}
	if !exists {
		set(1, p.BaseURL)
		set(3, p.Model)
		set(4, fmt.Sprint(p.Context))
	}
	if len(m.settings.fields) > 2 {
		m.settings.fields[m.settings.field].Blur()
		m.settings.field = 2
		m.settings.fields[2].Focus()
	}
	hint := "Pega tu clave API"
	if p.KeyURL != "" {
		hint += " (consíguela en " + p.KeyURL + ")"
	}
	m.toast, m.toastErr = hint+" · ctrl+s guarda", false
	return m
}

// maskKeyField oculta la clave API mientras se escribe.
func maskKeyField(fields []textinput.Model) {
	if len(fields) > 2 {
		fields[2].EchoMode = textinput.EchoPassword
		fields[2].EchoCharacter = '•'
	}
}

func (m Model) catalogScreen() []uiLine {
	boxW := clamp(m.width-8, 56, 96)
	if boxW > m.width-2 {
		boxW = m.width - 2
	}
	innerW := boxW - 4
	head := []uiLine{{text: modalTitleStyle.Render("Catálogo de IAs") + "  " + faintStyle.Render("enter elige · esc cierra")}}
	var body []uiLine
	for i, p := range config.Presets {
		tag := faintStyle.Render("de pago ")
		switch {
		case p.Local:
			tag = successStyle.Render("local   ")
		case p.Free:
			tag = successStyle.Render("gratis  ")
		}
		line := " " + tag + textStyle.Render(fit(p.Name, 22))
		note := fit(p.Note, innerW-34)
		if note != "" {
			line += "  " + mutedStyle.Render(note)
		}
		if i == m.catalog.cursor {
			line = selectRow(line, innerW)
		} else {
			line = pad(line, innerW)
		}
		body = append(body, uiLine{text: line, hits: []relHit{{x: 0, w: innerW, action: "catalogPick", arg: fmt.Sprint(i)}}})
	}
	sel := config.Presets[m.catalog.cursor]
	body = append(body, uiLine{}, uiLine{text: faintStyle.Render(fit(sel.BaseURL+" · "+sel.Model, innerW))})
	if sel.KeyURL != "" {
		body = append(body, uiLine{text: faintStyle.Render(fit("clave: "+sel.KeyURL, innerW))})
	}
	body = append(body, uiLine{text: faintStyle.Render("El modelo es editable: los nombres cambian seguido.")})
	return m.centerModal(m.modalFrame(boxW, head, body, "↑↓ mover · enter configurar · esc cerrar"), boxW)
}

// providersSlash: /providers abre el catálogo; /providers <id> lo abre ya elegido.
func (m Model) providersSlash(rest string) Model {
	arg := strings.TrimSpace(rest)
	if arg == "" {
		return m.openCatalog()
	}
	if p, ok := config.PresetByID(arg); ok {
		return m.startEditPreset(p)
	}
	m.toast, m.toastErr = "No conozco «"+arg+"». Usa /providers para ver la lista.", true
	return m
}
