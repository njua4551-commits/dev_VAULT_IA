package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

var hunkRe = regexp.MustCompile(`^@@ -(\d+)(?:,(\d+))? \+(\d+)(?:,(\d+))? @@`)

type patchHunk struct {
	start    int // línea esperada (1-based) del texto viejo
	old, new []string
}

type filePatch struct {
	path    string
	create  bool
	del     bool
	hunks   []patchHunk
	oldName string
}

// parseUnifiedDiff lee un diff unificado con uno o varios archivos.
func parseUnifiedDiff(text string) ([]filePatch, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	var out []filePatch
	i := 0
	clean := func(p string) string {
		p = strings.TrimSpace(p)
		if tab := strings.IndexByte(p, '\t'); tab >= 0 {
			p = p[:tab]
		}
		p = strings.Trim(p, `"`)
		return p
	}
	strip := func(p string) string {
		if strings.HasPrefix(p, "a/") || strings.HasPrefix(p, "b/") {
			return p[2:]
		}
		return p
	}
	for i < len(lines) {
		if !(strings.HasPrefix(lines[i], "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ")) {
			i++
			continue
		}
		oldP, newP := clean(lines[i][4:]), clean(lines[i+1][4:])
		i += 2
		fp := filePatch{oldName: oldP}
		switch {
		case oldP == "/dev/null":
			fp.create, fp.path = true, strip(newP)
		case newP == "/dev/null":
			fp.del, fp.path = true, strip(oldP)
		default:
			fp.path = strip(newP)
		}
		for i < len(lines) {
			if strings.HasPrefix(lines[i], "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ") {
				break
			}
			mm := hunkRe.FindStringSubmatch(lines[i])
			if mm == nil {
				i++
				continue
			}
			start, _ := strconv.Atoi(mm[1])
			h := patchHunk{start: start}
			i++
			for i < len(lines) {
				ln := lines[i]
				if strings.HasPrefix(ln, "@@") || (strings.HasPrefix(ln, "--- ") && i+1 < len(lines) && strings.HasPrefix(lines[i+1], "+++ ")) {
					break
				}
				switch {
				case strings.HasPrefix(ln, `\`):
				case strings.HasPrefix(ln, "+"):
					h.new = append(h.new, ln[1:])
				case strings.HasPrefix(ln, "-"):
					h.old = append(h.old, ln[1:])
				case strings.HasPrefix(ln, " "):
					h.old = append(h.old, ln[1:])
					h.new = append(h.new, ln[1:])
				case ln == "":
					// línea de contexto vacía (muchos modelos quitan el espacio) salvo al final del texto
					if i < len(lines)-1 {
						h.old = append(h.old, "")
						h.new = append(h.new, "")
					}
				default:
					return nil, fmt.Errorf("línea no válida dentro de un hunk de %s: %q", fp.path, ln)
				}
				i++
			}
			fp.hunks = append(fp.hunks, h)
		}
		if len(fp.hunks) == 0 {
			return nil, fmt.Errorf("el diff de %s no tiene hunks (@@ … @@)", fp.path)
		}
		out = append(out, fp)
	}
	if len(out) == 0 {
		return nil, errors.New("no encontré ningún archivo en el diff (faltan líneas --- y +++)")
	}
	return out, nil
}

func sameLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if strings.TrimRight(a[i], " \t\r") != strings.TrimRight(b[i], " \t\r") {
			return false
		}
	}
	return true
}

// applyHunks aplica los hunks buscando el texto viejo por contenido (los
// números de línea que dan los modelos suelen estar desplazados).
func applyHunks(content string, hunks []patchHunk) (string, error) {
	trailing := strings.HasSuffix(content, "\n")
	body := strings.TrimSuffix(content, "\n")
	var lines []string
	if content != "" {
		lines = strings.Split(body, "\n")
	}
	delta := 0
	for hi, h := range hunks {
		pos := -1
		if len(h.old) == 0 { // solo inserta
			pos = h.start - 1 + delta
			if pos < 0 {
				pos = 0
			}
			if pos > len(lines) {
				pos = len(lines)
			}
		} else {
			expect := h.start - 1 + delta
			best, bestDist := -1, 1<<30
			for p := 0; p+len(h.old) <= len(lines); p++ {
				if sameLines(lines[p:p+len(h.old)], h.old) {
					d := p - expect
					if d < 0 {
						d = -d
					}
					if d < bestDist {
						best, bestDist = p, d
					}
				}
			}
			pos = best
		}
		if pos < 0 {
			return "", fmt.Errorf("el hunk %d no coincide con el archivo (el contexto o las líneas «-» no existen tal cual); relee el archivo y rehaz el diff", hi+1)
		}
		next := append([]string{}, lines[:pos]...)
		next = append(next, h.new...)
		next = append(next, lines[pos+len(h.old):]...)
		lines = next
		delta += len(h.new) - len(h.old)
	}
	out := strings.Join(lines, "\n")
	if trailing || content == "" {
		out += "\n"
	}
	return out, nil
}

func runApplyPatch(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Patch string `json:"patch"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	patches, err := parseUnifiedDiff(a.Patch)
	if err != nil {
		return errResult("%v", err)
	}
	type planned struct {
		path, before, after string
		existed             bool
		mode                os.FileMode
	}
	var plan []planned
	for _, fp := range patches {
		if fp.del {
			return errResult("borrar archivos no se hace con apply_patch; usa bash (rm) o pídelo al usuario")
		}
		path, err := resolvePath(fp.path)
		if err != nil {
			return errResult("%v", err)
		}
		pl := planned{path: path, mode: 0o644}
		if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
			data, err := os.ReadFile(path)
			if err != nil {
				return errResult("no pude leer %s: %v", path, err)
			}
			pl.before, pl.existed, pl.mode = string(data), true, info.Mode().Perm()
		} else if !fp.create {
			return errResult("%s no existe (para crear archivos el diff debe usar --- /dev/null)", path)
		}
		if fp.create && pl.existed {
			return errResult("%s ya existe; un diff de archivo nuevo no puede pisarlo", path)
		}
		after, err := applyHunks(pl.before, fp.hunks)
		if err != nil {
			return errResult("%s: %v. No se cambió ningún archivo.", path, err)
		}
		pl.after = after
		plan = append(plan, pl)
	}
	select {
	case <-ctx.Done():
		return errResult("parche cancelado")
	default:
	}
	var summary, diff strings.Builder
	var edited []string
	for _, pl := range plan {
		done := snapshotBefore(pl.path)
		if err := atomicWrite(pl.path, []byte(pl.after), pl.mode); err != nil {
			return errResult("no pude guardar %s: %v (los archivos anteriores del parche ya se aplicaron; usa /undo)", pl.path, err)
		}
		done()
		kind := "modificado"
		if !pl.existed {
			kind = "creado"
		}
		fmt.Fprintf(&summary, "%s %s\n", kind, pl.path)
		diff.WriteString("# " + filepath.Base(pl.path) + "\n" + MiniDiff(pl.before, pl.after) + "\n")
		edited = append(edited, pl.path)
	}
	return Result{Text: fmt.Sprintf("parche aplicado a %d archivo(s):\n%s", len(plan), strings.TrimRight(summary.String(), "\n")),
		Diff: strings.TrimRight(diff.String(), "\n"), Edited: edited}
}
