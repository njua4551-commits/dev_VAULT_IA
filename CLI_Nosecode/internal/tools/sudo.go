package tools

import (
	"context"
	"regexp"
	"strings"
)

// Soporte de sudo con contraseña pedida por la propia TUI.
//
// La contraseña NUNCA va al modelo ni a disco: vive en memoria en la UI, viaja
// aquí por el context, se entrega a sudo por stdin (`sudo -S -p ''`) y se borra
// de cualquier salida antes de devolverla.

type sudoKey struct{}

// WithSudoPassword adjunta la contraseña al contexto de una ejecución de bash.
func WithSudoPassword(ctx context.Context, pw string) context.Context {
	return context.WithValue(ctx, sudoKey{}, pw)
}

func sudoPassword(ctx context.Context) string {
	pw, _ := ctx.Value(sudoKey{}).(string)
	return pw
}

var sudoWord = regexp.MustCompile(`(^|[\s;&|(` + "`" + `])sudo(\s|$)`)

// UsesSudo indica si el comando invoca sudo.
func UsesSudo(cmd string) bool { return sudoWord.MatchString(cmd) }

// rewriteSudo hace que cada `sudo` lea la contraseña de stdin y no imprima prompt.
func rewriteSudo(cmd string) string {
	return sudoWord.ReplaceAllString(cmd, "${1}sudo -S -p ''${2}")
}

// sudoBadPassword detecta el rechazo de la contraseña.
func sudoBadPassword(out string) bool {
	low := strings.ToLower(out)
	return strings.Contains(low, "incorrect password") || strings.Contains(low, "sorry, try again") ||
		strings.Contains(low, "contraseña incorrecta") || strings.Contains(low, "authentication failure")
}

// sudoStdin entrega la contraseña varias veces por si el comando usa sudo más de una vez.
func sudoStdin(pw string) string { return strings.Repeat(pw+"\n", 4) }
