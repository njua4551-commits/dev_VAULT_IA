//go:build linux

package ui

import "os/exec"

// Reproductores en orden de preferencia: todos existen en distros comunes.
var linuxPlayers = [][]string{
	{"ffplay", "-nodisp", "-autoexit", "-loglevel", "quiet"},
	{"mpg123", "-q"},
	{"cvlc", "--play-and-exit", "--intf", "dummy"},
}

func playSound(name string) {
	path, err := audioPath(name)
	if err != nil {
		bell()
		return
	}

	go func() {
		for _, player := range linuxPlayers {
			bin, err := exec.LookPath(player[0])
			if err != nil {
				continue
			}
			args := append(append([]string{}, player[1:]...), path)
			if err := exec.Command(bin, args...).Run(); err == nil {
				return
			}
		}
		bell()
	}()
}
