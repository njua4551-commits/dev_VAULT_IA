package tools

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

// Sandbox aísla los comandos de bash:
//   - Linux: bubblewrap (bwrap). Todo el disco en solo lectura salvo el proyecto,
//     /tmp, cachés habituales y las rutas extra; las carpetas con secretos
//     (~/.ssh, ~/.aws, config de Nosecode…) quedan ocultas; opcionalmente sin red.
//   - macOS: sandbox-exec con un perfil equivalente.
//   - Windows: no disponible.
//
// Modos: "off" (por defecto), "auto" (si hay herramienta; si no, sin aislar) y
// "on" (obligatorio: sin herramienta, bash se niega a ejecutar).

// SandboxConfig es lo que fija la aplicación desde la configuración.
type SandboxConfig struct {
	Mode      string
	NoNetwork bool
	Writable  []string
}

var (
	sbMu    sync.Mutex
	sbCfg   SandboxConfig
	sbProbe struct {
		done bool
		ok   bool
		why  string
	}
)

// SetSandbox actualiza la configuración del sandbox.
func SetSandbox(c SandboxConfig) {
	sbMu.Lock()
	defer sbMu.Unlock()
	c.Mode = strings.ToLower(strings.TrimSpace(c.Mode))
	if c.Mode != "auto" && c.Mode != "on" {
		c.Mode = "off"
	}
	sbCfg = c
}

// SandboxState describe el estado actual: modo, si se aplica de verdad y por qué no.
func SandboxState() (mode string, active bool, note string) {
	sbMu.Lock()
	c := sbCfg
	sbMu.Unlock()
	if c.Mode == "" || c.Mode == "off" {
		return "off", false, "desactivado"
	}
	ok, why := sandboxAvailable()
	if !ok {
		return c.Mode, false, why
	}
	net := "con red"
	if c.NoNetwork {
		net = "sin red"
	}
	return c.Mode, true, sandboxKind() + ", " + net
}

func sandboxKind() string {
	switch runtime.GOOS {
	case "linux":
		return "bubblewrap"
	case "darwin":
		return "sandbox-exec"
	}
	return "ninguno"
}

// sandboxAvailable comprueba (una vez) que la herramienta existe y funciona.
func sandboxAvailable() (bool, string) {
	sbMu.Lock()
	defer sbMu.Unlock()
	if sbProbe.done {
		return sbProbe.ok, sbProbe.why
	}
	sbProbe.done = true
	switch runtime.GOOS {
	case "linux":
		p, err := exec.LookPath("bwrap")
		if err != nil {
			sbProbe.why = "falta bubblewrap (instala el paquete «bubblewrap»)"
			return false, sbProbe.why
		}
		out, err := exec.Command(p, "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "true").CombinedOutput()
		if err != nil {
			sbProbe.why = "bubblewrap no puede crear el aislamiento aquí: " + strings.TrimSpace(string(out))
			return false, sbProbe.why
		}
	case "darwin":
		if _, err := exec.LookPath("sandbox-exec"); err != nil {
			sbProbe.why = "falta sandbox-exec"
			return false, sbProbe.why
		}
	default:
		sbProbe.why = "el sandbox no está disponible en " + runtime.GOOS
		return false, sbProbe.why
	}
	sbProbe.ok = true
	return true, ""
}

// ResetSandboxProbe fuerza a volver a comprobar la disponibilidad (tests).
func ResetSandboxProbe() {
	sbMu.Lock()
	sbProbe.done = false
	sbMu.Unlock()
}

// cacheDirs son carpetas donde las herramientas de desarrollo suelen escribir.
func cacheDirs(home string) []string {
	if home == "" {
		return nil
	}
	var out []string
	for _, d := range []string{".cache", "go", ".npm", ".cargo", ".local/share", ".bun", ".pnpm-store"} {
		p := filepath.Join(home, d)
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			out = append(out, p)
		}
	}
	return out
}

func secretPaths() (dirs, files []string) {
	for _, e := range sensitiveEntries() {
		st, err := os.Stat(e.path)
		if err != nil {
			continue
		}
		if st.IsDir() {
			dirs = append(dirs, e.path)
		} else {
			files = append(files, e.path)
		}
	}
	return
}

// bwrapArgs construye los argumentos de bubblewrap (sin el comando final).
func bwrapArgs(cwd, home string, c SandboxConfig, secretDirs, secretFiles []string) []string {
	a := []string{"--die-with-parent", "--unshare-pid", "--ro-bind", "/", "/", "--dev", "/dev", "--proc", "/proc", "--tmpfs", "/tmp"}
	if c.NoNetwork {
		a = append(a, "--unshare-net")
	}
	writable := append([]string{cwd}, cacheDirs(home)...)
	for _, w := range c.Writable {
		if w = strings.TrimSpace(w); w != "" {
			if strings.HasPrefix(w, "~/") && home != "" {
				w = filepath.Join(home, w[2:])
			}
			writable = append(writable, w)
		}
	}
	for _, w := range writable {
		if _, err := os.Stat(w); err == nil {
			a = append(a, "--bind", w, w)
		}
	}
	for _, d := range secretDirs {
		a = append(a, "--tmpfs", d)
	}
	for _, f := range secretFiles {
		a = append(a, "--ro-bind", "/dev/null", f)
	}
	return append(a, "--chdir", cwd, "--")
}

// seatbeltProfile construye el perfil de sandbox-exec (macOS).
func seatbeltProfile(cwd, home string, c SandboxConfig, secretDirs, secretFiles []string) string {
	q := func(p string) string { return `"` + strings.ReplaceAll(p, `"`, `\"`) + `"` }
	var b strings.Builder
	b.WriteString("(version 1)\n(allow default)\n(deny file-write*)\n(allow file-write*\n")
	for _, p := range append([]string{cwd, "/private/tmp", "/private/var/folders", "/dev"}, append(cacheDirs(home), c.Writable...)...) {
		b.WriteString("  (subpath " + q(p) + ")\n")
	}
	b.WriteString(")\n")
	for _, p := range secretDirs {
		b.WriteString("(deny file-read* (subpath " + q(p) + "))\n")
	}
	for _, p := range secretFiles {
		b.WriteString("(deny file-read* (literal " + q(p) + "))\n")
	}
	if c.NoNetwork {
		b.WriteString("(deny network*)\n")
	}
	return b.String()
}

// sandboxWrap envuelve el comando según la configuración. Devuelve el comando
// final, si se aisló, y un error si el modo «on» no puede cumplirse.
func sandboxWrap(name string, args []string) (string, []string, bool, error) {
	sbMu.Lock()
	c := sbCfg
	sbMu.Unlock()
	if c.Mode != "auto" && c.Mode != "on" {
		return name, args, false, nil
	}
	ok, why := sandboxAvailable()
	if !ok {
		if c.Mode == "on" {
			return "", nil, false, fmt.Errorf("el sandbox está en modo «on» pero no está disponible: %s. Instálalo o usa /sandbox auto|off", why)
		}
		return name, args, false, nil
	}
	cwd, _ := os.Getwd()
	home, _ := os.UserHomeDir()
	dirs, files := secretPaths()
	switch runtime.GOOS {
	case "linux":
		bw, _ := exec.LookPath("bwrap")
		full := append(bwrapArgs(cwd, home, c, dirs, files), name)
		return bw, append(full, args...), true, nil
	case "darwin":
		return "sandbox-exec", append([]string{"-p", seatbeltProfile(cwd, home, c, dirs, files), name}, args...), true, nil
	}
	return name, args, false, nil
}
