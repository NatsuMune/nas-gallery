package main

import (
	"fmt"
	"os"
)

// Version is the server version reported by /api/health and /api/me.
const Version = "1.0.0"

const usageText = `nas-gallery — self-hosted photo/video gallery server

Usage:
  nas-gallery init [-c config.yaml]        write example config (if absent), create DB + secret
  nas-gallery user add <name> [-c path]    create a user (prompts for password, hidden)
  nas-gallery user list [-c path]          list users
  nas-gallery user rm <name> [-c path]     remove a user
  nas-gallery serve [-c config.yaml]       run the HTTP server
  nas-gallery sampledata [-out dir]        generate a synthetic media tree for manual testing
  nas-gallery version                      print version

Run "nas-gallery help" for details.
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "nas-gallery: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		fmt.Fprint(os.Stderr, usageText)
		return fmt.Errorf("no command given")
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "init":
		return cmdInit(rest)
	case "user":
		return cmdUser(rest)
	case "serve":
		return cmdServe(rest)
	case "sampledata":
		return cmdSampleData(rest)
	case "version", "-v", "--version":
		fmt.Println(Version)
		return nil
	case "help", "-h", "--help":
		fmt.Print(usageText)
		return nil
	default:
		return fmt.Errorf("unknown command %q — try \"nas-gallery help\"", cmd)
	}
}
