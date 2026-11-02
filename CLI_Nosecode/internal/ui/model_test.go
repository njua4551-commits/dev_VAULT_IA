package ui

import (
	"strings"
	"testing"

	"nosecode/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func newTestModel(t *testing.T) Model {
	t.Helper()
	t.Setenv("NOSECODE_HOME", t.TempDir())
	t.Setenv("VAULT_HOME", t.TempDir())
	m := New(config.Defaults())
	updated, _ := m.Update(tea.WindowSizeMsg{Width: 110, Height: 34})
	model, ok := updated.(Model)
	if !ok {
		t.Fatal("Update devolvio un tipo inesperado")
	}
	return model
}

func TestChatViewRenders(t *testing.T) {
	m := newTestModel(t)
	out := m.View()
	if out == "" {
		t.Fatal("View devolvio vacio")
	}
	if !strings.Contains(out, "█") {
		t.Fatal("falta el logo en bloques")
	}
	if !strings.Contains(out, "ctrl+p comandos") || !strings.Contains(out, "Tip") {
		t.Fatal("faltan las pistas de la pantalla de inicio")
	}
	if !strings.Contains(out, "╹▀") {
		t.Fatal("falta el borde inferior del prompt")
	}
}

func TestSubmitOffline(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("hola")
	model, _ := m.submit()
	got, ok := model.(Model)
	if !ok {
		t.Fatal("submit devolvio un tipo inesperado")
	}
	if len(got.msgs) != 2 {
		t.Fatalf("mensajes = %d, queria 2", len(got.msgs))
	}
	if !strings.Contains(got.msgs[1].Text, "endpoint") {
		t.Fatalf("respuesta offline inesperada: %q", got.msgs[1].Text)
	}
	if got.View() == "" {
		t.Fatal("View vacio tras enviar")
	}
}

func TestSettingsAndPaletteRender(t *testing.T) {
	m := newTestModel(t)
	s := m.openSettings(0)
	if s.View() == "" {
		t.Fatal("settings renderizo vacio")
	}
	if !strings.Contains(s.View(), "Configuración") || !strings.Contains(s.View(), "Apariencia") {
		t.Fatal("faltan las secciones de ajustes")
	}

	p := m.openPalette()
	if p.View() == "" {
		t.Fatal("paleta renderizo vacia")
	}
	if len(p.palette.filtered) != len(p.palette.items) {
		t.Fatal("la paleta deberia mostrar todos los comandos al abrirse")
	}
	if !strings.Contains(p.View(), "Sesión") || !strings.Contains(p.View(), "Ajustes") {
		t.Fatal("faltan las categorias de la paleta")
	}
}

func TestSettingsSearchAndActions(t *testing.T) {
	m := newTestModel(t)
	m = m.openSettings(0)
	m.settings.query = "sonido"
	rows := m.filteredSettingRows()
	found := false
	for _, r := range rows {
		if r.kind == "toggle" && r.arg == "sound" {
			found = true
		}
		if r.kind == "theme" {
			t.Fatal("el filtro no deberia mostrar el tema")
		}
	}
	if !found {
		t.Fatal("el filtro no encontro Sonidos")
	}

	before := m.cfg.UI.Mute
	m = m.toggleOption("sound")
	if m.cfg.UI.Mute == before {
		t.Fatal("toggleOption no cambio los sonidos")
	}

	perm := m.cyclePermission("bash")
	if perm.cfg.Permissions.Bash != "allow" {
		t.Fatalf("permiso bash = %q", perm.cfg.Permissions.Bash)
	}
}

func TestSessionsPersistAcrossRestart(t *testing.T) {
	m := newTestModel(t)
	m.input.SetValue("mensaje persistente")
	model, _ := m.submit()
	saved := model.(Model)

	reloaded := newModel(saved.cfg, true)
	if len(reloaded.msgs) != 2 {
		t.Fatalf("mensajes recargados = %d, queria 2", len(reloaded.msgs))
	}
	if reloaded.msgs[0].Text != "mensaje persistente" {
		t.Fatalf("mensaje recargado inesperado: %q", reloaded.msgs[0].Text)
	}
	if reloaded.activeTitle() != "mensaje persistente" {
		t.Fatalf("titulo recargado = %q", reloaded.activeTitle())
	}

	second := saved.newSession()
	if len(second.sessions) != 2 {
		t.Fatalf("sesiones tras crear = %d, queria 2", len(second.sessions))
	}
	if again := newModel(second.cfg, true); len(again.sessions) != 2 {
		t.Fatalf("sesiones recargadas = %d, queria 2", len(again.sessions))
	}
}

func TestSessionsInSidebar(t *testing.T) {
	m := newTestModel(t)
	m = m.newSession()
	m.sessions[0].Title = "Primera charla"
	m.sessions[len(m.sessions)-1].Title = "Segunda charla"
	m.msgs = append(m.msgs, Message{Role: "user", Text: "hola"})
	if !m.sidebarVisible() {
		t.Fatal("la barra lateral deberia verse a 110 columnas")
	}
	view := m.View()
	if !strings.Contains(view, "Primera charla") || !strings.Contains(view, "Segunda charla") {
		t.Fatal("las sesiones no aparecen en la barra lateral")
	}

	hasSwitch, hasNew, hasClose := false, false, false
	for _, b := range m.hit.boxes {
		switch b.action {
		case "sessionSwitch":
			hasSwitch = true
		case "new":
			hasNew = true
		case "sessionClose":
			hasClose = true
		}
	}
	if !hasSwitch || !hasNew || !hasClose {
		t.Fatal("faltan zonas clicables en la barra lateral")
	}

	// un clic en una sesion inactiva la abre
	for _, b := range m.hit.boxes {
		if b.action == "sessionSwitch" && b.arg == m.sessions[0].ID {
			model, _ := m.mouse(tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: b.x + 1, Y: b.y})
			if got := model.(Model); got.activeID != m.sessions[0].ID {
				t.Fatal("el clic no cambio de sesion")
			}
			return
		}
	}
	t.Fatal("no hay zona clicable para la primera sesion")
}

func TestPaletteListsSessions(t *testing.T) {
	m := newTestModel(t)
	m = m.newSession()
	p := m.openPalette()
	count := 0
	for _, it := range p.palette.items {
		if it.category == "Sesiones" {
			count++
		}
	}
	if count != len(m.sessions) {
		t.Fatalf("sesiones en la paleta = %d, queria %d", count, len(m.sessions))
	}
	s := m.slash("/sessions")
	if s.screen != screenPalette || len(s.palette.filtered) != count {
		t.Fatalf("/sessions no filtro la paleta (%d resultados)", len(s.palette.filtered))
	}
}

func TestPromptBoxLayout(t *testing.T) {
	m := newTestModel(t)
	box := m.promptBox(60)
	if len(box) != inputHeight {
		t.Fatalf("alto del prompt = %d, queria %d", len(box), inputHeight)
	}
	for _, ln := range box {
		if w := lipgloss.Width(ln.text); w > 60 {
			t.Fatalf("linea del prompt de %d columnas", w)
		}
	}
	hasMode, hasFocus := false, false
	for _, ln := range box {
		for _, h := range ln.hits {
			hasMode = hasMode || h.action == "mode"
			hasFocus = hasFocus || h.action == "focusInput"
		}
	}
	if !hasMode || !hasFocus {
		t.Fatal("el prompt debe tener zonas para el modo y el foco")
	}
}

func TestPaintHelpers(t *testing.T) {
	if got := stripBg("\x1b[48;2;1;2;3mhola\x1b[0m"); got != "hola\x1b[0m" {
		t.Fatalf("stripBg 24 bits = %q", got)
	}
	if got := stripBg("\x1b[1;47;38;5;45mx"); got != "\x1b[1;38;5;45mx" {
		t.Fatalf("stripBg debe conservar el color de texto 38;5;45: %q", got)
	}
	if got := stripBg("\x1b[48;5;240mx"); got != "x" {
		t.Fatalf("stripBg 256 colores = %q", got)
	}
	if got := paint(colPanel, "ab"); !strings.Contains(got, "ab") {
		t.Fatalf("paint perdio el texto: %q", got)
	}
}

func TestSlashCommands(t *testing.T) {
	m := newTestModel(t)
	help := m.slash("/help")
	if len(help.msgs) == 0 {
		t.Fatal("/help no agrego mensajes")
	}
	before := m.cfg.UI.Tokens
	toggled := m.slash("/tokens")
	if toggled.cfg.UI.Tokens == before {
		t.Fatal("/tokens no cambio el estado")
	}
	if cmd := m.slash("/quit"); !cmd.quit {
		t.Fatal("/quit no marco la salida")
	}
}

func TestThemePaletteAndMigration(t *testing.T) {
	p := PaletteByName("vercel")
	if p.Bg != "#000000" || p.Primary != "#0070F3" || p.Text != "#EDEDED" {
		t.Fatalf("paleta vercel inesperada: %+v", p)
	}
	if PaletteByName("").Name != "vercel" {
		t.Fatal("el tema por defecto debe ser vercel")
	}
}

func TestMouseClickHits(t *testing.T) {
	m := newTestModel(t)
	m = m.openSettings(0)
	view := m.View()
	if view == "" {
		t.Fatal("settings renderizo vacio")
	}
	if len(m.hit.boxes) == 0 {
		t.Fatal("no se registraron zonas clicables")
	}

	var newBtn hitBox
	found := false
	for _, b := range m.hit.boxes {
		if b.action == "providerNew" {
			newBtn, found = b, true
			break
		}
	}
	if !found {
		t.Fatal("falta el boton Nuevo proveedor")
	}
	click := tea.MouseMsg{Action: tea.MouseActionPress, Button: tea.MouseButtonLeft, X: newBtn.x + 1, Y: newBtn.y}
	model, _ := m.mouse(click)
	got := model.(Model)
	if !got.settings.editing || !got.settings.isNew {
		t.Fatal("el clic en Nuevo no abrio el formulario")
	}
}

func TestRenderSizes(t *testing.T) {
	for _, size := range [][2]int{{80, 24}, {120, 40}, {60, 20}, {100, 30}, {44, 14}} {
		m := newTestModel(t)
		updated, _ := m.Update(tea.WindowSizeMsg{Width: size[0], Height: size[1]})
		sm := updated.(Model)
		chat := sm
		chat.msgs = []Message{
			{Role: "user", Text: "hola, esto es un mensaje de usuario bastante largo para comprobar el ajuste de línea en cualquier ancho"},
			{Role: "assistant", Text: "respuesta de prueba con varias palabras para envolver el texto en el panel de conversación"},
		}
		chat.layout()
		for _, view := range []string{
			sm.View(),
			chat.View(),
			sm.openSettings(0).View(),
			sm.openSettings(3).View(),
			sm.openPalette().View(),
		} {
			rows := strings.Split(view, "\n")
			if len(rows) != size[1] {
				t.Fatalf("%dx%d: filas = %d", size[0], size[1], len(rows))
			}
			for _, row := range rows {
				if w := lipgloss.Width(row); w > size[0] {
					t.Fatalf("%dx%d: ancho %d", size[0], size[1], w)
				}
			}
		}
	}
}

func TestWrapAndEstimate(t *testing.T) {
	if got := estimateTokens("12345678"); got != 2 {
		t.Fatalf("estimateTokens = %d", got)
	}
	lines := wrapText("uno dos tres cuatro cinco seis", 10)
	if len(lines) < 2 {
		t.Fatalf("wrapText no partio la linea: %v", lines)
	}
	for _, ln := range lines {
		if len([]rune(ln)) > 10 {
			t.Fatalf("linea demasiado larga: %q", ln)
		}
	}
}
