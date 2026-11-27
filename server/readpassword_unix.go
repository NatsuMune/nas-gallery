//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"syscall"
	"unsafe"
)

// readPassword prompts on stdout and reads one line from stdin with terminal
// echo disabled when stdin is a TTY. Non-TTY stdin (pipes, scripts) is read
// plainly, which keeps the CLI usable from scripts and tests.
func readPassword(prompt string) (string, error) {
	fd := int(os.Stdin.Fd())
	var old syscall.Termios
	if err := ioctlTermios(fd, ioctlReadTermios, &old); err != nil {
		// Not a terminal (or unsupported): fall back to a plain read.
		fmt.Fprint(os.Stdout, prompt)
		return readLineStdin()
	}
	raw := old
	raw.Lflag &^= syscall.ECHO // hide input, keep canonical line editing
	if err := ioctlTermios(fd, ioctlWriteTermios, &raw); err != nil {
		fmt.Fprint(os.Stdout, prompt)
		return readLineStdin()
	}
	fmt.Fprint(os.Stdout, prompt)
	line, readErr := readLineStdin()
	// Restore echo no matter what happened.
	_ = ioctlTermios(fd, ioctlWriteTermios, &old)
	fmt.Fprintln(os.Stdout)
	return line, readErr
}

func ioctlTermios(fd int, req uintptr, t *syscall.Termios) error {
	_, _, errno := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), req, uintptr(unsafe.Pointer(t)))
	if errno != 0 {
		return errno
	}
	return nil
}
