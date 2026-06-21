package main

import (
	"fmt"
	"os"
)

var version = "dev"

func versionLine() string {
	return "dari-docs " + version
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dari-docs: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	return execute(os.Args[1:])
}
