package tools

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestBashBackgroundYJobs(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	raw, _ := json.Marshal(map[string]any{"command": "echo hola; sleep 30", "background": true})
	res := runBash(context.Background(), raw)
	if res.Error || !strings.Contains(res.Text, "hola") || !strings.Contains(res.Text, "job ") {
		t.Fatalf("%+v", res)
	}
	jobs := Jobs()
	if len(jobs) == 0 {
		t.Fatal("sin jobs")
	}
	id := jobs[len(jobs)-1].ID
	out := runJobs(context.Background(), json.RawMessage(`{"action":"output","id":`+itoa(id)+`}`))
	if !strings.Contains(out.Text, "en ejecución") || !strings.Contains(out.Text, "hola") {
		t.Fatal(out.Text)
	}
	runJobs(context.Background(), json.RawMessage(`{"action":"kill","id":`+itoa(id)+`}`))
	<-jobs[len(jobs)-1].Proc.Done()
	list := runJobs(context.Background(), json.RawMessage(`{"action":"list"}`))
	if !strings.Contains(list.Text, "terminó") {
		t.Fatal(list.Text)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

func TestKillJobsMataTambienLosDePrimerPlano(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	before := LiveCount()
	raw, _ := json.Marshal(map[string]any{"command": "sleep 30", "timeout_s": 1})
	res := runBash(context.Background(), raw) // timeout suave: el proceso sigue vivo
	if !res.Soft || res.Process == nil {
		t.Fatalf("%+v", res)
	}
	if LiveCount() != before+1 {
		t.Fatalf("debía haber un proceso vivo más: %d → %d", before, LiveCount())
	}
	KillJobs()
	<-res.Process.Done()
	time.Sleep(50 * time.Millisecond)
	if LiveCount() != before {
		t.Fatalf("tras KillJobs no debe quedar nada vivo: %d", LiveCount())
	}
}
