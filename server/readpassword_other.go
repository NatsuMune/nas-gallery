//go:build !darwin && !linux

package main

import (
	"fmt"
	"strings"
)

// readPassword reads a line from stdin without echo suppression on platforms
// where we do not implement termios (e.g. Windows). The prompt warns the user.
func readPassword(prompt string) (string, error) {
	fmt.Print(prompt + " (input will be visible on this platform) ")
	line, err := readLineStdin()
	return strings.TrimSpace(line), err
}
