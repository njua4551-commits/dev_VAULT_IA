package tools

import (
	"fmt"
	"strings"
)

// MiniDiff devuelve un diff corto para mostrar en la TUI (no se envía al
// modelo): recorta el prefijo y el sufijo iguales, deja 2 líneas de contexto y
// marca el bloque cambiado con "-" y "+". Primera línea: "+N −M".
func MiniDiff(oldText, newText string) string {
	if oldText == newText {
		return ""
	}
	a := splitKeep(oldText)
	b := splitKeep(newText)
	pre := 0
	for pre < len(a) && pre < len(b) && a[pre] == b[pre] {
		pre++
	}
	suf := 0
	for suf < len(a)-pre && suf < len(b)-pre && a[len(a)-1-suf] == b[len(b)-1-suf] {
		suf++
	}
	delA, addB := a[pre:len(a)-suf], b[pre:len(b)-suf]
	const ctx = 2
	start := pre - ctx
	if start < 0 {
		start = 0
	}
	var sb strings.Builder
	fmt.Fprintf(&sb, "+%d −%d\n@@ línea %d @@\n", len(addB), len(delA), pre+1)
	for _, l := range a[start:pre] {
		sb.WriteString("  " + l + "\n")
	}
	emit := func(prefix string, ls []string) {
		const maxLines = 400
		for i, l := range ls {
			if i == maxLines {
				fmt.Fprintf(&sb, "%s… (%d líneas más)\n", prefix, len(ls)-maxLines)
				return
			}
			sb.WriteString(prefix + " " + l + "\n")
		}
	}
	emit("-", delA)
	emit("+", addB)
	end := len(a) - suf + ctx
	if end > len(a) {
		end = len(a)
	}
	for _, l := range a[len(a)-suf : end] {
		sb.WriteString("  " + l + "\n")
	}
	return strings.TrimRight(sb.String(), "\n")
}

func splitKeep(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}
