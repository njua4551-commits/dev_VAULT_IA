package tools

import (
	"bytes"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

// Shell persistente (Linux/macOS): un único bash vivo entre comandos, para que
// persistan cd, variables exportadas, entornos virtuales (source .venv/bin/activate),
// funciones y alias. Cada comando se ejecuta con «source» de un archivo temporal
// (con stdin en /dev/null) y termina con una línea centinela que lleva el código
// de salida y el directorio actual. No hay PTY: los programas interactivos siguen
// sin funcionar, igual que en el modo normal.
//
// Límites honestos: cancelar o pasar el timeout con «cancelar» mata el shell entero
// (se reinicia en el siguiente comando conservando el directorio, no las variables).

type exitStatus int

func (e exitStatus) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

type pshRun struct {
	token    string
	out      *limitedBuffer
	done     chan struct{}
	rc       int
	finished bool
}

type pshell struct {
	cmd      *exec.Cmd
	stdin    io.WriteCloser
	key      string
	sessProc *BashProcess

	mu       sync.Mutex
	cur      *pshRun
	dir      string
	dead     bool
	detached bool
}

var psh struct {
	mu       sync.Mutex
	enabled  bool
	s        *pshell
	lastDir  string
	lostNote bool // el shell anterior murió: el siguiente comando lo avisa
}

func init() { psh.enabled = true }

// SetPersistentShell activa o desactiva el shell persistente.
func SetPersistentShell(on bool) {
	psh.mu.Lock()
	psh.enabled = on
	psh.mu.Unlock()
	if !on {
		ResetShell()
	}
}

// PersistentShellOn dice si está activado.
func PersistentShellOn() bool {
	psh.mu.Lock()
	defer psh.mu.Unlock()
	return psh.enabled && runtime.GOOS != "windows"
}

// ShellStatus devuelve si hay un shell vivo y su directorio actual.
func ShellStatus() (alive bool, dir string) {
	psh.mu.Lock()
	defer psh.mu.Unlock()
	if psh.s == nil {
		return false, psh.lastDir
	}
	psh.s.mu.Lock()
	defer psh.s.mu.Unlock()
	return !psh.s.dead, psh.s.dir
}

// ResetShell mata el shell persistente (variables y entorno se pierden; el
// directorio se conserva para el siguiente).
func ResetShell() {
	psh.mu.Lock()
	s := psh.s
	psh.s = nil
	psh.mu.Unlock()
	if s != nil {
		s.kill()
	}
}

func (s *pshell) kill() {
	s.mu.Lock()
	s.dead = true
	s.mu.Unlock()
	killCommand(s.cmd)
}

func (s *pshell) busy() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.cur != nil && !s.cur.finished
}

func sandboxKey() string {
	mode, active, _ := SandboxState()
	sbMu.Lock()
	c := sbCfg
	sbMu.Unlock()
	return fmt.Sprintf("%s|%v|%v|%v", mode, active, c.NoNetwork, c.Writable)
}

func startShell(dir, key string) (*pshell, error) {
	bash, err := exec.LookPath("bash")
	if err != nil {
		return nil, errors.New("el shell persistente necesita bash en el PATH")
	}
	name, args, _, werr := sandboxWrap(bash, []string{"--noprofile", "--norc"})
	if werr != nil {
		return nil, werr
	}
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(name, args...)
	prepareCommand(cmd)
	cmd.Dir = dir
	cmd.Env = nonInteractiveEnv()
	cmd.Stdout, cmd.Stderr = w, w
	stdin, err := cmd.StdinPipe()
	if err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		r.Close()
		w.Close()
		return nil, err
	}
	w.Close() // solo el shell y sus hijos conservan el extremo de escritura
	s := &pshell{cmd: cmd, stdin: stdin, key: key, dir: dir}
	s.sessProc = &BashProcess{cmd: cmd, done: make(chan struct{}), start: time.Now()}
	trackLive(s.sessProc)
	go s.readLoop(r)
	go func() {
		err := cmd.Wait()
		time.Sleep(60 * time.Millisecond) // deja drenar la última salida
		rc := 0
		if err != nil {
			rc = exitCodeOf(err)
			if rc == 0 {
				rc = 1
			}
		}
		killCommand(cmd) // hijos que quedaran en el grupo
		r.Close()
		s.mu.Lock()
		s.dead = true
		cur := s.cur
		s.mu.Unlock()
		if cur != nil {
			s.finish(cur, maxInt(rc, 1), "", fmt.Sprintf("[el shell terminó (código %d); se reiniciará en el siguiente comando]", rc))
		}
		untrackLive(s.sessProc)
		close(s.sessProc.done)
	}()
	return s, nil
}

const sentinelByte = 1

// readLoop separa la salida de los comandos de las líneas centinela.
func (s *pshell) readLoop(r io.Reader) {
	buf := make([]byte, 0, 8192)
	chunk := make([]byte, 8192)
	midLine := false
	for {
		n, err := r.Read(chunk)
		if n > 0 {
			buf = append(buf, chunk[:n]...)
			for {
				i := bytes.IndexByte(buf, '\n')
				if i < 0 {
					break
				}
				line := buf[:i]
				buf = buf[i+1:]
				if !midLine && len(line) > 0 && line[0] == sentinelByte && s.handleSentinel(string(line)) {
					continue
				}
				s.write(append(append([]byte(nil), line...), '\n'))
				midLine = false
			}
			if len(buf) > 0 && buf[0] != sentinelByte { // salida parcial (prompts sin salto de línea)
				s.write(buf)
				buf = buf[:0]
				midLine = true
			}
		}
		if err != nil {
			return
		}
	}
}

func (s *pshell) write(p []byte) {
	s.mu.Lock()
	cur := s.cur
	s.mu.Unlock()
	if cur != nil && !cur.finished {
		_, _ = cur.out.Write(p)
	}
}

// handleSentinel interpreta «\x01NC:<token>:<rc>:<pwd>\x01».
func (s *pshell) handleSentinel(line string) bool {
	line = strings.TrimSuffix(strings.TrimPrefix(line, string(rune(sentinelByte))), string(rune(sentinelByte)))
	parts := strings.SplitN(line, ":", 4)
	if len(parts) != 4 || parts[0] != "NC" {
		return false
	}
	rc, err := strconv.Atoi(parts[2])
	if err != nil {
		return false
	}
	s.mu.Lock()
	cur := s.cur
	s.mu.Unlock()
	if cur == nil || cur.token != parts[1] {
		return true // centinela de una ejecución ya abandonada
	}
	s.finish(cur, rc, parts[3], "")
	return true
}

func (s *pshell) finish(cur *pshRun, rc int, dir, note string) {
	s.mu.Lock()
	if cur.finished {
		s.mu.Unlock()
		return
	}
	cur.finished = true
	cur.rc = rc
	prev := s.dir
	if dir != "" {
		s.dir = dir
	}
	detached := s.detached
	s.mu.Unlock()
	if dir != "" && dir != prev {
		_, _ = cur.out.Write([]byte("\n[directorio actual: " + dir + "]\n"))
		psh.mu.Lock()
		psh.lastDir = dir
		psh.mu.Unlock()
	}
	if note != "" {
		_, _ = cur.out.Write([]byte("\n" + note + "\n"))
	}
	close(cur.done)
	if detached {
		s.kill() // era un shell abandonado (comando mandado a segundo plano): ya no hace falta
	}
}

func newToken() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// persistentStart ejecuta command en el shell persistente y devuelve un
// BashProcess compatible con el resto del flujo (timeout suave, cancelar…).
func persistentStart(command string) (*BashProcess, error) {
	psh.mu.Lock()
	defer psh.mu.Unlock()
	key := sandboxKey()
	s := psh.s
	note := ""
	if s != nil {
		s.mu.Lock()
		dead := s.dead
		s.mu.Unlock()
		switch {
		case dead:
			note = "[el shell persistente se reinició: se conserva el directorio, no las variables]"
			psh.lastDir, _ = s.currentDir()
			s = nil
		case s.key != key:
			note = "[el shell persistente se reinició por un cambio de sandbox]"
			psh.lastDir, _ = s.currentDir()
			s.kill()
			s = nil
		case s.busy():
			// comando anterior mandado a segundo plano: ese shell queda para él
			s.mu.Lock()
			s.detached = true
			s.mu.Unlock()
			note = "[el shell anterior sigue ocupado con un comando en segundo plano; se abrió uno nuevo]"
			psh.lastDir, _ = s.currentDir()
			s = nil
		}
	}
	if s == nil {
		dir := psh.lastDir
		if dir == "" {
			dir, _ = os.Getwd()
		}
		if st, err := os.Stat(dir); err != nil || !st.IsDir() {
			dir, _ = os.Getwd()
		}
		ns, err := startShell(dir, key)
		if err != nil {
			return nil, err
		}
		s = ns
		psh.s = s
	}

	s.mu.Lock()
	run := &pshRun{token: newToken(), out: &limitedBuffer{max: 8 << 10}, done: make(chan struct{})}
	s.cur = run
	s.mu.Unlock()
	if note != "" {
		_, _ = run.out.Write([]byte(note + "\n"))
	}

	// El comando viaja en base64 y se evalúa en el shell vivo (sin archivos: funciona
	// igual dentro del sandbox, donde /tmp es otro tmpfs).
	enc := base64.StdEncoding.EncodeToString([]byte(command + "\n"))
	line := fmt.Sprintf("eval \"$(printf %%s %s | base64 -d)\" </dev/null; __nc_rc=$?; set +e +u +x; printf '\\n\\001NC:%s:%%d:%%s\\001\\n' \"$__nc_rc\" \"$PWD\"\n", enc, run.token)
	if _, err := io.WriteString(s.stdin, line); err != nil {
		s.kill()
		return nil, fmt.Errorf("no pude escribir en el shell persistente: %w", err)
	}

	proc := &BashProcess{cmd: s.cmd, out: run.out, done: make(chan struct{}), start: time.Now()}
	go func() {
		<-run.done
		if run.rc != 0 {
			proc.mu.Lock()
			proc.err = exitStatus(maxInt(run.rc, 1))
			proc.mu.Unlock()
		}
		close(proc.done)
	}()
	return proc, nil
}

func (s *pshell) currentDir() (string, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.dir, !s.dead
}

func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
