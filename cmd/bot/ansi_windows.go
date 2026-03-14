//go:build windows

package main

import (
	"os"
	"syscall"
	"unsafe"
)

func init() {
	// Enable Virtual Terminal Processing on Windows so ANSI escape codes work in cmd.exe
	var mode uint32
	handle := syscall.Handle(os.Stdout.Fd())
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	getMode := kernel32.NewProc("GetConsoleMode")
	setMode := kernel32.NewProc("SetConsoleMode")

	r, _, _ := getMode.Call(uintptr(handle), uintptr(unsafe.Pointer(&mode)))
	if r != 0 {
		const enableVirtualTerminalProcessing = 0x0004
		setMode.Call(uintptr(handle), uintptr(mode|enableVirtualTerminalProcessing)) //nolint:errcheck
	}
}
