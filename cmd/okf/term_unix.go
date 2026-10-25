//go:build linux || darwin || freebsd

package main

import (
	"os"
	"syscall"
	"unsafe"
)

// isTTY asks the terminal driver: the window-size ioctl only succeeds on a
// terminal (it fails with ENOTTY on /dev/null).
func isTTY(f *os.File) bool {
	var ws struct{ Row, Col, X, Y uint16 }
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), uintptr(syscall.TIOCGWINSZ), uintptr(unsafe.Pointer(&ws))) //nolint:gosec // TIOCGWINSZ fills ws
	return errno == 0
}

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
