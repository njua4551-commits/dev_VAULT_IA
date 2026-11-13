package tools

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

const (
	maxReadBytes  = 8 << 20
	maxReadLines  = 400
	maxReadOutput = 24 << 10
	maxGrepBytes  = 16 << 10
	maxGrepFile   = 2 << 20
	maxGrepFiles  = 5000
	maxList       = 200
)

func runReadFile(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Path  string `json:"path"`
		Start int    `json:"start"`
		End   int    `json:"end"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	select {
	case <-ctx.Done():
		return errResult("lectura cancelada")
	default:
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errResult("no pude leer %s: %v", path, err)
	}
	if len(data) > maxReadBytes {
		return errResult("%s es demasiado grande (%d bytes; máximo %d)", path, len(data), maxReadBytes)
	}
	if bytes.IndexByte(data, 0) >= 0 {
		return errResult("%s parece binario", path)
	}
	text := strings.ReplaceAll(string(data), "\r\n", "\n")
	lines := strings.Split(text, "\n")
	total := len(lines)
	start := a.Start
	if start < 1 {
		start = 1
	}
	end := a.End
	if end <= 0 {
		end = start + maxReadLines - 1
	}
	if end < start {
		return errResult("el rango es inválido: start=%d end=%d", start, end)
	}
	if start > total {
		return Result{Text: fmt.Sprintf("%s tiene %d líneas; start=%d está fuera.", path, total, start)}
	}
	if end > total {
		end = total
	}
	if end-start+1 > maxReadLines {
		end = start + maxReadLines - 1
	}

	var b strings.Builder
	fmt.Fprintf(&b, "%s · líneas %d-%d de %d\n", path, start, end, total)
	for n := start; n <= end; n++ {
		line := lines[n-1]
		if b.Len()+len(line)+16 > maxReadOutput {
			b.WriteString("[recortado: usa un rango más pequeño]\n")
			break
		}
		fmt.Fprintf(&b, "%d: %s\n", n, line)
	}
	return Result{Text: strings.TrimRight(b.String(), "\n")}
}

func runWriteFile(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Path    string `json:"path"`
		Content string `json:"content"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	select {
	case <-ctx.Done():
		return errResult("escritura cancelada")
	default:
	}
	if err := atomicWrite(path, []byte(a.Content), 0o644); err != nil {
		return errResult("no pude escribir %s: %v", path, err)
	}
	return Result{Text: fmt.Sprintf("escritos %d bytes en %s", len(a.Content), path)}
}

func runEditFile(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Path string `json:"path"`
		Old  string `json:"old"`
		New  string `json:"new"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	if a.Old == "" {
		return errResult("el texto a reemplazar (old) está vacío")
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	select {
	case <-ctx.Done():
		return errResult("edición cancelada")
	default:
	}
	info, err := os.Stat(path)
	if err != nil {
		return errResult("no pude leer %s: %v", path, err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errResult("no pude leer %s: %v", path, err)
	}
	text := string(data)
	count := strings.Count(text, a.Old)
	switch {
	case count == 0:
		return errResult("el texto no aparece en %s; no se cambió nada", path)
	case count > 1:
		return errResult("el texto aparece %d veces en %s; añade más contexto para que sea único", count, path)
	}
	updated := strings.Replace(text, a.Old, a.New, 1)
	if err := atomicWrite(path, []byte(updated), info.Mode().Perm()); err != nil {
		return errResult("no pude guardar %s: %v", path, err)
	}
	return Result{Text: fmt.Sprintf("reemplazo aplicado en %s (%d → %d bytes)", path, len(text), len(updated))}
}

func runListDir(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Path string `json:"path"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	if strings.TrimSpace(a.Path) == "" {
		a.Path = "."
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	select {
	case <-ctx.Done():
		return errResult("listado cancelado")
	default:
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return errResult("no pude listar %s: %v", path, err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	var b strings.Builder
	fmt.Fprintf(&b, "%s\n", path)
	for i, e := range entries {
		if i >= maxList {
			fmt.Fprintf(&b, "… (%d entradas más)\n", len(entries)-maxList)
			break
		}
		name := e.Name()
		if e.IsDir() {
			name += string(filepath.Separator)
		}
		b.WriteString(name)
		if info, err := e.Info(); err == nil && !e.IsDir() {
			fmt.Fprintf(&b, " (%d B)", info.Size())
		}
		b.WriteString("\n")
	}
	if len(entries) == 0 {
		b.WriteString("(carpeta vacía)")
	}
	return Result{Text: strings.TrimRight(b.String(), "\n")}
}

func runGrep(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Pattern    string `json:"pattern"`
		Path       string `json:"path"`
		Glob       string `json:"glob"`
		IgnoreCase bool   `json:"ignore_case"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	if strings.TrimSpace(a.Pattern) == "" {
		return errResult("el patrón está vacío")
	}
	if strings.TrimSpace(a.Path) == "" {
		a.Path = "."
	}
	path, err := resolvePath(a.Path)
	if err != nil {
		return errResult("%v", err)
	}
	pattern := a.Pattern
	if a.IgnoreCase {
		pattern = "(?i)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return errResult("expresión regular inválida: %v", err)
	}
	if a.Glob != "" {
		if _, err := filepath.Match(a.Glob, "x"); err != nil {
			return errResult("glob inválido: %v", err)
		}
	}

	rootInfo, err := os.Stat(path)
	if err != nil {
		return errResult("no pude abrir %s: %v", path, err)
	}
	rootDir := path
	if !rootInfo.IsDir() {
		rootDir = filepath.Dir(path)
	}
	allowedRoot := readRootFrom(ctx)
	allowSensitive := func(file string) bool {
		if allowedRoot == "" || !IsSensitivePath(allowedRoot) {
			return false
		}
		return containsPath(allowedRoot, canonicalPath(file))
	}

	var (
		b         strings.Builder
		matches   int
		visited   int
		truncated bool
	)
	writeMatch := func(file string, line int, text string) bool {
		display := file
		if rel, err := filepath.Rel(rootDir, file); err == nil {
			display = rel
		}
		entry := fmt.Sprintf("%s:%d: %s\n", display, line, strings.TrimRight(text, "\r"))
		if b.Len()+len(entry) > maxGrepBytes {
			truncated = true
			return false
		}
		b.WriteString(entry)
		matches++
		return true
	}

	checkFile := func(file string, d fs.DirEntry) error {
		if sensitive, _ := SensitiveName(d.Name()); sensitive && !allowSensitive(file) {
			return nil
		}
		if a.Glob != "" {
			ok, _ := filepath.Match(a.Glob, d.Name())
			if !ok {
				return nil
			}
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() || info.Size() > maxGrepFile {
			return nil
		}
		visited++
		if visited > maxGrepFiles {
			return io.EOF
		}
		f, err := os.Open(file)
		if err != nil {
			return nil
		}
		defer f.Close()
		head := make([]byte, 8192)
		n, _ := f.Read(head)
		if bytes.IndexByte(head[:n], 0) >= 0 {
			return nil
		}
		_, _ = f.Seek(0, io.SeekStart)
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		line := 0
		for sc.Scan() {
			line++
			if re.MatchString(sc.Text()) {
				if !writeMatch(file, line, sc.Text()) {
					return io.EOF
				}
				if matches >= 100 {
					truncated = true
					return io.EOF
				}
			}
		}
		return nil
	}

	if !rootInfo.IsDir() {
		d := fs.FileInfoToDirEntry(rootInfo)
		if err := checkFile(path, d); err != nil && err != io.EOF {
			return errResult("error leyendo %s: %v", path, err)
		}
	} else {
		err = filepath.WalkDir(rootDir, func(file string, d fs.DirEntry, err error) error {
			if err != nil {
				return nil
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			if d.IsDir() {
				if file != rootDir && skipDir(d.Name()) {
					return filepath.SkipDir
				}
				if file != rootDir && !allowSensitive(file) && IsSensitivePath(file) {
					return filepath.SkipDir
				}
				return nil
			}
			return checkFile(file, d)
		})
		if err != nil && err != io.EOF && err != context.Canceled {
			return errResult("error buscando: %v", err)
		}
	}

	if b.Len() == 0 {
		return Result{Text: "sin coincidencias"}
	}
	header := fmt.Sprintf("%d coincidencias", matches)
	if truncated {
		header += " (resultado recortado)"
	}
	return Result{Text: header + "\n" + strings.TrimRight(b.String(), "\n")}
}

func skipDir(name string) bool {
	switch name {
	case ".git", ".hg", ".svn", "node_modules", "vendor", "dist", "build",
		".venv", "venv", "__pycache__", ".idea", ".vscode", "target":
		return true
	}
	return false
}

// MemoryPath devuelve el archivo de memoria persistente de Nosecode.
func MemoryPath() string {
	if p := strings.TrimSpace(os.Getenv("NOSECODE_MEMORY")); p != "" {
		return p
	}
	cwd, err := os.Getwd()
	if err != nil {
		cwd = "."
	}
	return filepath.Join(cwd, "NOSECODE.md")
}

func runRemember(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Text string `json:"text"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	text := strings.TrimSpace(a.Text)
	if text == "" {
		return errResult("la nota está vacía")
	}
	select {
	case <-ctx.Done():
		return errResult("memoria cancelada")
	default:
	}
	path := MemoryPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return errResult("no pude crear la carpeta de memoria: %v", err)
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR|os.O_APPEND, 0o644)
	if err != nil {
		return errResult("no pude abrir %s: %v", path, err)
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return errResult("no pude revisar %s: %v", path, err)
	}
	if st.Size() == 0 {
		if _, err := f.WriteString("# Memoria de NOSECODE\n\n"); err != nil {
			return errResult("no pude inicializar %s: %v", path, err)
		}
	} else {
		if _, err := f.Seek(-1, io.SeekEnd); err == nil {
			last := make([]byte, 1)
			if n, _ := f.Read(last); n == 1 && last[0] != '\n' {
				if _, err := f.WriteString("\n"); err != nil {
					return errResult("no pude preparar %s: %v", path, err)
				}
			}
		}
	}
	if _, err := fmt.Fprintf(f, "- %s\n", text); err != nil {
		return errResult("no pude guardar la nota: %v", err)
	}
	return Result{Text: "Memoria actualizada en " + path}
}
