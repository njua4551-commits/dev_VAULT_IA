package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	webTimeout      = 20 * time.Second
	webMaxBytes     = 20 << 10
	webMaxSource    = 2 << 20
	webMaxRedirects = 5
	webContentOpen  = "<<<CONTENIDO_WEB_NO_CONFIABLE>>>"
	webContentClose = "<<<FIN_CONTENIDO_WEB>>>"
)

type webLocalKey struct{}

// WithWebLocal permite que web_fetch acceda a localhost y a la red privada.
// Solo debe usarse cuando permissions.webfetch_local = "allow".
func WithWebLocal(ctx context.Context) context.Context {
	return context.WithValue(ctx, webLocalKey{}, true)
}

func webLocalFrom(ctx context.Context) bool {
	ok, _ := ctx.Value(webLocalKey{}).(bool)
	return ok
}

// webHTTPClient aplica la política SSRF en el momento de conectar, no solo al
// validar la URL: así una redirección o un DNS rebinding tampoco esquivan el
// bloqueo.
func webHTTPClient(ctx context.Context) *http.Client {
	allowLocal := webLocalFrom(ctx)
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	transport := &http.Transport{
		Proxy:                 nil, // un proxy HTTP podría resolver el destino fuera de este filtro
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          10,
		IdleConnTimeout:       30 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: webTimeout,
		DialContext: func(dctx context.Context, network, addr string) (net.Conn, error) {
			return webDialContext(dctx, dialer, network, addr, allowLocal)
		},
	}
	return &http.Client{
		Transport: transport,
		Timeout:   webTimeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= webMaxRedirects {
				return fmt.Errorf("demasiadas redirecciones (máximo %d)", webMaxRedirects)
			}
			return validateWebTarget(req.URL, allowLocal)
		},
	}
}

func webDialContext(ctx context.Context, dialer *net.Dialer, network, addr string, allowLocal bool) (net.Conn, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return nil, err
	}
	if allowLocal {
		return dialer.DialContext(ctx, network, addr)
	}
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if err := validateWebHostName(host); err != nil {
		return nil, err
	}
	if ip := net.ParseIP(host); ip != nil {
		if err := checkWebIP(ip); err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, addr)
	}
	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("no pude resolver %s: %w", host, err)
	}
	if len(ips) == 0 {
		return nil, fmt.Errorf("no pude resolver %s", host)
	}
	for _, addr := range ips {
		if err := checkWebIP(addr.IP); err != nil {
			return nil, err
		}
	}
	var lastErr error
	for _, addr := range ips {
		conn, err := dialer.DialContext(ctx, network, net.JoinHostPort(addr.IP.String(), port))
		if err == nil {
			return conn, nil
		}
		lastErr = err
	}
	return nil, lastErr
}

func validateWebTarget(u *url.URL, allowLocal bool) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return fmt.Errorf("URL inválida; usa http:// o https://")
	}
	if allowLocal {
		return nil
	}
	host := strings.TrimSuffix(strings.ToLower(u.Hostname()), ".")
	if err := validateWebHostName(host); err != nil {
		return err
	}
	if ip := net.ParseIP(host); ip != nil {
		return checkWebIP(ip)
	}
	return nil
}

func validateWebHostName(host string) error {
	if host == "" {
		return fmt.Errorf("la URL no tiene host")
	}
	if host == "localhost" || strings.HasSuffix(host, ".localhost") ||
		host == "ip6-localhost" || host == "ip6-loopback" {
		return fmt.Errorf("destino bloqueado: %s es un nombre local", host)
	}
	switch host {
	case "metadata.google.internal", "metadata.goog", "instance-data":
		return fmt.Errorf("destino bloqueado: %s es un servicio de metadatos", host)
	}
	return nil
}

func checkWebIP(ip net.IP) error {
	if ip == nil {
		return fmt.Errorf("destino bloqueado: dirección IP inválida")
	}
	blocked := ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() ||
		ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast()
	if v4 := ip.To4(); v4 != nil {
		if v4[0] == 0 || (v4[0] == 100 && v4[1]&0xc0 == 0x40) || ip.Equal(net.IPv4bcast) {
			blocked = true
		}
	}
	if blocked {
		return fmt.Errorf("destino bloqueado: %s es una dirección local o privada", ip)
	}
	return nil
}

var (
	reScript  = regexp.MustCompile(`(?is)<script\b[^>]*>.*?</script>`)
	reStyle   = regexp.MustCompile(`(?is)<style\b[^>]*>.*?</style>`)
	reComment = regexp.MustCompile(`(?s)<!--.*?-->`)
	reLI      = regexp.MustCompile(`(?i)<li\b[^>]*>`)
	reBlock   = regexp.MustCompile(`(?i)</?(p|div|br|tr|h[1-6]|section|article|header|footer|main|nav|pre|blockquote|table|ul|ol)\b[^>]*>`)
	reTag     = regexp.MustCompile(`(?s)<[^>]+>`)
	// reNoise elimina bloques de navegacion que no aportan contenido util
	// (menus de GitHub, cabeceras, formularios, iconos SVG...).
	reNoise = []*regexp.Regexp{
		regexp.MustCompile(`(?is)<nav\b[^>]*>.*?</nav>`),
		regexp.MustCompile(`(?is)<header\b[^>]*>.*?</header>`),
		regexp.MustCompile(`(?is)<footer\b[^>]*>.*?</footer>`),
		regexp.MustCompile(`(?is)<aside\b[^>]*>.*?</aside>`),
		regexp.MustCompile(`(?is)<form\b[^>]*>.*?</form>`),
		regexp.MustCompile(`(?is)<svg\b[^>]*>.*?</svg>`),
		regexp.MustCompile(`(?is)<noscript\b[^>]*>.*?</noscript>`),
		regexp.MustCompile(`(?is)<template\b[^>]*>.*?</template>`),
	}
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
	if err := validateWebTarget(u, webLocalFrom(ctx)); err != nil {
		return errResult("%v", err)
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

	resp, err := webHTTPClient(ctx).Do(req)
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
	return Result{Text: header + "\n\n" + webContentOpen + "\n" + text + "\n" + webContentClose}
}

func htmlToText(s string) string {
	s = reScript.ReplaceAllString(s, " ")
	s = reStyle.ReplaceAllString(s, " ")
	s = reComment.ReplaceAllString(s, " ")
	for _, re := range reNoise {
		s = re.ReplaceAllString(s, " ")
	}
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
