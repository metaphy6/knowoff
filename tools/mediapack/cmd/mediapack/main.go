package main

import (
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	cmd := os.Args[1]
	switch cmd {
	case "text-prepare", "text-build-fixture", "text-certify", "text-simulate", "text-publish", "text-duplicates", "text-actions":
		os.Exit(runTextCommand(cmd, os.Args[2:], os.Stdout, os.Stderr))
	case "build", "certify", "simulate", "publish":
		fmt.Fprintln(os.Stderr, "legacy media commands are retired; use the versioned text commands")
		os.Exit(2)
	default:
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: mediapack <text-prepare|text-build-fixture|text-certify|text-simulate|text-publish|text-duplicates|text-actions> [options]")
}
