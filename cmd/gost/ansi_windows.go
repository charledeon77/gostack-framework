//go:build windows
// +build windows

package main

import (
	"syscall"
)

/*
Purpose:
This file automatically enables Virtual Terminal Processing (ANSI escape sequence support)
on Windows systems when the global `gost` binary is run.

Why it is needed:
By default, the Windows Console Host (used by both cmd.exe and powershell.exe) does not
process ANSI escape sequences (colors and formatting), which causes raw codes like
"←[1;36m" to be displayed. This file calls the Windows kernel to enable ANSI support.

How it works:
Using Go build tags, this file is only compiled on Windows. At startup, the `init()`
function retrieves the standard output handle and updates the console mode to include
`ENABLE_VIRTUAL_TERMINAL_PROCESSING` (0x0004).
*/
func init() {
	handle, err := syscall.GetStdHandle(syscall.STD_OUTPUT_HANDLE)
	if err != nil {
		return
	}

	var mode uint32
	err = syscall.GetConsoleMode(handle, &mode)
	if err != nil {
		return
	}

	// 0x0004 is the Windows constant for ENABLE_VIRTUAL_TERMINAL_PROCESSING
	const enableVirtualTerminalProcessing = 0x0004
	mode |= enableVirtualTerminalProcessing

	// Call SetConsoleMode natively using the lazy-loaded kernel32.dll
	kernel32 := syscall.NewLazyDLL("kernel32.dll")
	setConsoleMode := kernel32.NewProc("SetConsoleMode")
	_, _, _ = setConsoleMode.Call(uintptr(handle), uintptr(mode))
}
