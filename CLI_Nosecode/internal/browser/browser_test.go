package browser

import (
	"testing"

	"nosecode/internal/config"
)

func TestCDPPort(t *testing.T) {
	servers := map[string]config.MCPServer{
		"playwright": {Command: []string{"node", "cli.js", "--cdp-endpoint", "http://127.0.0.1:9222"}},
	}
	port, ok := CDPPort(servers)
	if !ok || port != 9222 {
		t.Fatalf("puerto = %d, ok=%v", port, ok)
	}
}

func TestCDPPortIgnoresDisabled(t *testing.T) {
	off := false
	servers := map[string]config.MCPServer{
		"playwright": {Enabled: &off, Command: []string{"node", "cli.js", "--cdp-endpoint=http://127.0.0.1:9333"}},
	}
	if _, ok := CDPPort(servers); ok {
		t.Fatal("no deberia usar un servidor deshabilitado")
	}
}

func TestExecutableFromCommand(t *testing.T) {
	got, err := executableFromCommand(`"C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe" --single-argument %1`)
	if err != nil {
		t.Fatal(err)
	}
	if got != `C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe` {
		t.Fatalf("exe = %q", got)
	}
}

func TestIsChromium(t *testing.T) {
	cases := map[string]bool{
		`C:\Program Files\BraveSoftware\Brave-Browser\Application\brave.exe`: true,
		`C:\Program Files\Google\Chrome\Application\chrome.exe`:              true,
		`C:\Program Files\Mozilla Firefox\firefox.exe`:                       false,
		`C:\Program Files\Microsoft\Edge\Application\msedge.exe`:             true,
		"/usr/bin/brave":          true,
		"/usr/bin/google-chrome":  true,
		"/usr/bin/chromium":       true,
		"/usr/bin/microsoft-edge": true,
		"/usr/bin/firefox":        false,
	}
	for exe, want := range cases {
		if got := IsChromium(exe); got != want {
			t.Fatalf("%s = %v, queria %v", exe, got, want)
		}
	}
}
