package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func execute(args []string) error {
	cmd := newRootCommand()
	cmd.SetArgs(normalizeRootArgs(args))
	return cmd.Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "dari-docs",
		Short:         "Run Flue apps to test and improve docs",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.SetVersionTemplate(versionLine() + "\n")

	root.AddCommand(
		newCheckOptimizeCommand("check"),
		newCheckOptimizeCommand("optimize"),
		newInitCommand(),
		newAuthCommand(),
		newBillingCommand(),
		newAgentsCommand(),
		newRunsCommand(),
		newVersionCommand(),
	)
	return root
}

func newVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "version",
		Short:         "Print version",
		Args:          cobra.NoArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			cmd.Println(versionLine())
			return nil
		},
	}
}

func unsupportedManagedModeError() error {
	return fmt.Errorf("the hosted/managed Dari Docs path is not supported in this CLI; run the Flue apps yourself and call their URLs instead:\n  dari-docs init\n  cd .dari-docs/agents/docs-user-tester-agent && bun install --frozen-lockfile && bun run build && bun run start\n  dari-docs check . --tester-url https://your-tester.example --task \"Install the SDK\"")
}

func newUnsupportedManagedCommand(use string) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Hidden:             true,
		DisableFlagParsing: true,
		SilenceUsage:       true,
		SilenceErrors:      true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return unsupportedManagedModeError()
		},
	}
}

func normalizeRootArgs(args []string) []string {
	if len(args) == 0 {
		return []string{"optimize"}
	}
	switch args[0] {
	case "-h", "--help", "help", "completion", "init", "optimize", "check", "auth", "billing", "agents", "runs", "version":
		return args
	case "-v", "--version":
		return []string{"version"}
	default:
		out := make([]string, 0, len(args)+1)
		out = append(out, "optimize")
		out = append(out, args...)
		return out
	}
}
