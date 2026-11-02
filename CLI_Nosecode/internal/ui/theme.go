package ui

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
)

// Palette define los colores de la interfaz (tema Vercel).
type Palette struct {
	Name         string
	Bg           string
	Panel        string
	Element      string
	Border       string
	BorderActive string
	Text         string
	Muted        string
	Faint        string
	Primary      string
	Secondary    string
	Accent       string
	Success      string
	Warning      string
	Danger       string
	Cyan         string
}

var Vercel = Palette{
	Name:         "vercel",
	Bg:           "#000000",
	Panel:        "#1A1A1A",
	Element:      "#292929",
	Border:       "#1F1F1F",
	BorderActive: "#454545",
	Text:         "#EDEDED",
	Muted:        "#878787",
	Faint:        "#5C5C5C",
	Primary:      "#0070F3",
	Secondary:    "#52A8FF",
	Accent:       "#8E4EC6",
	Success:      "#46A758",
	Warning:      "#FFB224",
	Danger:       "#E5484D",
	Cyan:         "#50E3C2",
}

var Midnight = Palette{
	Name:         "midnight",
	Bg:           "#0B0B0B",
	Panel:        "#141414",
	Element:      "#1E1E1E",
	Border:       "#2A2A2A",
	BorderActive: "#4A4A4A",
	Text:         "#E6E6E6",
	Muted:        "#8F8F8F",
	Faint:        "#5C5C5C",
	Primary:      "#7EE0D0",
	Secondary:    "#F2CC60",
	Accent:       "#C792EA",
	Success:      "#8FD46D",
	Warning:      "#F2CC60",
	Danger:       "#F2777A",
	Cyan:         "#7EE0D0",
}

// PaletteByName resuelve un tema por nombre; vercel es el default.
func PaletteByName(name string) Palette {
	switch strings.ToLower(strings.TrimSpace(name)) {
	case "midnight", "nosecode-dark", "nosecode":
		return Midnight
	default:
		return Vercel
	}
}

var (
	colBg, colPanel, colElement, colBorder, colBorderActive lipgloss.Color
	colText, colMuted, colFaint                             lipgloss.Color
	colPrimary, colSecondary, colAccent                     lipgloss.Color
	colSuccess, colWarning, colDanger, colCyan              lipgloss.Color
)

var (
	appStyle          lipgloss.Style
	textStyle         lipgloss.Style
	titleStyle        lipgloss.Style
	mutedStyle        lipgloss.Style
	faintStyle        lipgloss.Style
	primaryStyle      lipgloss.Style
	secondaryStyle    lipgloss.Style
	accentStyle       lipgloss.Style
	successStyle      lipgloss.Style
	warningStyle      lipgloss.Style
	dangerStyle       lipgloss.Style
	cyanStyle         lipgloss.Style
	logoStyle         lipgloss.Style
	logoMutedStyle    lipgloss.Style
	chipStyle         lipgloss.Style
	chipPrimaryStyle  lipgloss.Style
	chipMutedStyle    lipgloss.Style
	chipWarningStyle  lipgloss.Style
	chipDangerStyle   lipgloss.Style
	tabActiveStyle    lipgloss.Style
	tabIdleStyle      lipgloss.Style
	rowSelectedStyle  lipgloss.Style
	selectedStyle     lipgloss.Style
	selectStyle       lipgloss.Style
	keyStyle          lipgloss.Style
	borderStyle       lipgloss.Style
	borderActiveStyle lipgloss.Style
	modalTitleStyle   lipgloss.Style
	statusStyle       lipgloss.Style
	inputStyle        lipgloss.Style
)

func init() {
	applyPalette(Vercel)
}

// applyPalette reconstruye todos los estilos con los colores del tema.
func applyPalette(p Palette) {
	colBg = lipgloss.Color(p.Bg)
	colPanel = lipgloss.Color(p.Panel)
	colElement = lipgloss.Color(p.Element)
	colBorder = lipgloss.Color(p.Border)
	colBorderActive = lipgloss.Color(p.BorderActive)
	colText = lipgloss.Color(p.Text)
	colMuted = lipgloss.Color(p.Muted)
	colFaint = lipgloss.Color(p.Faint)
	colPrimary = lipgloss.Color(p.Primary)
	colSecondary = lipgloss.Color(p.Secondary)
	colAccent = lipgloss.Color(p.Accent)
	colSuccess = lipgloss.Color(p.Success)
	colWarning = lipgloss.Color(p.Warning)
	colDanger = lipgloss.Color(p.Danger)
	colCyan = lipgloss.Color(p.Cyan)

	appStyle = lipgloss.NewStyle().Background(colBg).Foreground(colText)
	textStyle = lipgloss.NewStyle().Foreground(colText)
	titleStyle = lipgloss.NewStyle().Foreground(colText).Bold(true)
	mutedStyle = lipgloss.NewStyle().Foreground(colMuted)
	faintStyle = lipgloss.NewStyle().Foreground(colFaint)
	primaryStyle = lipgloss.NewStyle().Foreground(colPrimary)
	secondaryStyle = lipgloss.NewStyle().Foreground(colSecondary)
	accentStyle = lipgloss.NewStyle().Foreground(colAccent)
	successStyle = lipgloss.NewStyle().Foreground(colSuccess)
	warningStyle = lipgloss.NewStyle().Foreground(colWarning)
	dangerStyle = lipgloss.NewStyle().Foreground(colDanger)
	cyanStyle = lipgloss.NewStyle().Foreground(colCyan)
	logoStyle = lipgloss.NewStyle().Foreground(colText).Bold(true)
	logoMutedStyle = lipgloss.NewStyle().Foreground(colMuted).Bold(true)

	chipStyle = lipgloss.NewStyle().Background(colElement).Foreground(colText).Padding(0, 1)
	chipPrimaryStyle = lipgloss.NewStyle().Background(colPrimary).Foreground(lipgloss.Color("#FFFFFF")).Padding(0, 1)
	chipMutedStyle = lipgloss.NewStyle().Background(colElement).Foreground(colMuted).Padding(0, 1)
	chipWarningStyle = lipgloss.NewStyle().Background(colWarning).Foreground(lipgloss.Color("#000000")).Bold(true).Padding(0, 1)
	chipDangerStyle = lipgloss.NewStyle().Background(colDanger).Foreground(lipgloss.Color("#FFFFFF")).Bold(true).Padding(0, 1)
	tabActiveStyle = lipgloss.NewStyle().Background(colElement).Foreground(colPrimary).Bold(true).Padding(0, 1)
	tabIdleStyle = lipgloss.NewStyle().Foreground(colMuted).Padding(0, 1)
	rowSelectedStyle = lipgloss.NewStyle().Background(colElement).Foreground(colText)
	selectedStyle = lipgloss.NewStyle().Background(colPrimary).Foreground(lipgloss.Color("#FFFFFF"))
	selectStyle = lipgloss.NewStyle().Reverse(true)
	keyStyle = lipgloss.NewStyle().Foreground(colSecondary)
	borderStyle = lipgloss.NewStyle().Foreground(colBorder)
	borderActiveStyle = lipgloss.NewStyle().Foreground(colBorderActive)
	modalTitleStyle = lipgloss.NewStyle().Foreground(colText).Bold(true)
	statusStyle = lipgloss.NewStyle().Background(colPanel).Foreground(colMuted)
	inputStyle = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(colBorderActive)
}

// --- utilidades de texto ------------------------------------------------

func pad(s string, w int) string {
	if w <= 0 {
		return ""
	}
	if lipgloss.Width(s) > w {
		return ansi.Truncate(s, w, "…")
	}
	return s + strings.Repeat(" ", w-lipgloss.Width(s))
}

func spaces(n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(" ", n)
}

func center(s string, w int) string {
	if w <= 0 {
		return ""
	}
	d := w - lipgloss.Width(s)
	if d <= 0 {
		return pad(s, w)
	}
	l := d / 2
	return spaces(l) + s + spaces(d-l)
}

// sanitizeText elimina secuencias de control que pueden romper el terminal.
func sanitizeText(s string) string {
	s = ansi.Strip(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r == '\n':
			b.WriteRune(r)
		case r == '\t':
			b.WriteString("    ")
		case r == '\r':
			continue
		case r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f):
			continue
		default:
			b.WriteRune(r)
		}
	}
	return b.String()
}

func wrapText(s string, width int) []string {
	if width < 10 {
		width = 10
	}
	var out []string
	for _, para := range strings.Split(s, "\n") {
		// una línea que ya cabe se respeta tal cual (sangrías y columnas)
		if lipgloss.Width(para) <= width {
			out = append(out, para)
			continue
		}
		words := strings.Fields(para)
		if len(words) == 0 {
			out = append(out, "")
			continue
		}
		line := ""
		for _, word := range words {
			for lipgloss.Width(word) > width {
				piece, rest := splitDisplay(word, width)
				if line != "" {
					out = append(out, line)
					line = ""
				}
				if piece != "" {
					out = append(out, piece)
				}
				word = rest
			}
			if word == "" {
				continue
			}
			if line == "" {
				line = word
				continue
			}
			if lipgloss.Width(line)+1+lipgloss.Width(word) <= width {
				line += " " + word
			} else {
				out = append(out, line)
				line = word
			}
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// splitDisplay parte una palabra larga sin romper caracteres anchos.
func splitDisplay(s string, width int) (string, string) {
	var b strings.Builder
	used := 0
	for i, r := range s {
		w := lipgloss.Width(string(r))
		if used+w > width && used > 0 {
			return b.String(), s[i:]
		}
		b.WriteRune(r)
		used += w
	}
	return b.String(), ""
}

// --- logotipo -----------------------------------------------------------

var glyphs = map[rune][]string{
	'N': {"█▀▀█", "█  █", "▀  ▀"},
	'O': {"█▀▀█", "█  █", "▀▀▀▀"},
	'S': {"█▀▀▀", "▀▀▀█", "▀▀▀▀"},
	'E': {"█▀▀▀", "█▀▀ ", "▀▀▀▀"},
	'C': {"█▀▀▀", "█   ", "▀▀▀▀"},
	'D': {"█▀▀▄", "█  █", "▀▀▀▀"},
}

func glyphRows(word string) []string {
	rows := []string{"", "", ""}
	for i, ch := range word {
		g, ok := glyphs[ch]
		if !ok {
			continue
		}
		for r := 0; r < 3; r++ {
			if i > 0 {
				rows[r] += " "
			}
			rows[r] += g[r]
		}
	}
	return rows
}

// logoRows devuelve el wordmark en bloques: NOSE atenuado y CODE resaltado,
// igual que «open» y «code» en opencode.
func logoRows() []string {
	left := glyphRows("NOSE")
	right := glyphRows("CODE")
	out := make([]string, 3)
	for i := 0; i < 3; i++ {
		out[i] = logoMutedStyle.Render(left[i]) + "   " + logoStyle.Render(right[i])
	}
	return out
}
