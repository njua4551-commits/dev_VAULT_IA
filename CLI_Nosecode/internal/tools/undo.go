package tools

import (
	"crypto/sha256"
	"fmt"
	"os"
	"sync"
)

// Pila de deshacer para write_file y edit_file. Vive en memoria mientras
// Nosecode está abierto; guarda el contenido anterior de cada archivo tocado.

const (
	undoMaxEntries = 50
	undoMaxBytes   = 2 << 20 // no se guardan archivos de más de 2 MB
)

type undoEntry struct {
	path    string
	existed bool
	before  []byte
	after   [32]byte // hash del contenido que dejó la herramienta
	tooBig  bool
}

var (
	undoMu    sync.Mutex
	undoStack []undoEntry
	redoStack []undoEntry
)

// snapshotBefore guarda el estado actual antes de escribir. Devuelve una
// función que registra el hash del resultado.
func snapshotBefore(path string) func() {
	e := undoEntry{path: path}
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() {
		e.existed = true
		if info.Size() > undoMaxBytes {
			e.tooBig = true
		} else if data, err := os.ReadFile(path); err == nil {
			e.before = data
		}
	}
	return func() {
		if data, err := os.ReadFile(path); err == nil {
			e.after = sha256.Sum256(data)
		}
		undoMu.Lock()
		defer undoMu.Unlock()
		undoStack = append(undoStack, e)
		if len(undoStack) > undoMaxEntries {
			undoStack = undoStack[len(undoStack)-undoMaxEntries:]
		}
		redoStack = nil
	}
}

// UndoList describe los cambios que se pueden deshacer (el más reciente primero).
func UndoList() []string {
	undoMu.Lock()
	defer undoMu.Unlock()
	var out []string
	for i := len(undoStack) - 1; i >= 0; i-- {
		e := undoStack[i]
		kind := "editado"
		if !e.existed {
			kind = "creado"
		}
		out = append(out, fmt.Sprintf("%s (%s)", e.path, kind))
	}
	return out
}

// Undo revierte el último cambio. Si el archivo cambió después (por ejemplo, a
// mano), se niega salvo que force sea true.
func Undo(force bool) (string, error) {
	undoMu.Lock()
	defer undoMu.Unlock()
	if len(undoStack) == 0 {
		return "", fmt.Errorf("no hay cambios que deshacer")
	}
	e := undoStack[len(undoStack)-1]
	if e.tooBig {
		return "", fmt.Errorf("%s era demasiado grande para guardarlo (>2 MB); no se puede deshacer", e.path)
	}
	cur, err := os.ReadFile(e.path)
	if err == nil && sha256.Sum256(cur) != e.after && !force {
		return "", fmt.Errorf("%s cambió después de la edición del agente; usa /undo force para sobrescribir", e.path)
	}
	redo := undoEntry{path: e.path, existed: err == nil, before: cur}
	if e.existed {
		if err := atomicWrite(e.path, e.before, 0o644); err != nil {
			return "", fmt.Errorf("no pude restaurar %s: %w", e.path, err)
		}
		redo.after = sha256.Sum256(e.before)
	} else {
		if err := os.Remove(e.path); err != nil && !os.IsNotExist(err) {
			return "", fmt.Errorf("no pude borrar %s: %w", e.path, err)
		}
		redo.after = sha256.Sum256(nil)
	}
	undoStack = undoStack[:len(undoStack)-1]
	redoStack = append(redoStack, redo)
	if e.existed {
		return "restaurado " + e.path, nil
	}
	return "eliminado " + e.path + " (lo había creado el agente)", nil
}

// Redo repite el último cambio deshecho.
func Redo() (string, error) {
	undoMu.Lock()
	defer undoMu.Unlock()
	if len(redoStack) == 0 {
		return "", fmt.Errorf("no hay nada que rehacer")
	}
	r := redoStack[len(redoStack)-1]
	cur, err := os.ReadFile(r.path)
	if err == nil && sha256.Sum256(cur) != r.after {
		return "", fmt.Errorf("%s cambió desde el /undo; no se rehace", r.path)
	}
	back := undoEntry{path: r.path, existed: err == nil, before: cur}
	if r.existed {
		if err := atomicWrite(r.path, r.before, 0o644); err != nil {
			return "", err
		}
		back.after = sha256.Sum256(r.before)
	} else {
		_ = os.Remove(r.path)
		back.after = sha256.Sum256(nil)
	}
	redoStack = redoStack[:len(redoStack)-1]
	undoStack = append(undoStack, back)
	return "rehecho " + r.path, nil
}

// resetUndoForTest vacía las pilas (solo pruebas).
func resetUndoForTest() {
	undoMu.Lock()
	undoStack, redoStack = nil, nil
	undoMu.Unlock()
}
