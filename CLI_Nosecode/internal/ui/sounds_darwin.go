//go:build darwin

package ui

import "os/exec"

// afplay viene con macOS y reproduce mp3 sin instalar nada.
func playSound(name string) {
	path, err := audioPath(name)
	if err != nil {
		bell()
		return
	}
	go func() {
		if err := exec.Command("afplay", path).Run(); err != nil {
			bell()
		}
	}()
}
