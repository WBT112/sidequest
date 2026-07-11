//go:build !linux

package preflight

import "fmt"

func isTerminal(uintptr) bool {
	return false
}

func terminalSize(uintptr) (Size, error) {
	return Size{}, fmt.Errorf("terminal checks are only implemented on Linux")
}
