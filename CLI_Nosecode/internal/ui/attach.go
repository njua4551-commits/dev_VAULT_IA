package ui

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"nosecode/internal/api"
	"nosecode/internal/config"
	"nosecode/internal/images"
)

// Imágenes que el usuario manda al modelo. Se guardan ya reducidas (JPEG,
// máx. 1024 px) en ~/.nosecode/chat-images; la sesión solo guarda la ruta.

const (
	maxAttachments  = 4
	keepImagesInCtx = 2 // solo las imágenes de los últimos mensajes viajan al modelo
	thumbCols       = 30
	thumbRows       = 8
)

var imageExts = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".gif": true}

func chatImagesDir() string { return filepath.Join(config.Home(), "chat-images") }

// storeImage reduce la imagen, la guarda y devuelve su ruta.
func storeImage(raw []byte) (string, error) {
	_, data, err := images.Prepare(raw, "")
	if err != nil {
		return "", err
	}
	dir := chatImagesDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	path := filepath.Join(dir, hex.EncodeToString(sum[:6])+".jpg")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// attachFile añade una imagen del disco a lo que se enviará.
func (m Model) attachFile(src string) (Model, error) {
	if len(m.attachments) >= maxAttachments {
		return m, fmt.Errorf("máximo %d imágenes por mensaje", maxAttachments)
	}
	info, err := os.Stat(src)
	if err != nil || info.IsDir() {
		return m, fmt.Errorf("no encontré el archivo %s", src)
	}
	if info.Size() > 25<<20 {
		return m, fmt.Errorf("la imagen pesa más de 25 MB")
	}
	raw, err := os.ReadFile(src)
	if err != nil {
		return m, err
	}
	path, err := storeImage(raw)
	if err != nil {
		return m, err
	}
	m.attachments = append(append([]string(nil), m.attachments...), path)
	return m, nil
}

// extractImagePaths separa del texto las rutas de imágenes existentes (lo que
// pega una terminal al arrastrar un archivo: con comillas, con \espacios o
// como file://). Devuelve el texto sin ellas y las rutas.
func extractImagePaths(text string) (string, []string) {
	var keep, paths []string
	for _, tok := range splitShellish(text) {
		p := tok
		if strings.HasPrefix(p, "file://") {
			if u, err := url.Parse(p); err == nil {
				p = u.Path
			}
		}
		if strings.HasPrefix(p, "~/") {
			if home, err := os.UserHomeDir(); err == nil {
				p = filepath.Join(home, p[2:])
			}
		}
		if imageExts[strings.ToLower(filepath.Ext(p))] {
			if info, err := os.Stat(p); err == nil && !info.IsDir() {
				paths = append(paths, p)
				continue
			}
		}
		keep = append(keep, tok)
	}
	return strings.Join(keep, " "), paths
}

// splitShellish parte por espacios respetando comillas y «\ ».
func splitShellish(s string) []string {
	var out []string
	var cur strings.Builder
	var quote rune
	esc := false
	flush := func() {
		if cur.Len() > 0 {
			out = append(out, cur.String())
			cur.Reset()
		}
	}
	for _, r := range s {
		switch {
		case esc:
			cur.WriteRune(r)
			esc = false
		case r == '\\' && quote != '\'':
			esc = true
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				cur.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n':
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	flush()
	return out
}

func isPNG(b []byte) bool { return bytes.HasPrefix(b, []byte("\x89PNG")) }

// clipboardImage lee una imagen del portapapeles (Wayland, X11 o Windows).
func clipboardImage() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	var attempts [][]string
	switch runtime.GOOS {
	case "windows":
		ps := `Add-Type -AssemblyName System.Windows.Forms; $i=[System.Windows.Forms.Clipboard]::GetImage(); ` +
			`if($i){$m=New-Object System.IO.MemoryStream; $i.Save($m,[System.Drawing.Imaging.ImageFormat]::Png); ` +
			`[Console]::OpenStandardOutput().Write($m.ToArray(),0,$m.Length)}`
		attempts = [][]string{{"powershell.exe", "-NoProfile", "-STA", "-Command", ps}}
	case "darwin":
		return nil, fmt.Errorf("en macOS usa /img <ruta> o arrastra el archivo")
	default:
		attempts = [][]string{
			{"wl-paste", "--type", "image/png"},
			{"xclip", "-selection", "clipboard", "-t", "image/png", "-o"},
		}
	}
	for _, a := range attempts {
		if _, err := exec.LookPath(a[0]); err != nil {
			continue
		}
		out, err := exec.CommandContext(ctx, a[0], a[1:]...).Output()
		if err == nil && isPNG(out) {
			return out, nil
		}
	}
	return nil, fmt.Errorf("no hay una imagen en el portapapeles (necesita wl-clipboard o xclip)")
}

// imgSlash maneja /img <ruta> · /img (portapapeles) · /img clear.
func (m Model) imgSlash(rest string) Model {
	arg := strings.TrimSpace(rest)
	switch strings.ToLower(arg) {
	case "clear", "limpiar", "quitar":
		m.attachments = nil
		m.toast, m.toastErr = "Imágenes adjuntas quitadas", false
		return m
	case "":
		raw, err := clipboardImage()
		if err != nil {
			m.toast, m.toastErr = err.Error(), true
			return m
		}
		path, err := storeImage(raw)
		if err != nil {
			m.toast, m.toastErr = err.Error(), true
			return m
		}
		m.attachments = append(append([]string(nil), m.attachments...), path)
		m.toast, m.toastErr = fmt.Sprintf("Imagen pegada (%d adjunta(s)) · escribe tu mensaje y enter", len(m.attachments)), false
		return m
	}
	_, paths := extractImagePaths(arg)
	if len(paths) == 0 {
		paths = []string{strings.Trim(arg, `"'`)}
	}
	for _, p := range paths {
		var err error
		if m, err = m.attachFile(p); err != nil {
			m.toast, m.toastErr = err.Error(), true
			return m
		}
	}
	m.toast, m.toastErr = fmt.Sprintf("%d imagen(es) adjunta(s) · escribe tu mensaje y enter", len(m.attachments)), false
	return m
}

// pasteImageKey: ctrl+v pega una imagen si el portapapeles la tiene; si no, devuelve false.
func (m Model) pasteImageKey() (Model, bool) {
	raw, err := clipboardImage()
	if err != nil {
		return m, false
	}
	path, err := storeImage(raw)
	if err != nil || len(m.attachments) >= maxAttachments {
		return m, false
	}
	m.attachments = append(append([]string(nil), m.attachments...), path)
	m.toast, m.toastErr = fmt.Sprintf("Imagen pegada (%d adjunta(s)) · escribe tu mensaje y enter", len(m.attachments)), false
	return m, true
}

// ---------------------------------------------------------- contenido API ---

func imageDataURL(path string) (string, bool) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	return "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(data), true
}

// userContent arma el contenido de un mensaje de usuario: texto o texto+imágenes.
func userContent(text string, imgs []string, withImages bool) any {
	if len(imgs) == 0 {
		return text
	}
	if !withImages {
		return text + fmt.Sprintf("\n[%d imagen(es) enviada(s) antes; ya no se reenvían]", len(imgs))
	}
	parts := []api.ContentPart{{Type: "text", Text: text}}
	for _, p := range imgs {
		if u, ok := imageDataURL(p); ok {
			parts = append(parts, api.ContentPart{Type: "image_url", ImageURL: &api.ImageURL{URL: u}})
		}
	}
	return parts
}

// ------------------------------------------------------------- miniaturas ---

var thumbCache = struct {
	sync.Mutex
	m map[string]thumbEntry
}{m: map[string]thumbEntry{}}

type thumbEntry struct {
	rows []string
	cols int
}

// thumb devuelve las filas ANSI de una imagen (con caché por ruta y tamaño).
func thumb(path string, cols, rows int) ([]string, int, bool) {
	key := path + "|" + strconv.Itoa(cols) + "x" + strconv.Itoa(rows)
	thumbCache.Lock()
	if e, ok := thumbCache.m[key]; ok {
		thumbCache.Unlock()
		return e.rows, e.cols, len(e.rows) > 0
	}
	thumbCache.Unlock()
	data, err := os.ReadFile(path)
	var r []string
	var w int
	if err == nil {
		r, w, _ = images.Thumbnail(data, cols, rows)
	}
	thumbCache.Lock()
	if len(thumbCache.m) > 64 {
		thumbCache.m = map[string]thumbEntry{}
	}
	thumbCache.m[key] = thumbEntry{rows: r, cols: w}
	thumbCache.Unlock()
	return r, w, len(r) > 0
}

// ----------------------------------------------------------------- visor ---

// imgViewState es el visor grande de imágenes.
type imgViewState struct {
	list []string
	idx  int
	zoom int // porcentaje: 50..100
}

func (m Model) openImageView(list []string, idx int) Model {
	if len(list) == 0 {
		return m
	}
	m.imgView = imgViewState{list: list, idx: idx, zoom: 100}
	m.screen = screenImage
	return m
}

// lastChatImages devuelve las imágenes del último mensaje que las tenga.
func (m Model) lastChatImages() []string {
	for i := len(m.msgs) - 1; i >= 0; i-- {
		if len(m.msgs[i].Images) > 0 {
			return m.msgs[i].Images
		}
	}
	return nil
}

func (m Model) imageKey(k tea.KeyMsg) (tea.Model, tea.Cmd) {
	v := m.imgView
	switch k.String() {
	case "esc", "q", "enter", "ctrl+g":
		m.screen = screenChat
		m.imgView = imgViewState{}
		return m, nil
	case "left", "h":
		if v.idx > 0 {
			v.idx--
		}
	case "right", "l":
		if v.idx < len(v.list)-1 {
			v.idx++
		}
	case "+", "=":
		if v.zoom < 100 {
			v.zoom += 25
		}
	case "-", "_":
		if v.zoom > 50 {
			v.zoom -= 25
		}
	}
	m.imgView = v
	return m, nil
}

func (m Model) imageScreen() []uiLine {
	v := m.imgView
	if len(v.list) == 0 {
		return nil
	}
	maxCols := clamp(m.width-10, 20, 120) * v.zoom / 100
	maxRows := clamp(m.height-10, 6, 50) * v.zoom / 100
	rows, w, ok := thumb(v.list[v.idx], maxCols, maxRows)
	boxW := clamp(w+4, 40, m.width-2)
	head := []uiLine{{text: modalTitleStyle.Render(fmt.Sprintf("Imagen %d/%d · %s", v.idx+1, len(v.list), filepath.Base(v.list[v.idx])))}}
	var body []uiLine
	if !ok {
		body = append(body, uiLine{text: warningStyle.Render("No pude leer la imagen")})
	}
	for _, r := range rows {
		body = append(body, uiLine{text: r})
	}
	footer := "←/→ cambiar · +/- tamaño · esc cerrar"
	return m.centerModal(m.modalFrame(boxW, head, body, footer), boxW)
}
