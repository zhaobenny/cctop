//go:build windows

package output

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procGetConsoleScreenBufferInfo = kernel32.NewProc("GetConsoleScreenBufferInfo")
)

type coord struct {
	x int16
	y int16
}

type smallRect struct {
	left   int16
	top    int16
	right  int16
	bottom int16
}

type consoleScreenBufferInfo struct {
	size              coord
	cursorPosition    coord
	attributes        uint16
	window            smallRect
	maximumWindowSize coord
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

	// Try to get console width via Windows API
	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return defaultWidth
	}

	var info consoleScreenBufferInfo
	ret, _, _ := procGetConsoleScreenBufferInfo.Call(
		uintptr(handle),
		uintptr(unsafe.Pointer(&info)))
	if ret != 0 {
		width := int(info.window.right - info.window.left + 1)
		if width > 0 {
			return width
		}
	}

	return defaultWidth
}
