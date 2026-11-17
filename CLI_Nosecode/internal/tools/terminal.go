package tools

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Herramienta «terminal»: sesiones con un PTY real para programas interactivos
// (python, node, psql, instaladores con preguntas, editores simples…). El modelo
// ve la pantalla ya renderizada (texto) y manda texto o teclas.

const (
	termMaxSessions = 4
	termDefCols     = 100
	termDefRows     = 30
	termMaxCols     = 200
	termMaxRows     = 60
	termQuiet       = 300 * time.Millisecond
	termDefWait     = 2 * time.Second
	termMaxWait     = 60 * time.Second
)

type termSession struct {
	id      int
	command string
	cols    int
	rows    int
	f       *os.File
	cmd     *exec.Cmd
	vt      *vt
	proc    *BashProcess // para trackLive/KillJobs
	started time.Time

	lastWrite atomic.Int64 // UnixNano de la última salida
	mu        sync.Mutex
	exited    bool
	exitCode  int
	done      chan struct{}
}

var terms struct {
	mu   sync.Mutex
	next int
	m    map[int]*termSession
}

func init() { terms.m = map[int]*termSession{} }

// terminalTool devuelve la herramienta (solo donde hay PTY).
func terminalTool() (Tool, bool) {
	if !ptySupported {
		return Tool{}, false
	}
	return Tool{
		Name: "terminal",
		Description: "Terminal interactiva (PTY) para programas que piden teclado: python/node/psql, instaladores con preguntas, editores. " +
			"action: open (command, cwd) | send (id, text y/o keys como Enter, Tab, Esc, Up, Down, Left, Right, Backspace, Ctrl-C, Ctrl-D) | read (id, until, history) | resize | close | list. " +
			"Devuelve la pantalla. Usa bash para comandos normales. Nunca escribas contraseñas: pídele al usuario que use /terminal.",
		Parameters: schema(`{"type":"object","properties":{"action":{"type":"string","enum":["open","send","read","resize","close","list"]},"id":{"type":"integer"},"command":{"type":"string","description":"open: programa a ejecutar; vacío = shell"},"cwd":{"type":"string"},"text":{"type":"string","description":"send: texto literal (\\n = Enter)"},"keys":{"type":"array","items":{"type":"string"}},"until":{"type":"string","description":"send/read: esperar a que aparezca este texto"},"wait_ms":{"type":"integer","description":"espera máxima (por defecto 2000, máx 60000)"},"history":{"type":"boolean","description":"read: incluir líneas que salieron de la pantalla"},"cols":{"type":"integer"},"rows":{"type":"integer"}},"required":["action"],"additionalProperties":false}`),
		Run:        runTerminal,
	}, true
}

type termArgs struct {
	Action  string   `json:"action"`
	ID      int      `json:"id"`
	Command string   `json:"command"`
	Cwd     string   `json:"cwd"`
	Text    string   `json:"text"`
	Keys    []string `json:"keys"`
	Until   string   `json:"until"`
	WaitMS  int      `json:"wait_ms"`
	History bool     `json:"history"`
	Cols    int      `json:"cols"`
	Rows    int      `json:"rows"`
}

func runTerminal(ctx context.Context, raw json.RawMessage) Result {
	var a termArgs
	if err := json.Unmarshal(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	a.Action = strings.ToLower(strings.TrimSpace(a.Action))
	if a.Action == "" && a.Command != "" {
		a.Action = "open"
	}
	wait := termDefWait
	if a.WaitMS > 0 {
		wait = min(time.Duration(a.WaitMS)*time.Millisecond, termMaxWait)
	}
	switch a.Action {
	case "open":
		return termOpen(ctx, a, wait)
	case "list":
		return termList()
	case "send", "read", "resize", "close":
		s, err := termGet(a.ID)
		if err != nil {
			return errResult("%v", err)
		}
		switch a.Action {
		case "send":
			return termSend(ctx, s, a, wait)
		case "read":
			if a.Until == "" {
				wait = min(wait, time.Second)
			}
			s.settle(ctx, 0, wait, a.Until)
			return termResult(s, a.History)
		case "resize":
			cols, rows := clampInt(a.Cols, 20, termMaxCols), clampInt(a.Rows, 5, termMaxRows)
			s.resize(cols, rows)
			s.settle(ctx, 100*time.Millisecond, time.Second, "")
			return termResult(s, false)
		default:
			return termClose(s)
		}
	default:
		return errResult("action debe ser open, send, read, resize, close o list")
	}
}

func termGet(id int) (*termSession, error) {
	terms.mu.Lock()
	defer terms.mu.Unlock()
	if id == 0 && len(terms.m) == 1 {
		for _, s := range terms.m {
			return s, nil
		}
	}
	s := terms.m[id]
	if s == nil {
		return nil, fmt.Errorf("no existe la terminal %d (usa action=list)", id)
	}
	return s, nil
}

func termOpen(ctx context.Context, a termArgs, wait time.Duration) Result {
	cols, rows := a.Cols, a.Rows
	if cols <= 0 {
		cols = termDefCols
	}
	if rows <= 0 {
		rows = termDefRows
	}
	cols, rows = clampInt(cols, 20, termMaxCols), clampInt(rows, 5, termMaxRows)
	dir := a.Cwd
	if dir == "" {
		dir, _ = os.Getwd()
	} else if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return errResult("cwd no es una carpeta: %s", dir)
	}

	terms.mu.Lock()
	for id, s := range terms.m { // libera las que ya terminaron
		if s.isExited() && len(terms.m) >= termMaxSessions {
			delete(terms.m, id)
		}
	}
	if len(terms.m) >= termMaxSessions {
		terms.mu.Unlock()
		return errResult("ya hay %d terminales abiertas; cierra alguna (action=close)", termMaxSessions)
	}
	terms.next++
	id := terms.next
	terms.mu.Unlock()

	shell := "sh"
	if p, err := exec.LookPath("bash"); err == nil {
		shell = p
	}
	args := []string{"--noprofile", "--norc", "-i"}
	label := "shell"
	if c := strings.TrimSpace(a.Command); c != "" {
		args, label = []string{"-c", c}, c
	}
	name, wargs, _, werr := sandboxWrap(shell, args)
	if werr != nil {
		return errResult("%v", werr)
	}
	cmd := exec.Command(name, wargs...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "PAGER=cat", "GIT_PAGER=cat", "PS1=$ ",
		"COLUMNS="+strconv.Itoa(cols), "LINES="+strconv.Itoa(rows))
	f, err := startPTY(cmd, cols, rows)
	if err != nil {
		return errResult("no pude abrir el PTY: %v", err)
	}
	s := &termSession{id: id, command: label, cols: cols, rows: rows, f: f, cmd: cmd, started: time.Now(), done: make(chan struct{})}
	s.vt = newVT(cols, rows, func(b []byte) { _, _ = f.Write(b) })
	s.proc = &BashProcess{cmd: cmd, done: make(chan struct{}), start: s.started}
	trackLive(s.proc)
	terms.mu.Lock()
	terms.m[id] = s
	terms.mu.Unlock()
	s.lastWrite.Store(time.Now().UnixNano())

	go func() {
		buf := make([]byte, 8192)
		for {
			n, err := f.Read(buf)
			if n > 0 {
				s.vt.Write(buf[:n])
				s.lastWrite.Store(time.Now().UnixNano())
			}
			if err != nil {
				return
			}
		}
	}()
	go func() {
		err := cmd.Wait()
		time.Sleep(150 * time.Millisecond) // deja leer la última salida
		s.mu.Lock()
		s.exited = true
		s.exitCode = exitCodeOf(err)
		s.mu.Unlock()
		killCommand(cmd) // lo que quedara en el grupo
		_ = f.Close()
		untrackLive(s.proc)
		close(s.proc.done)
		close(s.done)
	}()

	s.settle(ctx, 300*time.Millisecond, wait, a.Until)
	return termResult(s, false)
}

func (s *termSession) isExited() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.exited
}

func (s *termSession) resize(cols, rows int) {
	s.cols, s.rows = cols, rows
	_ = setPTYSize(s.f, cols, rows)
	s.vt.Resize(cols, rows)
}

// settle espera a que la pantalla deje de cambiar (o a que aparezca «until»,
// o a que el proceso termine), hasta maxWait.
func (s *termSession) settle(ctx context.Context, minWait, maxWait time.Duration, until string) {
	start := time.Now()
	for {
		if s.isExited() {
			return
		}
		el := time.Since(start)
		if until != "" && strings.Contains(s.vt.Text(), until) {
			return
		}
		quiet := time.Since(time.Unix(0, s.lastWrite.Load()))
		if until == "" && el >= minWait && quiet >= termQuiet {
			return
		}
		if el >= maxWait {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(25 * time.Millisecond):
		}
	}
}

var passwordPrompt = regexp.MustCompile(`(?i)(password|passphrase|contrase[ñn]a|\[sudo\]|\bpin\b)[^\n]{0,60}:\s*$`)

// termWaitingPassword dice si el cursor está tras un aviso de contraseña.
func (s *termSession) waitingPassword() bool {
	lines, row, _, _, _ := s.vt.Screen()
	if row-1 < 0 || row-1 >= len(lines) {
		return false
	}
	return passwordPrompt.MatchString(lines[row-1])
}

var keyMap = map[string]string{
	"enter": "\r", "return": "\r", "tab": "\t", "esc": "\x1b", "escape": "\x1b", "space": " ",
	"backspace": "\x7f", "bs": "\x7f", "delete": "\x1b[3~", "del": "\x1b[3~", "insert": "\x1b[2~",
	"up": "\x1b[A", "down": "\x1b[B", "right": "\x1b[C", "left": "\x1b[D",
	"home": "\x1b[H", "end": "\x1b[F", "pageup": "\x1b[5~", "pgup": "\x1b[5~", "pagedown": "\x1b[6~", "pgdn": "\x1b[6~",
	"shift-tab": "\x1b[Z", "btab": "\x1b[Z",
	"f1": "\x1bOP", "f2": "\x1bOQ", "f3": "\x1bOR", "f4": "\x1bOS",
	"f5": "\x1b[15~", "f6": "\x1b[17~", "f7": "\x1b[18~", "f8": "\x1b[19~",
	"f9": "\x1b[20~", "f10": "\x1b[21~", "f11": "\x1b[23~", "f12": "\x1b[24~",
}

// keyBytes traduce un nombre de tecla («Enter», «Ctrl-C», «Alt-x», «q»).
func keyBytes(k string) (string, bool) {
	t := strings.ToLower(strings.TrimSpace(k))
	if s, ok := keyMap[t]; ok {
		return s, true
	}
	for _, p := range []string{"ctrl-", "ctrl+", "c-", "^"} {
		if strings.HasPrefix(t, p) {
			r := strings.TrimPrefix(t, p)
			if len(r) == 1 {
				switch c := r[0]; {
				case c >= 'a' && c <= 'z':
					return string(rune(c - 'a' + 1)), true
				case c == '[':
					return "\x1b", true
				case c == '\\':
					return "\x1c", true
				case c == ']':
					return "\x1d", true
				case c == ' ':
					return "\x00", true
				}
			}
			return "", false
		}
	}
	for _, p := range []string{"alt-", "alt+", "m-", "meta-"} {
		if strings.HasPrefix(t, p) {
			if rest, ok := keyBytes(k[len(p):]); ok {
				return "\x1b" + rest, true
			}
			return "", false
		}
	}
	if len([]rune(k)) == 1 { // una letra o símbolo suelto
		return k, true
	}
	return "", false
}

func termSend(ctx context.Context, s *termSession, a termArgs, wait time.Duration) Result {
	if s.isExited() {
		return errResult("la terminal %d ya terminó; ábrela de nuevo", s.id)
	}
	if (a.Text != "" || len(a.Keys) > 0) && s.waitingPassword() {
		return errResult("el programa está pidiendo una contraseña. No la escribas tú ni la pidas en el chat: dile al usuario que la escriba él mismo con /terminal %d", s.id)
	}
	var out strings.Builder
	if a.Text != "" {
		out.WriteString(strings.ReplaceAll(strings.ReplaceAll(a.Text, "\r\n", "\r"), "\n", "\r"))
	}
	for _, k := range a.Keys {
		b, ok := keyBytes(k)
		if !ok {
			return errResult("tecla desconocida: %q (usa Enter, Tab, Esc, Up, Down, Left, Right, Backspace, Ctrl-C, F1…)", k)
		}
		out.WriteString(b)
	}
	if out.Len() > 0 {
		s.lastWrite.Store(time.Now().UnixNano())
		if _, err := s.f.WriteString(out.String()); err != nil {
			return errResult("no pude escribir en la terminal: %v", err)
		}
	}
	s.settle(ctx, 200*time.Millisecond, wait, a.Until)
	return termResult(s, a.History)
}

func termClose(s *termSession) Result {
	s.settle(context.Background(), 0, 50*time.Millisecond, "")
	res := termResult(s, false)
	killCommand(s.cmd)
	select {
	case <-s.done:
	case <-time.After(2 * time.Second):
	}
	terms.mu.Lock()
	delete(terms.m, s.id)
	terms.mu.Unlock()
	res.Text = strings.Replace(res.Text, "en ejecución", "cerrada", 1)
	return res
}

func termList() Result {
	terms.mu.Lock()
	ss := make([]*termSession, 0, len(terms.m))
	for _, s := range terms.m {
		ss = append(ss, s)
	}
	terms.mu.Unlock()
	if len(ss) == 0 {
		return Result{Text: "No hay terminales abiertas."}
	}
	sort.Slice(ss, func(i, j int) bool { return ss[i].id < ss[j].id })
	var b strings.Builder
	for _, s := range ss {
		fmt.Fprintf(&b, "%d  %s  %s\n", s.id, s.statusText(), s.command)
	}
	return Result{Text: strings.TrimRight(b.String(), "\n")}
}

func (s *termSession) statusText() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.exited {
		return "terminó (código " + strconv.Itoa(s.exitCode) + ")"
	}
	return "en ejecución"
}

// termResult arma la pantalla como texto para el modelo.
func termResult(s *termSession, history bool) Result {
	lines, row, col, alt, hidden := s.vt.Screen()
	var b strings.Builder
	fmt.Fprintf(&b, "[terminal %d · %s · %dx%d · %s]\n", s.id, s.command, s.cols, s.rows, s.statusText())
	hist := s.vt.History(0)
	if history && len(hist) > 0 {
		h := s.vt.History(200)
		b.WriteString("── historial ──\n" + strings.Join(h, "\n") + "\n── pantalla ──\n")
	}
	b.WriteString(strings.Join(lines, "\n"))
	b.WriteString("\n")
	tail := fmt.Sprintf("[cursor fila %d, columna %d", row, col)
	if hidden {
		tail += ", oculto"
	}
	if alt {
		tail += " · pantalla completa"
	}
	tail += "]"
	if !history && len(hist) > 0 {
		tail += fmt.Sprintf(" [%d líneas anteriores: read con history:true]", len(hist))
	}
	b.WriteString(tail)
	text := b.String()
	return Result{Text: text, Bytes: len(text)}
}

// ── API para la interfaz (/terminal) ──

// TermInfo describe una sesión abierta.
type TermInfo struct {
	ID      int
	Command string
	Status  string
	Exited  bool
}

// TermList devuelve las sesiones abiertas.
func TermList() []TermInfo {
	terms.mu.Lock()
	defer terms.mu.Unlock()
	var out []TermInfo
	for _, s := range terms.m {
		out = append(out, TermInfo{ID: s.id, Command: s.command, Status: s.statusText(), Exited: s.isExited()})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// TermScreen devuelve la pantalla actual de una sesión.
func TermScreen(id int) (lines []string, row, col int, exited bool, err error) {
	s, e := termGet(id)
	if e != nil {
		return nil, 0, 0, false, e
	}
	lines, row, col, _, _ = s.vt.Screen()
	return lines, row, col, s.isExited(), nil
}

// TermWrite envía bytes crudos (teclas del usuario) a una sesión.
func TermWrite(id int, data []byte) error {
	s, err := termGet(id)
	if err != nil {
		return err
	}
	if s.isExited() {
		return errors.New("la terminal terminó")
	}
	_, err = s.f.Write(data)
	return err
}

// TermClose cierra una sesión.
func TermClose(id int) {
	if s, err := termGet(id); err == nil {
		termClose(s)
	}
}

// CloseTerminals mata todas las sesiones (KillJobs ya mata los procesos).
func CloseTerminals() {
	terms.mu.Lock()
	for id, s := range terms.m {
		killCommand(s.cmd)
		delete(terms.m, id)
	}
	terms.mu.Unlock()
}

// TermOpen abre una sesión desde la interfaz (/terminal new) y devuelve su id.
func TermOpen(command string) (int, error) {
	if !ptySupported {
		return 0, errors.New("terminal no está disponible en Windows")
	}
	r := runTerminal(context.Background(), mustJSON(map[string]any{"action": "open", "command": command, "wait_ms": 300}))
	if r.Error {
		return 0, errors.New(r.Text)
	}
	terms.mu.Lock()
	defer terms.mu.Unlock()
	id := 0
	for k := range terms.m {
		id = max(id, k)
	}
	return id, nil
}

// TermResize adapta la sesión al tamaño de la ventana.
func TermResize(id, cols, rows int) {
	if s, err := termGet(id); err == nil {
		cols, rows = clampInt(cols, 20, termMaxCols), clampInt(rows, 5, termMaxRows)
		if cols != s.cols || rows != s.rows {
			s.resize(cols, rows)
		}
	}
}

func mustJSON(v any) json.RawMessage { b, _ := json.Marshal(v); return b }
