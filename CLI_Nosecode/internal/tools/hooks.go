package tools

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// RunHooks ejecuta comandos definidos por el usuario (hooks.after_edit)
// sustituyendo {file}. Devuelve un resumen solo si algo falló o imprimió; así
// un hook que va bien no gasta contexto.
func RunHooks(cmds []string, file string) string {
	var out strings.Builder
	for _, tpl := range cmds {
		tpl = strings.TrimSpace(tpl)
		if tpl == "" {
			continue
		}
		command := strings.ReplaceAll(tpl, "{file}", shellQuote(file))
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-Command", command)
		} else {
			c = exec.CommandContext(ctx, "sh", "-c", command)
		}
		c.Env = nonInteractiveEnv()
		res, err := c.CombinedOutput()
		cancel()
		text := strings.TrimSpace(string(res))
		if err == nil && text == "" {
			continue
		}
		status := "ok"
		if err != nil {
			status = "falló: " + err.Error()
		}
		out.WriteString("\n[hook " + tpl + " → " + status + "]")
		if text != "" {
			out.WriteString("\n" + TruncateText(text, 1500))
		}
	}
	return out.String()
}

func shellQuote(s string) string {
	if runtime.GOOS == "windows" {
		return "'" + strings.ReplaceAll(s, "'", "''") + "'"
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}
