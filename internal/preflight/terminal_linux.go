//go:build linux

package preflight

import (
	"fmt"
	"syscall"
	"unsafe"
)

type winsize struct {
	rows    uint16
	cols    uint16
	xpixels uint16
	ypixels uint16
}

func isTerminal(fd uintptr) bool {
	var termios syscall.Termios
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TCGETS, uintptr(unsafe.Pointer(&termios)))
	return errno == 0
}

func terminalSize(fd uintptr) (Size, error) {
	var size winsize
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, fd, syscall.TIOCGWINSZ, uintptr(unsafe.Pointer(&size)))
	if errno != 0 {
		return Size{}, errno
	}
	if size.cols == 0 || size.rows == 0 {
		return Size{}, fmt.Errorf("terminal reported %dx%d", size.cols, size.rows)
	}
	return Size{Columns: int(size.cols), Rows: int(size.rows)}, nil
}
