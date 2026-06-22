package main

import "github.com/spf13/cobra"

func newAuthCommand() *cobra.Command {
	return newUnsupportedManagedCommand("auth [command]")
}
