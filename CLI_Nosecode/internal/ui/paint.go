package ui

import (
	"regexp"
	"strconv"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Utilidades de pintado: opencode dibuja paneles con fondo propio (prompt,
// mensajes del usuario, diálogos, barra lateral). Lipgloss reinicia los
// colores tras cada segmento con estilo, así que `paint` vuelve a aplicar el
// fondo después de cada reinicio para que el panel quede continuo.

// styleSeq devuelve la secuencia ANSI que activa el estilo st ("" si el
// terminal no soporta color, p. ej. en tests o con --demo sin TTY).
func styleSeq(st lipgloss.Style) string {
	s := st.Render("X")
	i := strings.Index(s, "X")
	if i <= 0 {
		return ""
	}
	return s[:i]
}

// paintWith aplica el estilo st a toda la cadena, incluso tras estilos anidados.
func paintWith(st lipgloss.Style, s string) string {
	seq := styleSeq(st)
	if seq == "" {
		return s
	}
	return seq + strings.ReplaceAll(s, "\x1b[0m", "\x1b[0m"+seq) + "\x1b[0m"
}

// paint pinta el fondo c (con el color de texto por defecto) sobre la cadena.
func paint(c lipgloss.Color, s string) string {
	return paintWith(lipgloss.NewStyle().Background(c).Foreground(colText), s)
}

var sgrRe = regexp.MustCompile("\x1b\\[([0-9;]*)m")

// stripBg elimina los fondos de una cadena con ANSI (el textarea trae el
// suyo, que chocaría con el panel).
func stripBg(s string) string {
	return sgrRe.ReplaceAllStringFunc(s, func(seq string) string {
		body := seq[2 : len(seq)-1]
		if body == "" {
			return seq
		}
		params := strings.Split(body, ";")
		keep := make([]string, 0, len(params))
		for i := 0; i < len(params); i++ {
			n, err := strconv.Atoi(params[i])
			if err != nil {
				keep = append(keep, params[i])
				continue
			}
			switch {
			case n == 38 && i+1 < len(params):
				// color de texto 38;5;n o 38;2;r;g;b: se conserva entero
				extra := 0
				switch params[i+1] {
				case "5":
					extra = 2
				case "2":
					extra = 4
				}
				for j := 0; j <= extra && i+j < len(params); j++ {
					keep = append(keep, params[i+j])
				}
				i += extra
			case n == 48 && i+1 < len(params):
				// fondo 48;5;n o 48;2;r;g;b: se descarta entero
				switch params[i+1] {
				case "5":
					i += 2
				case "2":
					i += 4
				}
			case (n >= 40 && n <= 47) || n == 49 || (n >= 100 && n <= 107):
				// fondos simples: se descartan
			default:
				keep = append(keep, params[i])
			}
		}
		if len(keep) == 0 {
			return ""
		}
		return "\x1b[" + strings.Join(keep, ";") + "m"
	})
}

// selectRow pinta una fila seleccionada como opencode: barra con el color
// primario y texto blanco, sin estilos internos que rompan el fondo.
func selectRow(text string, w int) string {
	return selectedStyle.Render(pad(ansi.Strip(text), w))
}

// agentColor es el color del agente activo (Build/Plan), como en opencode.
func (m Model) agentColor() lipgloss.Color {
	if m.mode == "plan" {
		return colAccent
	}
	return colSecondary
}
