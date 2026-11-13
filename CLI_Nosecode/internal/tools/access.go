package tools

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// ReadDecision es lo que debe hacer el cliente ante una lectura.
type ReadDecision string

const (
	ReadAllow ReadDecision = "allow"
	ReadAsk   ReadDecision = "ask"
	ReadDeny  ReadDecision = "deny"
)

// ReadCheck describe una ruta ya resuelta (sin symlinks ni "..") y si es
// sensible o queda fuera del directorio de trabajo.
type ReadCheck struct {
	Requested string
	Path      string
	Sensitive bool
	Outside   bool
	Reason    string
}

// CheckReadPath normaliza la ruta antes de comprobarla: expande ~, la hace
// absoluta, limpia ".." y resuelve symlinks (también los del tramo que exista).
func CheckReadPath(rawPath, cwd string) ReadCheck {
	requested := strings.TrimSpace(rawPath)
	if requested == "" {
		requested = "."
	}
	if strings.TrimSpace(cwd) == "" {
		cwd, _ = os.Getwd()
	}
	check := ReadCheck{Requested: requested, Path: canonicalPathIn(requested, cwd)}
	check.Outside = !containsPath(canonicalPath(cwd), check.Path)
	check.Sensitive, check.Reason = sensitivePath(check.Path)
	if !check.Sensitive && check.Outside {
		check.Reason = "ruta fuera del directorio de trabajo"
	}
	return check
}

// EvaluateRead aplica permissions.read. Las rutas sensibles siempre piden
// aprobación (bloqueo duro salvo en modo deny); fuera del proyecto manda el
// permiso configurado.
func EvaluateRead(check ReadCheck, mode string) ReadDecision {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "allow":
		if check.Sensitive {
			return ReadAsk
		}
		return ReadAllow
	case "deny":
		if check.Sensitive || check.Outside {
			return ReadDeny
		}
		return ReadAllow
	default: // ask
		if check.Sensitive || check.Outside {
			return ReadAsk
		}
		return ReadAllow
	}
}

// SensitiveName detecta archivos que nunca deberían leerse sin aprobación,
// estén donde estén.
func SensitiveName(name string) (bool, string) {
	lower := strings.ToLower(strings.TrimSpace(name))
	if lower == ".env" || strings.HasPrefix(lower, ".env.") {
		return true, "archivo .env (puede contener secretos)"
	}
	if strings.HasSuffix(lower, ".pem") || strings.HasSuffix(lower, ".key") {
		return true, "archivo de clave (*.pem/*.key)"
	}
	return false, ""
}

// IsSensitivePath dice si una ruta cae en una carpeta o archivo protegido.
func IsSensitivePath(path string) bool {
	sensitive, _ := sensitivePath(path)
	return sensitive
}

func sensitivePath(path string) (bool, string) {
	canon := canonicalPath(path)
	for _, entry := range sensitiveEntries() {
		entryPath := canonicalPath(entry.path)
		if entry.dir {
			if containsPath(entryPath, canon) {
				return true, entry.label
			}
			continue
		}
		if samePath(entryPath, canon) {
			return true, entry.label
		}
	}
	return SensitiveName(filepath.Base(canon))
}

type sensitiveEntry struct {
	path  string
	dir   bool
	label string
}

func sensitiveEntries() []sensitiveEntry {
	home, _ := os.UserHomeDir()
	configDir, _ := os.UserConfigDir()
	out := make([]sensitiveEntry, 0, 10)
	add := func(path string, dir bool, label string) {
		if strings.TrimSpace(path) == "" {
			return
		}
		out = append(out, sensitiveEntry{path: path, dir: dir, label: label})
	}
	if home != "" {
		add(filepath.Join(home, ".dev-vault"), true, "configuración del Dev Vault")
		add(filepath.Join(home, ".nosecode", "config.json"), false, "configuración de Nosecode")
		add(filepath.Join(home, ".ssh"), true, "claves SSH")
		add(filepath.Join(home, ".gnupg"), true, "claves GPG")
		add(filepath.Join(home, ".kaggle"), true, "credenciales de Kaggle")
		add(filepath.Join(home, ".aws"), true, "credenciales de AWS")
		add(filepath.Join(home, ".git-credentials"), false, "credenciales de Git")
		add(filepath.Join(home, ".config", "gcloud"), true, "credenciales de Google Cloud")
	}
	if configDir != "" {
		add(filepath.Join(configDir, "gcloud"), true, "credenciales de Google Cloud")
	}
	return out
}

// canonicalPath expande ~, hace la ruta absoluta y resuelve symlinks. Si el
// archivo aún no existe, resuelve la parte que sí existe y conserva el resto.
func canonicalPath(p string) string {
	return canonicalPathIn(p, "")
}

func canonicalPathIn(p, base string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		p = "."
	}
	if p == "~" || strings.HasPrefix(p, "~/") || strings.HasPrefix(p, `~\`) {
		if home, err := os.UserHomeDir(); err == nil {
			if p == "~" {
				p = home
			} else {
				p = filepath.Join(home, p[2:])
			}
		}
	}
	if !filepath.IsAbs(p) && strings.TrimSpace(base) != "" {
		p = filepath.Join(base, p)
	}
	abs, err := filepath.Abs(p)
	if err != nil {
		abs = filepath.Clean(p)
	}
	return evalPath(abs)
}

func evalPath(p string) string {
	if real, err := filepath.EvalSymlinks(p); err == nil {
		return filepath.Clean(real)
	}
	cur := filepath.Clean(p)
	rest := ""
	for {
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		rest = filepath.Join(filepath.Base(cur), rest)
		cur = parent
		if real, err := filepath.EvalSymlinks(cur); err == nil {
			return filepath.Join(real, rest)
		}
	}
	return filepath.Clean(p)
}

func containsPath(base, target string) bool {
	if strings.TrimSpace(base) == "" || strings.TrimSpace(target) == "" {
		return false
	}
	if runtime.GOOS == "windows" {
		base, target = strings.ToLower(base), strings.ToLower(target)
	}
	rel, err := filepath.Rel(filepath.Clean(base), filepath.Clean(target))
	if err != nil {
		return false
	}
	return rel == "." ||
		(rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel))
}

func samePath(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
	}
	return filepath.Clean(a) == filepath.Clean(b)
}

// WithReadRoot marca la raíz que el usuario aprobó para una herramienta
// recursiva como grep. Solo sirve para no volver a pedir permiso dentro de ella.
func WithReadRoot(ctx context.Context, root string) context.Context {
	if strings.TrimSpace(root) == "" {
		return ctx
	}
	return context.WithValue(ctx, readRootKey{}, root)
}

type readRootKey struct{}

func readRootFrom(ctx context.Context) string {
	root, _ := ctx.Value(readRootKey{}).(string)
	return root
}

// ReadDeniedMessage explica al modelo por qué no se leyó la ruta.
func ReadDeniedMessage(check ReadCheck) string {
	reason := check.Reason
	if reason == "" {
		reason = "ruta no permitida"
	}
	return fmt.Sprintf("lectura bloqueada: %s (%s). No insistas; pide permiso al usuario "+
		"o cambia permissions.read si es una ruta de trabajo.", reason, check.Path)
}
