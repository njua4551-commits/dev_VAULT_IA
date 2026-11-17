package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"time"
)

// Job es un proceso que sigue vivo en segundo plano (servidor, watch, etc.).
type Job struct {
	ID      int
	Command string
	Proc    *BashProcess
	Started time.Time
}

var (
	jobsMu   sync.Mutex
	jobsList = map[int]*Job{}
	jobsNext = 1
)

// RegisterJob guarda un proceso vivo y devuelve su id.
func RegisterJob(command string, p *BashProcess) int {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	id := jobsNext
	jobsNext++
	jobsList[id] = &Job{ID: id, Command: command, Proc: p, Started: time.Now()}
	return id
}

// Jobs devuelve los jobs ordenados por id.
func Jobs() []*Job {
	jobsMu.Lock()
	defer jobsMu.Unlock()
	out := make([]*Job, 0, len(jobsList))
	for _, j := range jobsList {
		out = append(out, j)
	}
	sort.Slice(out, func(i, k int) bool { return out[i].ID < out[k].ID })
	return out
}

// KillJobs termina todo proceso de bash que siga vivo (trabajos en segundo
// plano, comandos en primer plano esperando en el aviso de timeout, etc.).
// Se llama al salir y al recibir SIGHUP/SIGTERM para no dejar residuos.
func KillJobs() {
	for _, p := range liveProcs() {
		p.Kill()
	}
}

var (
	liveMu  sync.Mutex
	liveSet = map[*BashProcess]struct{}{}
)

func trackLive(p *BashProcess) {
	liveMu.Lock()
	liveSet[p] = struct{}{}
	liveMu.Unlock()
}

func untrackLive(p *BashProcess) {
	liveMu.Lock()
	delete(liveSet, p)
	liveMu.Unlock()
}

func liveProcs() []*BashProcess {
	liveMu.Lock()
	defer liveMu.Unlock()
	out := make([]*BashProcess, 0, len(liveSet))
	for p := range liveSet {
		out = append(out, p)
	}
	return out
}

// LiveCount dice cuántos procesos de bash siguen vivos.
func LiveCount() int { return len(liveProcs()) }

func (j *Job) running() bool {
	select {
	case <-j.Proc.Done():
		return false
	default:
		return true
	}
}

func (j *Job) status() string {
	if j.running() {
		return "en ejecución"
	}
	return fmt.Sprintf("terminó (código %d)", j.Proc.ExitCode())
}

// backgroundResult espera un instante por si el comando termina solo y, si no,
// lo registra como job.
func backgroundResult(ctx context.Context, proc *BashProcess, command string) Result {
	select {
	case <-proc.Done():
		return finalBashResult(proc, 0)
	case <-ctx.Done():
		proc.Kill()
		return errResult("cancelado por el usuario")
	case <-time.After(2 * time.Second):
	}
	id := RegisterJob(command, proc)
	text := proc.Output()
	if text != "" {
		text += "\n"
	}
	text += fmt.Sprintf("Sigue en segundo plano como job %d. Usa jobs {action:\"output\",id:%d} para ver su salida o {action:\"kill\",id:%d} para pararlo.", id, id, id)
	return Result{Text: text, Bytes: len(text)}
}

// JobNote es el texto que recibe el modelo cuando el usuario manda a segundo
// plano un comando que seguía vivo tras el timeout.
func JobNote(id int) string {
	return fmt.Sprintf("El comando sigue en segundo plano como job %d (la salida parcial está arriba). Usa jobs {action:\"output\",id:%d} para ver más o {action:\"kill\",id:%d} para pararlo.", id, id, id)
}

type jobsArgs struct {
	Action string `json:"action"`
	ID     int    `json:"id"`
}

func runJobs(_ context.Context, raw json.RawMessage) Result {
	var a jobsArgs
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	jobsMu.Lock()
	byID := func(id int) *Job { return jobsList[id] }
	jobsMu.Unlock()
	switch a.Action {
	case "list", "":
		list := Jobs()
		if len(list) == 0 {
			return Result{Text: "(no hay procesos en segundo plano)"}
		}
		var sb strings.Builder
		for _, j := range list {
			fmt.Fprintf(&sb, "job %d · %s · %s · $ %s\n", j.ID, j.status(), time.Since(j.Started).Round(time.Second), j.Command)
		}
		return Result{Text: strings.TrimRight(sb.String(), "\n")}
	case "output", "kill":
		j := byID(a.ID)
		if j == nil {
			return errResult("no existe el job %d", a.ID)
		}
		if a.Action == "kill" {
			j.Proc.Kill()
			return Result{Text: fmt.Sprintf("job %d detenido", j.ID)}
		}
		out := j.Proc.Output()
		if out == "" {
			out = "(sin salida todavía)"
		}
		return Result{Text: fmt.Sprintf("job %d · %s\n%s", j.ID, j.status(), out)}
	}
	return errResult("acción desconocida %q (list, output o kill)", a.Action)
}
