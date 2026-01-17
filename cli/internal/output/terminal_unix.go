//go:build !windows

package output

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// winsize struct for ioctl TIOCGWINSZ
type winsize struct {
	Row    uint16
	Col    uint16
	Xpixel uint16
	Ypixel uint16
}

// getTerminalWidth returns the current terminal width
func getTerminalWidth() int {
	// Check COLUMNS env var first
	if cols := os.Getenv("COLUMNS"); cols != "" {
		var width int
		if _, err := fmt.Sscanf(cols, "%d", &width); err == nil && width > 0 {
			return width
		}
	}

	// Try to get from terminal using ioctl
	ws := &winsize{}
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL,
		uintptr(syscall.Stdout),
		uintptr(syscall.TIOCGWINSZ),
		uintptr(unsafe.Pointer(ws)))
	if errno == 0 && ws.Col > 0 {
		return int(ws.Col)
	}

	return defaultWidth
}
