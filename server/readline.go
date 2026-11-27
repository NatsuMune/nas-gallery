package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

// readLineStdin reads one line from stdin, tolerating EOF-terminated final
// lines without a trailing newline (piped input in scripts/tests).
func readLineStdin() (string, error) {
	r := bufio.NewReader(os.Stdin)
	line, err := r.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil && !(err == io.EOF && line != "") {
		return line, err
	}
	return line, nil
}
