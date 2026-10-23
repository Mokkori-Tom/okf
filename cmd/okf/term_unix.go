//go:build linux || darwin || freebsd

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// ttyWidth returns the column count of the terminal on f, or 0 if f is not a
// terminal.
func ttyWidth(f *os.File) int {
	var ws struct{ Row, Col, X, Y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws))) //nolint:gosec // TIOCGWINSZ fills ws
	if errno != 0 {
		return 0
	}
	return int(ws.Col)
}
