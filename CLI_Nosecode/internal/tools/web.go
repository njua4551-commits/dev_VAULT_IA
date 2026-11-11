package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	webTimeout   = 20 * time.Second
	webMaxBytes  = 20 << 10
	webMaxSource = 2 << 20
)

var (
	reScript  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	reStyle   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reLI      = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	reBlock   = regexp.MustCompile(`(?i)</?(p|div|br|tr|h[1-6]|section|article|header|footer|main|nav|pre|blockquote|table|ul|ol)\b[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
)

func runWebFetch(ctx context.Context, raw json.RawMessage) Result {
	var a struct {
		URL      string `json:"url"`
		MaxBytes int    `json:"max_bytes"`
	}
	if err := decode(raw, &a); err != nil {
		return errResult("argumentos inválidos: %v", err)
	}
	target := strings.TrimSpace(a.URL)
	if target == "" {
		return errResult("la URL está vacía")
	}
	u, err := url.Parse(target)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return errResult("URL inválida; usa http:// o https://")
	}
	limit := a.MaxBytes
	if limit <= 0 || limit > webMaxBytes {
		limit = webMaxBytes
	}

	ctx, cancel := context.WithTimeout(ctx, webTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return errResult("no pude preparar la petición: %v", err)
	}
	req.Header.Set("User-Agent", "nosecode/0.4 (+https://github.com/njua4551-commits/dev_VAULT_IA)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.5")

	resp, err := (&http.Client{Timeout: webTimeout}).Do(req)
	if err != nil {
		return errResult("no pude descargar %s: %v", target, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return errResult("%s respondió HTTP %d", target, resp.StatusCode)
	}

	source, err := io.ReadAll(io.LimitReader(resp.Body, webMaxSource))
	if err != nil {
		return errResult("error leyendo la respuesta: %v", err)
	}
	contentType := strings.ToLower(resp.Header.Get("Content-Type"))
	text := string(source)
	if strings.Contains(contentType, "html") || strings.HasPrefix(strings.TrimSpace(text), "<") {
		text = htmlToText(text)
	} else if !strings.Contains(contentType, "text/") && !strings.Contains(contentType, "json") &&
		!strings.Contains(contentType, "xml") {
		return errResult("el contenido no es texto (%s)", contentType)
	}

	text = truncateText(strings.TrimSpace(text), limit)
	header := fmt.Sprintf("%s · HTTP %d", target, resp.StatusCode)
	if contentType != "" {
		header += " · " + contentType
	}
	if text == "" {
		text = "(sin texto)"
	}
	return Result{Text: header + "\n\n" + text}
}

func htmlToText(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reStyle.ReplaceAllString(s, " ")
	s = reComment.ReplaceAllString(s, " ")
	s = reLI.ReplaceAllString(s, "\n- ")
	s = reBlock.ReplaceAllString(s, "\n")
	s = reTag.ReplaceAllString(s, " ")
	s = html.UnescapeString(s)

	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	blank := false
	for _, line := range lines {
		line = strings.Join(strings.Fields(line), " ")
		if line == "" {
			if !blank && len(out) > 0 {
				out = append(out, "")
			}
			blank = true
			continue
		}
		out = append(out, line)
		blank = false
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}
