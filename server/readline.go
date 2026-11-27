package main

import (
	"bufio"
	"io"
	"os"
	"strings"
)

var stdinReader = bufio.NewReader(os.Stdin)

// readLineStdin reads one line from stdin, tolerating EOF-terminated final
// lines without a trailing newline (piped input in scripts/tests).
func readLineStdin() (string, error) {
	line, err := stdinReader.ReadString('\n')
	line = strings.TrimRight(line, "\r\n")
	if err != nil && !(err == io.EOF && line != "") {
		return line, err
	}
	return line, nil
}
