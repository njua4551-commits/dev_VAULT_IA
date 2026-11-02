//go:build !windows && !linux && !darwin

package ui

// Sin reproductor conocido: se usa la campana del terminal.
func playSound(name string) {
	bell()
}
