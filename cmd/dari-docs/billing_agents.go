package main

import "github.com/spf13/cobra"

func newBillingCommand() *cobra.Command {
	return newUnsupportedManagedCommand("billing [command]")
}

func newAgentsCommand() *cobra.Command {
	return newUnsupportedManagedCommand("agents [command]")
}
