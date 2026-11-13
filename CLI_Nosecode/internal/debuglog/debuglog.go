// Package debuglog escribe un registro rotativo en ~/.nosecode/debug.log para
// diagnosticar bloqueos. No guarda claves ni contenido del chat; los comandos
// de bash se registran redactados (tokens y contraseñas ocultos).
package debuglog

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"nosecode/internal/config"
)

const maxSize = 2 << 20 // 2 MB

var mu sync.Mutex

// Path devuelve la ruta del log activo.
func Path() string { return filepath.Join(config.Home(), "debug.log") }

// Logf añade una línea con fecha y hora. Nunca devuelve error a la interfaz.
func Logf(format string, args ...any) {
	line := time.Now().Format("2006-01-02 15:04:05.000") + " " + fmt.Sprintf(format, args...)
	mu.Lock()
	defer mu.Unlock()
	if err := os.MkdirAll(config.Home(), 0o700); err != nil {
		return
	}
	path := Path()
	if st, err := os.Stat(path); err == nil && st.Size() > maxSize {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer f.Close()
	_, _ = fmt.Fprintln(f, line)
}
