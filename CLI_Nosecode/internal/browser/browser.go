// Package browser abre el navegador predeterminado del sistema con el puerto
// de depuracion que Playwright MCP necesita. No guarda secretos.
package browser

import (
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"nosecode/internal/config"
	"nosecode/internal/debuglog"
)

const cdpProbeTimeout = 600 * time.Millisecond

// CDPPort busca un --cdp-endpoint en los servidores MCP y devuelve su puerto.
func CDPPort(servers map[string]config.MCPServer) (int, bool) {
	for _, s := range servers {
		if s.Enabled != nil && !*s.Enabled {
			continue
		}
		for i, arg := range s.Command {
			value := ""
			switch {
			case strings.HasPrefix(arg, "--cdp-endpoint="):
				value = strings.TrimPrefix(arg, "--cdp-endpoint=")
			case arg == "--cdp-endpoint" && i+1 < len(s.Command):
				value = s.Command[i+1]
			}
			if value == "" {
				continue
			}
			u, err := url.Parse(value)
			if err != nil {
				continue
			}
			port, err := strconv.Atoi(u.Port())
			if err == nil && port > 0 {
				return port, true
			}
		}
	}
	return 0, false
}

// EnsureDefault comprueba el CDP y, si esta libre, abre el navegador
// predeterminado con un perfil aislado. Si el predeterminado no es Chromium
// devuelve un error claro en vez de intentar Firefox.
func EnsureDefault(servers map[string]config.MCPServer) error {
	port, ok := CDPPort(servers)
	if !ok {
		debuglog.Logf("browser cdp not configured; using playwright browser")
		return nil // Playwright lanzara su propio navegador.
	}
	if cdpAlive(port) {
		debuglog.Logf("browser cdp already alive port=%d", port)
		return nil
	}
	exe, err := DefaultExecutable()
	if err != nil {
		debuglog.Logf("browser detect error: %s", err)
		return err
	}
	if !IsChromium(exe) {
		return fmt.Errorf("el navegador predeterminado (%s) no soporta CDP; usa Chrome, Edge o Brave, o quita --cdp-endpoint de Playwright", filepath.Base(exe))
	}
	profile := filepath.Join(config.Home(), "browser-profile")
	if err := os.MkdirAll(profile, 0o700); err != nil {
		return err
	}
	args := []string{
		fmt.Sprintf("--remote-debugging-port=%d", port),
		"--remote-allow-origins=*",
		"--user-data-dir=" + profile,
		"--no-first-run",
		"--no-default-browser-check",
		"about:blank",
	}
	debuglog.Logf("browser launch exe=%s port=%d", filepath.Base(exe), port)
	cmd := exec.Command(exe, args...)
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		debuglog.Logf("browser launch error: %s", err)
		return fmt.Errorf("no pude abrir %s: %w", filepath.Base(exe), err)
	}
	_ = cmd.Process.Release()

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if cdpAlive(port) {
			debuglog.Logf("browser cdp ready port=%d", port)
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	debuglog.Logf("browser cdp timeout port=%d", port)
	return fmt.Errorf("el navegador predeterminado no abrio el puerto CDP %d", port)
}

func cdpAlive(port int) bool {
	client := &http.Client{Timeout: cdpProbeTimeout}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/json/version", port))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

// DefaultExecutable lee el navegador predeterminado de Windows.
func DefaultExecutable() (string, error) {
	if runtime.GOOS != "windows" {
		return "", fmt.Errorf("detectar el navegador predeterminado solo esta implementado en Windows")
	}
	progID, err := regValue(`HKCU\Software\Microsoft\Windows\Shell\Associations\UrlAssociations\https\UserChoice`, "ProgId")
	if err != nil {
		return "", err
	}
	command, err := regValue(`HKEY_CLASSES_ROOT\`+progID+`\shell\open\command`, "")
	if err != nil {
		return "", err
	}
	exe, err := executableFromCommand(command)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(exe); err != nil {
		return "", fmt.Errorf("el navegador predeterminado no existe: %s", exe)
	}
	return exe, nil
}

func regValue(key, name string) (string, error) {
	args := []string{"query", key}
	if name == "" {
		args = append(args, "/ve")
	} else {
		args = append(args, "/v", name)
	}
	out, err := exec.Command("reg", args...).Output()
	if err != nil {
		return "", fmt.Errorf("no pude leer el navegador predeterminado")
	}
	for _, line := range strings.Split(string(out), "\n") {
		if i := strings.Index(line, "REG_SZ"); i >= 0 {
			value := strings.TrimSpace(line[i+len("REG_SZ"):])
			if value != "" {
				return value, nil
			}
		}
	}
	return "", fmt.Errorf("no pude leer el navegador predeterminado")
}

func executableFromCommand(command string) (string, error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", fmt.Errorf("el navegador predeterminado no tiene comando")
	}
	if command[0] == '"' {
		if end := strings.Index(command[1:], `"`); end >= 0 {
			return command[1 : end+1], nil
		}
	}
	if fields := strings.Fields(command); len(fields) > 0 {
		return strings.Trim(fields[0], `"`), nil
	}
	return "", fmt.Errorf("no pude interpretar el comando del navegador")
}

// IsChromium indica si el ejecutable acepta el protocolo CDP de Chromium.
// Normaliza separadores de Windows y Linux antes de usar filepath.Base.
func IsChromium(exe string) bool {
	normalized := strings.ReplaceAll(exe, "\\", string(filepath.Separator))
	switch strings.ToLower(filepath.Base(normalized)) {
	case "brave.exe", "brave-browser.exe", "chrome.exe", "msedge.exe",
		"chromium.exe", "opera.exe", "vivaldi.exe", "brave", "brave-browser",
		"google-chrome", "chromium", "microsoft-edge", "opera", "vivaldi":
		return true
	}
	return false
}
