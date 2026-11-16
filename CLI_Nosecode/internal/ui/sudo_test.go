package ui

import (
	"runtime"
	"testing"
	"time"
)

func TestSudoSiemprePideAprobacionYContrasena(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip()
	}
	m := newTestModel(t)
	m.cfg.Permissions.Bash = "allow"
	c := callOf("bash", `{"command":"sudo apt update"}`)
	if !m.callNeedsApproval(c) {
		t.Fatal("sudo debe pedir aprobación aunque bash esté en allow")
	}
	if !m.needsSudoPassword(c) {
		t.Fatal("debe pedir la contraseña")
	}
	m.sudoPW, m.sudoUntil = "x", time.Now().Add(time.Minute)
	if m.needsSudoPassword(c) {
		t.Fatal("con contraseña vigente no se vuelve a pedir")
	}
	m.sudoUntil = time.Now().Add(-time.Second)
	if !m.needsSudoPassword(c) {
		t.Fatal("caducada: se pide otra vez")
	}
	m = m.forgetSudo()
	if m.sudoPW != "" {
		t.Fatal("forgetSudo")
	}
	if m.needsSudoPassword(callOf("bash", `{"command":"ls"}`)) {
		t.Fatal("ls no necesita contraseña")
	}
}
