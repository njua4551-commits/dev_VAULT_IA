//go:build windows

package ui

import (
	"fmt"
	"sync"
	"syscall"
	"unsafe"
)

var (
	winmm         = syscall.NewLazyDLL("winmm.dll")
	mciSendString = winmm.NewProc("mciSendStringW")
	soundSeq      uint64
	soundSeqMu    sync.Mutex
)

func mci(cmd string) error {
	p, err := syscall.UTF16PtrFromString(cmd)
	if err != nil {
		return err
	}
	r, _, _ := mciSendString.Call(uintptr(unsafe.Pointer(p)), 0, 0, 0)
	if r != 0 {
		return fmt.Errorf("mci %d", r)
	}
	return nil
}

// playSound usa MCI (winmm) para reproducir mp3 sin dependencias externas.
func playSound(name string) {
	path, err := audioPath(name)
	if err != nil {
		bell()
		return
	}

	soundSeqMu.Lock()
	soundSeq++
	alias := fmt.Sprintf("nosecode%d", soundSeq)
	soundSeqMu.Unlock()

	go func() {
		open := fmt.Sprintf(`open "%s" type mpegvideo alias %s`, path, alias)
		if err := mci(open); err != nil {
			bell()
			return
		}
		defer mci("close " + alias)
		if err := mci("play " + alias + " wait"); err != nil {
			bell()
		}
	}()
}
