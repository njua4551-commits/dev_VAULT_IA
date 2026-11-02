// Comando nosecode: cliente de terminal para modelos OpenAI-compatibles.
package main

import (
	"fmt"
	"os"
	"strings"

	"nosecode/internal/config"
	"nosecode/internal/console"
	"nosecode/internal/ui"
	"nosecode/internal/version"

	tea "github.com/charmbracelet/bubbletea"
)

const help = `Nosecode: cliente de terminal para modelos OpenAI-compatibles

Uso:
  nosecode                 abre la interfaz
  nosecode --demo[=pantalla] imprime una pantalla sin TTY
                           pantallas: home, session, settings,
                           settings-edit, palette
  nosecode --config        muestra la ruta del config
  nosecode --version       muestra la version
  nosecode --help          esta ayuda

Sesiones:
  se guardan en ~/.nosecode/sessions y aparecen en la barra lateral y en /sessions

Variables:
  NOSECODE_HOME   carpeta de datos (por defecto ~/.nosecode)
  VAULT_HOME      carpeta del Dev Vault (por defecto ~/.dev-vault)
`

func main() {
	console.Init()
	cfg, err := config.Load()
	if err != nil {
		fmt.Fprintf(os.Stderr, "nosecode: %v\n", err)
		os.Exit(1)
	}

	for _, arg := range os.Args[1:] {
		switch {
		case arg == "--version" || arg == "-v" || arg == "version":
			fmt.Printf("%s %s (%s)\n", version.Name, version.Version, version.Commit)
			return
		case arg == "--config" || arg == "config":
			fmt.Println(cfg.Path)
			return
		case arg == "--help" || arg == "-h" || arg == "help":
			fmt.Print(help)
			return
		case arg == "--demo" || strings.HasPrefix(arg, "--demo="):
			kind := "home"
			if strings.HasPrefix(arg, "--demo=") {
				kind = strings.TrimPrefix(arg, "--demo=")
			}
			fmt.Print(ui.Demo(cfg, kind, 100, 32))
			return
		default:
			fmt.Fprintf(os.Stderr, "nosecode: opcion desconocida %q\n\n%s", arg, help)
			os.Exit(2)
		}
	}

	p := tea.NewProgram(ui.New(cfg), tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := p.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "nosecode: %v\n", err)
		os.Exit(1)
	}
}
