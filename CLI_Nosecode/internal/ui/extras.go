package ui

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"nosecode/internal/config"
	"nosecode/internal/tools"

	tea "github.com/charmbracelet/bubbletea"
)

// ------------------------------------------------------------ @archivo ---

const (
	mentionFileMax  = 48 << 10
	mentionTotalMax = 96 << 10
	mentionDirMax   = 100
)

var mentionRe = regexp.MustCompile(`(^|\s)@([^\s]+)`)

// expandMentions añade al texto el contenido de los archivos y carpetas citados
// con @ruta. El texto visible del chat no cambia: solo lo que viaja al modelo.
func expandMentions(text, cwd string) string {
	if !strings.Contains(text, "@") {
		return text
	}
	var extra strings.Builder
	used := 0
	seen := map[string]bool{}
	for _, mm := range mentionRe.FindAllStringSubmatch(text, -1) {
		raw := strings.TrimRight(mm[2], ".,;:)!?»\"'")
		if raw == "" || seen[raw] {
			continue
		}
		seen[raw] = true
		p := raw
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(cwd, p)
		}
		st, err := os.Stat(p)
		if err != nil {
			continue
		}
		if st.IsDir() {
			entries, err := os.ReadDir(p)
			if err != nil {
				continue
			}
			var names []string
			for i, e := range entries {
				if i >= mentionDirMax {
					names = append(names, "…")
					break
				}
				n := e.Name()
				if e.IsDir() {
					n += "/"
				}
				names = append(names, n)
			}
			fmt.Fprintf(&extra, "\n\n[carpeta @%s]\n%s", raw, strings.Join(names, "\n"))
			continue
		}
		if !st.Mode().IsRegular() || used >= mentionTotalMax {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil || strings.IndexByte(string(data[:minInt(len(data), 4096)]), 0) >= 0 {
			continue // binario
		}
		body := string(data)
		if len(body) > mentionFileMax {
			body = body[:mentionFileMax] + "\n[… recortado; usa read_file para el resto]"
		}
		used += len(body)
		fmt.Fprintf(&extra, "\n\n[archivo @%s]\n```\n%s\n```", raw, strings.TrimRight(body, "\n"))
	}
	return text + extra.String()
}

// mentionizePaths convierte rutas de archivos existentes (lo que pega la
// terminal al arrastrar un archivo) en menciones @ruta relativas al proyecto.
func mentionizePaths(text, cwd string) string {
	parts := splitShellish(text)
	if len(parts) == 0 {
		return text
	}
	changed := false
	for i, tok := range parts {
		if !strings.ContainsAny(tok, `/\`) || strings.HasPrefix(tok, "@") {
			continue
		}
		p := tok
		if strings.HasPrefix(p, "file://") {
			p = strings.TrimPrefix(p, "file://")
		}
		if !filepath.IsAbs(p) {
			continue // solo rutas absolutas: evita confundir texto normal
		}
		st, err := os.Stat(p)
		if err != nil || !(st.Mode().IsRegular() || st.IsDir()) {
			continue
		}
		if rel, err := filepath.Rel(cwd, p); err == nil && !strings.HasPrefix(rel, "..") {
			p = rel
		}
		if strings.ContainsAny(p, " \t") {
			continue // las menciones no admiten espacios
		}
		parts[i] = "@" + p
		changed = true
	}
	if !changed {
		return text
	}
	return strings.Join(parts, " ")
}

// ---------------------------------------------------- verificación /check ---

// detectChecks propone los comandos de verificación del proyecto.
func detectChecks(cwd string) []string {
	has := func(name string) bool { _, err := os.Stat(filepath.Join(cwd, name)); return err == nil }
	switch {
	case has("go.mod"):
		return []string{"go build ./...", "go vet ./...", "go test ./..."}
	case has("Cargo.toml"):
		return []string{"cargo check", "cargo test"}
	case has("package.json"):
		var pkg struct {
			Scripts map[string]string `json:"scripts"`
		}
		if data, err := os.ReadFile(filepath.Join(cwd, "package.json")); err == nil {
			_ = json.Unmarshal(data, &pkg)
		}
		var out []string
		for _, s := range []string{"lint", "typecheck", "build", "test"} {
			if _, ok := pkg.Scripts[s]; ok {
				out = append(out, "npm run "+s)
			}
		}
		return out
	case has("pyproject.toml") || has("pytest.ini") || has("setup.py"):
		return []string{"python -m pytest -q"}
	case has("Makefile"):
		return []string{"make test"}
	}
	return nil
}

func checkPrompt(cmds []string) string {
	return "Verifica el proyecto: ejecuta " + strings.Join(cmds, " y luego ") +
		". Si algo falla, lee el error, corrige la causa con edit_file y repite hasta que todo pase. " +
		"No declares que terminaste mientras haya errores; al final resume qué corregiste."
}

// checkStart lanza el bucle verificar → corregir → verificar.
func (m Model) checkStart(arg string) (tea.Model, tea.Cmd) {
	if m.mode == "plan" {
		m.toast, m.toastErr = "/check puede editar archivos: pasa a Build (shift+tab)", true
		return m, nil
	}
	if !m.providerReady() {
		m.toast, m.toastErr = "Conecta un proveedor para usar /check", true
		return m, nil
	}
	cmds := []string{}
	if strings.TrimSpace(arg) != "" {
		cmds = []string{strings.TrimSpace(arg)}
	} else {
		cwd, _ := os.Getwd()
		cmds = detectChecks(cwd)
	}
	if len(cmds) == 0 {
		m.toast, m.toastErr = "No reconozco el proyecto: usa /check <comando>", true
		return m, nil
	}
	m.msgs = append(m.msgs, Message{Role: "user", Text: checkPrompt(cmds)})
	m = m.saveActive()
	return m.startAgent()
}

// projectPrompt añade al sistema cómo verificar este proyecto y las skills.
func projectPrompt(cwd string) string {
	var b strings.Builder
	if cmds := detectChecks(cwd); len(cmds) > 0 {
		b.WriteString("\n\nVerificación de este proyecto: " + strings.Join(cmds, " ; ") +
			". Tras cambiar código, ejecútala y corrige hasta que pase antes de dar la tarea por terminada.")
	}
	b.WriteString(tools.SkillsPrompt())
	return b.String()
}

// ---------------------------------------------------- comandos propios ---

type customCommand struct {
	name, desc, body string
}

// loadCustomCommands lee .nosecode/commands/*.md del proyecto y de ~/.nosecode/commands.
// El proyecto gana sobre el usuario. Plantilla: $ARGUMENTS, $1, $2…
func loadCustomCommands(cwd, home string) []customCommand {
	seen := map[string]bool{}
	var out []customCommand
	for _, dir := range []string{filepath.Join(cwd, ".nosecode", "commands"), filepath.Join(home, "commands")} {
		files, _ := filepath.Glob(filepath.Join(dir, "*.md"))
		for _, f := range files {
			name := strings.ToLower(strings.TrimSuffix(filepath.Base(f), ".md"))
			if seen[name] || !mcpNameRe.MatchString(name) {
				continue
			}
			data, err := os.ReadFile(f)
			if err != nil {
				continue
			}
			body := strings.TrimSpace(string(data))
			desc := "comando propio"
			if strings.HasPrefix(body, "---") {
				if end := strings.Index(body[3:], "\n---"); end >= 0 {
					for _, ln := range strings.Split(body[3:3+end], "\n") {
						if k, v, ok := strings.Cut(ln, ":"); ok && strings.TrimSpace(k) == "description" {
							desc = strings.Trim(strings.TrimSpace(v), `"'`)
						}
					}
					body = strings.TrimSpace(body[3+end+4:])
				}
			}
			if body == "" {
				continue
			}
			seen[name] = true
			out = append(out, customCommand{name: name, desc: desc, body: body})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out
}

func (c customCommand) expand(args string) string {
	body := strings.ReplaceAll(c.body, "$ARGUMENTS", args)
	for i, w := range strings.Fields(args) {
		body = strings.ReplaceAll(body, fmt.Sprintf("$%d", i+1), w)
	}
	if !strings.Contains(c.body, "$ARGUMENTS") && strings.TrimSpace(args) != "" {
		body += "\n\n" + args
	}
	return strings.TrimSpace(body)
}

func (m Model) customCommands() []customCommand {
	cwd, _ := os.Getwd()
	return loadCustomCommands(cwd, config.Home())
}

// expandCustom convierte «/nombre args» en el texto de la plantilla.
func (m Model) expandCustom(text string) (string, bool) {
	f := strings.Fields(text)
	if len(f) == 0 || !strings.HasPrefix(f[0], "/") {
		return "", false
	}
	name := strings.ToLower(strings.TrimPrefix(f[0], "/"))
	for _, e := range slashEntries {
		if e.name == "/"+name {
			return "", false // un comando integrado nunca se pisa
		}
	}
	for _, c := range m.customCommands() {
		if c.name == name {
			return c.expand(strings.TrimSpace(strings.TrimPrefix(text, f[0]))), true
		}
	}
	return "", false
}

func (m Model) commandsSlash() Model {
	list := m.customCommands()
	var b strings.Builder
	if len(list) == 0 {
		b.WriteString("No hay comandos propios.\nCrea .nosecode/commands/revisar.md (en el proyecto) o ~/.nosecode/commands/revisar.md y úsalo como /revisar.\n" +
			"Dentro del archivo, $ARGUMENTS o $1, $2… se sustituyen por lo que escribas; un bloque opcional ---\\ndescription: …\\n--- da la descripción.")
	} else {
		b.WriteString("Comandos propios:\n")
		for _, c := range list {
			b.WriteString("\n• /" + c.name + " — " + c.desc)
		}
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: b.String()})
	return m
}

// userTextForModel es el texto de un mensaje de usuario tal como viaja al modelo.
func userTextForModel(text string) string {
	cwd, _ := os.Getwd()
	return expandMentions(text, cwd)
}

const commitPrompt = "Prepara un commit de git con los cambios actuales: ejecuta git status y git diff (usa --stat primero si es grande), " +
	"decide qué archivos corresponden a un mismo cambio, escribe un mensaje claro (primera línea de máx. 72 caracteres, " +
	"en imperativo, y un cuerpo breve si hace falta), haz git add de esos archivos y git commit. " +
	"No hagas push, no uses --amend ni --force, y no incluyas archivos con secretos (.env, claves)."

// promptStart envía un texto como si lo hubiera escrito el usuario y arranca el agente.
func (m Model) promptStart(text, needsEdit string) (tea.Model, tea.Cmd) {
	if m.mode == "plan" {
		m.toast, m.toastErr = needsEdit+" puede cambiar archivos: pasa a Build (shift+tab)", true
		return m, nil
	}
	if !m.providerReady() {
		m.toast, m.toastErr = "Conecta un proveedor para usar "+needsEdit, true
		return m, nil
	}
	m.msgs = append(m.msgs, Message{Role: "user", Text: text})
	m = m.saveActive()
	return m.startAgent()
}

// mapSlash imprime el mapa del repositorio que ve el agente con repo_map.
func (m Model) mapSlash() Model {
	cwd, _ := os.Getwd()
	text := tools.RepoMap(cwd, 6000)
	if text == "" {
		text = "(carpeta vacía)"
	}
	m.msgs = append(m.msgs, Message{Role: "assistant", Text: "Mapa del proyecto (lo mismo que ve el agente con repo_map):\n\n" + text})
	return m
}

const prPrompt = "Prepara un pull request: 1) git status y git branch para ver dónde estás; si estás en main/master, crea una rama con nombre descriptivo. " +
	"2) Si hay cambios sin commit, haz commit con un buen mensaje. 3) Muestra git log y git diff contra la rama base y redacta título y descripción " +
	"(qué cambia y por qué, cómo probarlo). 4) Haz git push -u origin <rama> y luego gh pr create con ese título y descripción " +
	"(si gh no está instalado o no hay sesión, dilo y da el comando exacto). Nunca uses --force."

func worktreePrompt(name string) string {
	return "Crea un git worktree aislado para trabajar sin tocar la rama actual: ejecuta `git worktree add ../<carpeta>-" + name +
		" -b " + name + "` desde la raíz del repositorio (comprueba primero con git rev-parse y git worktree list que no existe ya). " +
		"Al terminar dime la ruta nueva y cómo entrar (cd) y cómo borrarlo después (git worktree remove)."
}
