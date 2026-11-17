package tools

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var (
	mapSkipDirs = map[string]bool{".git": true, "node_modules": true, "vendor": true, "dist": true, "build": true,
		"target": true, "__pycache__": true, ".venv": true, "venv": true, ".next": true, ".cache": true}
	symRes = map[string]*regexp.Regexp{
		".go":   regexp.MustCompile(`^(?:func (?:\([^)]*\) )?([A-Za-z_]\w*)|type ([A-Za-z_]\w*) )`),
		".py":   regexp.MustCompile(`^(?:def|class) ([A-Za-z_]\w*)`),
		".js":   regexp.MustCompile(`^(?:export )?(?:default )?(?:async )?(?:function|class) ([A-Za-z_$][\w$]*)`),
		".ts":   regexp.MustCompile(`^(?:export )?(?:default )?(?:async )?(?:function|class|interface|type) ([A-Za-z_$][\w$]*)`),
		".rs":   regexp.MustCompile(`^\s*(?:pub )?(?:async )?(?:fn|struct|enum|trait) ([A-Za-z_]\w*)`),
		".java": regexp.MustCompile(`^\s*(?:public |private |protected )?(?:static )?(?:class|interface|enum) ([A-Za-z_]\w*)`),
	}
)

func init() {
	symRes[".tsx"], symRes[".jsx"], symRes[".mjs"] = symRes[".ts"], symRes[".js"], symRes[".js"]
}

// RepoMap devuelve un árbol compacto del proyecto con los símbolos principales
// de cada archivo, recortado a maxBytes.
func RepoMap(root string, maxBytes int) string {
	type entry struct {
		rel  string
		syms []string
	}
	var files []entry
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		name := d.Name()
		if d.IsDir() {
			if p != root && (mapSkipDirs[name] || strings.HasPrefix(name, ".")) {
				return filepath.SkipDir
			}
			return nil
		}
		if len(files) >= 2000 || strings.HasPrefix(name, ".") {
			return nil
		}
		rel, _ := filepath.Rel(root, p)
		e := entry{rel: filepath.ToSlash(rel)}
		if re, ok := symRes[strings.ToLower(filepath.Ext(name))]; ok {
			e.syms = scanSymbols(p, re)
		}
		files = append(files, e)
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].rel < files[j].rel })
	var b strings.Builder
	for _, f := range files {
		line := f.rel
		if len(f.syms) > 0 {
			line += ": " + strings.Join(f.syms, ", ")
		}
		if b.Len()+len(line)+1 > maxBytes {
			fmt.Fprintf(&b, "[… recortado; %d archivos en total, usa list_dir/grep para más]\n", len(files))
			break
		}
		b.WriteString(line + "\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func scanSymbols(path string, re *regexp.Regexp) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 64<<10), 1<<20)
	for sc.Scan() && len(out) < 12 {
		if m := re.FindStringSubmatch(sc.Text()); m != nil {
			for _, g := range m[1:] {
				if g != "" {
					out = append(out, g)
					break
				}
			}
		}
	}
	return out
}

func runRepoMap(_ context.Context, raw json.RawMessage) Result {
	var a struct {
		Path string `json:"path"`
	}
	_ = decode(raw, &a)
	root := "."
	if strings.TrimSpace(a.Path) != "" {
		p, err := resolvePath(a.Path)
		if err != nil {
			return errResult("%v", err)
		}
		root = p
	}
	text := RepoMap(root, 6000)
	if text == "" {
		text = "(carpeta vacía)"
	}
	return Result{Text: text, Bytes: len(text)}
}
