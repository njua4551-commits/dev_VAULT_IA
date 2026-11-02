// Package session guarda las conversaciones locales de Nosecode.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"nosecode/internal/config"
)

// Message es un turno de la conversacion.
type Message struct {
	Role string `json:"role"`
	Text string `json:"text"`
}

// Session es una conversacion persistida en ~/.nosecode/sessions.
type Session struct {
	ID          string    `json:"id"`
	Title       string    `json:"title"`
	TitleEdited bool      `json:"title_edited,omitempty"`
	Created     time.Time `json:"created"`
	Updated     time.Time `json:"updated"`
	Messages    []Message `json:"messages"`
}

// Dir devuelve la carpeta de sesiones.
func Dir() string { return filepath.Join(config.Home(), "sessions") }

// New crea una sesion vacia en memoria (aun no guardada).
func New() Session {
	now := time.Now()
	return Session{
		ID:      fmt.Sprintf("s-%d", now.UnixNano()),
		Title:   "Nueva sesión",
		Created: now,
		Updated: now,
	}
}

func validID(id string) bool {
	if !strings.HasPrefix(id, "s-") || len(id) < 3 {
		return false
	}
	for _, r := range id[2:] {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// List devuelve las sesiones ordenadas por fecha de creacion.
func List() ([]Session, error) {
	entries, err := os.ReadDir(Dir())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	out := make([]Session, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".json") {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(Dir(), entry.Name()))
		if err != nil {
			continue
		}
		var s Session
		if json.Unmarshal(raw, &s) != nil || s.ID == "" || !validID(s.ID) {
			continue
		}
		out = append(out, s)
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Created.Before(out[j].Created) })
	return out, nil
}

// Save escribe la sesion en disco.
func (s Session) Save() error {
	if !validID(s.ID) {
		return fmt.Errorf("id de sesion invalido: %q", s.ID)
	}
	if err := os.MkdirAll(Dir(), 0o700); err != nil {
		return err
	}
	if s.Title == "" {
		s.Title = "Nueva sesión"
	}
	raw, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	raw = append(raw, '\n')
	return os.WriteFile(filepath.Join(Dir(), s.ID+".json"), raw, 0o600)
}

// Load lee una sesion concreta.
func Load(id string) (Session, error) {
	var s Session
	if !validID(id) {
		return s, fmt.Errorf("id de sesion invalido: %q", id)
	}
	raw, err := os.ReadFile(filepath.Join(Dir(), id+".json"))
	if err != nil {
		return s, err
	}
	err = json.Unmarshal(raw, &s)
	return s, err
}

// Delete borra una sesion del disco.
func Delete(id string) error {
	if !validID(id) {
		return fmt.Errorf("id de sesion invalido: %q", id)
	}
	err := os.Remove(filepath.Join(Dir(), id+".json"))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

// TitleFrom resume el primer mensaje para usarlo como titulo de la sesion.
func TitleFrom(text string) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return "Nueva sesión"
	}
	line := strings.SplitN(text, "\n", 2)[0]
	line = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) {
			return ' '
		}
		return r
	}, line))
	runes := []rune(line)
	if len(runes) > 32 {
		line = strings.TrimSpace(string(runes[:31])) + "…"
	}
	if line == "" {
		return "Nueva sesión"
	}
	return line
}
