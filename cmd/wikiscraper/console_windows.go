//go:build windows

package main

import (
	"syscall"
	"unsafe"
)

// ownsConsoleWindow reports whether this program is the only process attached
// to its console window. That is true when the .exe is double-clicked in
// Explorer (Windows creates a new console just for it) and false when it is
// started from PowerShell or Command Prompt, whose shell shares the console.
func ownsConsoleWindow() bool {
	getConsoleProcessList := syscall.NewLazyDLL("kernel32.dll").NewProc("GetConsoleProcessList")
	if getConsoleProcessList.Find() != nil {
		return false
	}
	processIDs := make([]uint32, 4)
	count, _, _ := getConsoleProcessList.Call(
		uintptr(unsafe.Pointer(&processIDs[0])),
		uintptr(len(processIDs)),
	)
	return count == 1
}
