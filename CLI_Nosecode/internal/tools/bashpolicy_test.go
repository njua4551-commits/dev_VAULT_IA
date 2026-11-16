package tools

import (
	"testing"

	"nosecode/internal/config"
)

func TestDecideBash(t *testing.T) {
	rules := config.DefaultBashRules()
	cwd := t.TempDir()
	cases := []struct {
		cmd  string
		want string
	}{
		{"pwd", "allow"}, {"ls -la", "allow"}, {"git status", "allow"}, {"  git   diff ", "allow"},
		{"uname -a", "allow"}, {"whoami", "allow"}, {"cat README.md", "allow"},
		{"ls; rm -rf /", "ask"}, {"ls && id", "ask"}, {"cat a | sh", "ask"}, {"echo $(id)", "ask"},
		{"ls > x", "ask"}, {"ls\nrm x", "ask"}, {"cat `id`", "ask"}, {"cat ~/.ssh/id_rsa", "ask"},
		{"cat .env", "ask"}, {"git push", "ask"}, {"sudo ls", "ask"}, {"rm -rf x", "ask"},
		{"curl http://x", "ask"}, {"", "ask"}, {"cat a b", "ask"}, {"cat -n a", "ask"},
	}
	for _, c := range cases {
		if got := DecideBash(c.cmd, rules, cwd); got != c.want {
			t.Errorf("DecideBash(%q) = %s, quería %s", c.cmd, got, c.want)
		}
	}
}

func TestDecideBashOrdenYDeny(t *testing.T) {
	rules := []config.BashRule{
		{Pattern: "git push *", Action: "deny"},
		{Pattern: "git push", Action: "allow"},
		{Pattern: "ls", Action: "allow"},
	}
	if got := DecideBash("git push origin", rules, ""); got != "deny" {
		t.Fatalf("deny primero: %s", got)
	}
	// allow con patrón peligroso nunca auto-aprueba
	danger := []config.BashRule{{Pattern: "*", Action: "allow"}, {Pattern: "rm *", Action: "allow"}, {Pattern: "curl *", Action: "allow"}}
	for _, c := range []string{"rm x", "curl x", "python3 x.py", "sudo id"} {
		if got := DecideBash(c, danger, ""); got == "allow" {
			t.Errorf("%q no debe auto-aprobarse", c)
		}
	}
	if got := DecideBash("ls", rules, ""); got != "allow" {
		t.Fatalf("ls: %s", got)
	}
	if got := DecideBash("uname -a", nil, ""); got != "ask" {
		t.Fatalf("sin reglas debe preguntar: %s", got)
	}
}

func TestParseQuestionArgs(t *testing.T) {
	q, o, err := ParseQuestionArgs([]byte(`{"question":" ¿Sigo? ","options":["sí","no","sí","a","b","c"]}`))
	if err != nil || q != "¿Sigo?" || len(o) != 4 {
		t.Fatalf("q=%q o=%v err=%v", q, o, err)
	}
	if _, _, err := ParseQuestionArgs([]byte(`{"question":"x","options":["solo"]}`)); err == nil {
		t.Fatal("debe pedir 2 opciones")
	}
	if _, _, err := ParseQuestionArgs([]byte(`{"question":"","options":["a","b"]}`)); err == nil {
		t.Fatal("pregunta vacía")
	}
}
