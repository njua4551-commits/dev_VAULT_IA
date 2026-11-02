//go:build windows

// Package console ajusta la consola de Windows para la interfaz.
package console

import "syscall"

var (
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	setConsoleCP     = kernel32.NewProc("SetConsoleCP")
	setConsoleOutput = kernel32.NewProc("SetConsoleOutputCP")
)

// Init activa UTF-8 para que acentos y bordes se dibujen bien.
func Init() {
	_, _, _ = setConsoleCP.Call(65001)
	_, _, _ = setConsoleOutput.Call(65001)
}
