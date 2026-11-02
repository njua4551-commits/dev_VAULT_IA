package ui

import (
	"embed"
	"fmt"
	"os"
	"path/filepath"
)

// Sonidos incluidos (MIT): paquete @opencode-ai/ui.
//
//go:embed audio/*.mp3
var audioFiles embed.FS

const (
	soundDone       = "bip-bop-01.mp3"
	soundQuestion   = "bip-bop-03.mp3"
	soundPermission = "staplebops-06.mp3"
	soundError      = "nope-03.mp3"
	soundSuccess    = "yup-01.mp3"
)

// audioPath escribe el mp3 embebido en el directorio temporal y devuelve su
// ruta, para que lo pueda abrir el reproductor del sistema.
func audioPath(name string) (string, error) {
	data, err := audioFiles.ReadFile("audio/" + name)
	if err != nil {
		return "", err
	}
	dir := filepath.Join(os.TempDir(), "nosecode-audio")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(dir, name)
	if st, err := os.Stat(path); err == nil && st.Size() == int64(len(data)) {
		return path, nil
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

// bell es el respaldo cuando no hay reproductor disponible.
func bell() {
	fmt.Fprint(os.Stderr, "\a")
}
