// Package version guarda los datos de compilacion de Nosecode.
package version

const Name = "Nosecode"

// Version y Commit se sobreescriben al compilar con -ldflags.
var (
	Version = "0.6.0"
	Commit  = "dev"
)
