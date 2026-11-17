package tools

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SkillDirs son las carpetas donde se buscan skills (cada una con
// subcarpetas <nombre>/SKILL.md). La interfaz las fija al arrancar.
var SkillDirs []string

// Skill es una guía reutilizable que el agente carga bajo demanda.
type Skill struct {
	Name        string
	Description string
	Path        string
}

// ListSkills devuelve las skills encontradas (la primera carpeta gana).
func ListSkills() []Skill {
	seen := map[string]bool{}
	var out []Skill
	for _, dir := range SkillDirs {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			p := filepath.Join(dir, e.Name(), "SKILL.md")
			data, err := os.ReadFile(p)
			if err != nil {
				continue
			}
			name, desc := skillMeta(string(data))
			if name == "" {
				name = e.Name()
			}
			if seen[name] {
				continue
			}
			seen[name] = true
			out = append(out, Skill{Name: name, Description: desc, Path: p})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// skillMeta lee name y description del frontmatter.
func skillMeta(text string) (name, desc string) {
	if !strings.HasPrefix(text, "---") {
		return "", ""
	}
	rest := strings.TrimPrefix(text, "---")
	end := strings.Index(rest, "\n---")
	if end < 0 {
		return "", ""
	}
	for _, ln := range strings.Split(rest[:end], "\n") {
		k, v, ok := strings.Cut(ln, ":")
		if !ok {
			continue
		}
		v = strings.Trim(strings.TrimSpace(v), `"'`)
		switch strings.TrimSpace(k) {
		case "name":
			name = v
		case "description":
			desc = v
		}
	}
	return
}

// SkillsPrompt es el índice breve que va al prompt del sistema.
func SkillsPrompt() string {
	list := ListSkills()
	if len(list) == 0 {
		return ""
	}
	var b strings.Builder
	b.WriteString("\n\nSkills disponibles (carga una con la herramienta skill cuando la tarea encaje):")
	for i, s := range list {
		if i >= 20 {
			break
		}
		b.WriteString("\n- " + s.Name + ": " + TruncateText(s.Description, 160))
	}
	return b.String()
}

func runSkill(_ context.Context, raw json.RawMessage) Result {
	var a struct {
		Name string `json:"name"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	for _, s := range ListSkills() {
		if strings.EqualFold(s.Name, strings.TrimSpace(a.Name)) {
			data, err := os.ReadFile(s.Path)
			if err != nil {
				return errResult("no pude leer la skill: %v", err)
			}
			text := TruncateText(string(data), 12<<10)
			text += "\n\n(Carpeta de la skill: " + filepath.Dir(s.Path) + " — puedes leer sus otros archivos con read_file.)"
			return Result{Text: text, Bytes: len(text)}
		}
	}
	names := []string{}
	for _, s := range ListSkills() {
		names = append(names, s.Name)
	}
	return errResult("no existe la skill %q. Disponibles: %s", a.Name, strings.Join(names, ", "))
}
