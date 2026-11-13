package ui

import "regexp"

// redactSecrets oculta credenciales antes de escribir un comando en debug.log.
// La confirmación de la TUI sigue mostrando el comando completo.
var redactRules = []struct {
	re   *regexp.Regexp
	repl string
}{
	// https://usuario:contraseña@host
	{regexp.MustCompile(`(https?://[^:/@\s]+:)[^@\s]+(@)`), `${1}***${2}`},
	// TOKEN=..., API_KEY=..., PASSWORD=...
	{regexp.MustCompile(`(?i)\b([A-Z0-9_]*(?:TOKEN|SECRET|PASSWORD|PASSWD|API_KEY|APIKEY|ACCESS_KEY|PRIVATE_KEY|CREDENTIAL)[A-Z0-9_]*)\s*=\s*("[^"]*"|'[^']*'|[^\s;|&]+)`), `${1}=***`},
	// --token ..., --password=..., --api-key ...
	{regexp.MustCompile(`(?i)(--?(?:token|password|passwd|secret|api[-_]?key|access[-_]?key|auth|bearer)(?:\s+|=))("[^"]*"|'[^']*'|[^\s]+)`), `${1}***`},
	// Authorization: Bearer ...
	{regexp.MustCompile(`(?i)\b(bearer)\s+[A-Za-z0-9._~+/-]+=*`), `${1} ***`},
	// Tokens con prefijo conocido.
	{regexp.MustCompile(`\b(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|sk-[A-Za-z0-9_-]{16,}|xox[baprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16})\b`), `***`},
}

func redactSecrets(s string) string {
	for _, rule := range redactRules {
		s = rule.re.ReplaceAllString(s, rule.repl)
	}
	return s
}
