package tools

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

const (
	captureTimeout = 15 * time.Second
	maxImageBytes  = 20 << 20
)

func runScreenshot(ctx context.Context, _ json.RawMessage) Result {
	ctx, cancel := context.WithTimeout(ctx, captureTimeout)
	defer cancel()
	data, err := captureScreen(ctx)
	if err != nil {
		return errResult("%v", err)
	}
	if len(data) == 0 {
		return errResult("la captura salió vacía")
	}
	return Result{
		Text: "Captura de pantalla.",
		Images: []Image{{
			MIME: "image/png",
			Data: base64.StdEncoding.EncodeToString(data),
		}},
	}
}

func captureScreen(ctx context.Context) ([]byte, error) {
	switch runtime.GOOS {
	case "windows":
		return captureWindows(ctx)
	case "linux":
		return captureLinux(ctx)
	default:
		return nil, fmt.Errorf("screenshot no está implementado para %s", runtime.GOOS)
	}
}

func captureLinux(ctx context.Context) ([]byte, error) {
	if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		grim, err := exec.LookPath("grim")
		if err != nil {
			return nil, fmt.Errorf("grim no está instalado; en Arch/CachyOS: sudo pacman -S grim")
		}
		out, err := runCapture(ctx, grim, "-")
		if err != nil {
			return nil, fmt.Errorf("grim falló: %w", err)
		}
		return out, nil
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) != "" {
		if scrot, err := exec.LookPath("scrot"); err == nil {
			if out, err := runCapture(ctx, scrot, "-o", "-"); err == nil && len(out) > 0 {
				return out, nil
			}
		}
		if imp, err := exec.LookPath("import"); err == nil {
			if out, err := runCapture(ctx, imp, "-window", "root", "png:-"); err == nil && len(out) > 0 {
				return out, nil
			}
		}
		return nil, fmt.Errorf("no pude capturar X11; instala scrot (sudo pacman -S scrot) o imagemagick")
	}
	return nil, fmt.Errorf("no hay sesión gráfica (WAYLAND_DISPLAY o DISPLAY); no puedo capturar la pantalla")
}

func captureWindows(ctx context.Context) ([]byte, error) {
	tmp, err := os.CreateTemp("", "nosecode-shot-*.png")
	if err != nil {
		return nil, err
	}
	path := tmp.Name()
	_ = tmp.Close()
	defer os.Remove(path)

	script := fmt.Sprintf(`$ErrorActionPreference='Stop'
Add-Type -AssemblyName System.Windows.Forms
Add-Type -AssemblyName System.Drawing
$r=[System.Windows.Forms.SystemInformation]::VirtualScreen
$bmp=New-Object System.Drawing.Bitmap $r.Width,$r.Height
$g=[System.Drawing.Graphics]::FromImage($bmp)
$g.CopyFromScreen($r.Left,$r.Top,0,0,$bmp.Size)
$bmp.Save('%s',[System.Drawing.Imaging.ImageFormat]::Png)
$g.Dispose()
$bmp.Dispose()`, strings.ReplaceAll(path, "'", "''"))

	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", script)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.CombinedOutput()
	if err != nil {
		text := strings.TrimSpace(string(out))
		if text != "" {
			return nil, fmt.Errorf("PowerShell no pudo capturar la pantalla: %s", oneLine(text, 300))
		}
		return nil, fmt.Errorf("PowerShell no pudo capturar la pantalla: %w", err)
	}
	return os.ReadFile(path)
}

func runCapture(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.WaitDelay = 2 * time.Second
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return nil, fmt.Errorf("la captura superó %s", captureTimeout)
		}
		if ee, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("%s: %s", name, oneLine(string(ee.Stderr), 240))
		}
		return nil, err
	}
	return out, nil
}

func runViewImage(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		Path string `json:"path"`
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
	st, err := os.Stat(path)
	if err != nil {
		return errResult("no pude abrir %s: %v", path, err)
	}
	if st.Size() > maxImageBytes {
		return errResult("%s es demasiado grande (%d bytes; máximo %d)", path, st.Size(), maxImageBytes)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return errResult("no pude leer %s: %v", path, err)
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return errResult("%s no parece una imagen (%s)", path, mime)
	}
	return Result{
		Text: "Imagen: " + path,
		Images: []Image{{
			MIME: mime,
			Data: base64.StdEncoding.EncodeToString(data),
		}},
	}
}

func oneLine(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	if len(s) > max {
		s = s[:max] + "…"
	}
	return s
}
