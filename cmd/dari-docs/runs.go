package main

import "github.com/spf13/cobra"

func newRunsCommand() *cobra.Command {
	return newUnsupportedManagedCommand("runs [command]")
}
