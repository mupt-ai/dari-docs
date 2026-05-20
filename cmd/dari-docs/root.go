package main

import "github.com/spf13/cobra"

func execute(args []string) error {
	cmd := newRootCommand()
	cmd.SetArgs(normalizeRootArgs(args))
	return cmd.Execute()
}

func newRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:           "dari-docs",
		Short:         "Run hosted docs user tests and propose docs improvements",
		SilenceUsage:  true,
		SilenceErrors: true,
		Version:       version,
	}
	root.SetVersionTemplate(versionLine() + "\n")

	root.AddCommand(
		newCheckCommand(),
		newOptimizeCommand(),
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
