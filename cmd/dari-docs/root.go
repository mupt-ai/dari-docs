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
		passthroughCommand("init [repo]", "Extract or deploy bundled agents", runInit),
		newAuthCommand(),
		newBillingCommand(),
		newAgentsCommand(),
		newRunsCommand(),
		&cobra.Command{
			Use:   "version",
			Short: "Print version",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				cmd.Println(versionLine())
				return nil
			},
		},
	)
	return root
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

func passthroughCommand(use, short string, run func([]string) error) *cobra.Command {
	return &cobra.Command{
		Use:                use,
		Short:              short,
		DisableFlagParsing: true,
		Args:               cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if wantsCommandHelp(args) {
				return cmd.Help()
			}
			return run(args)
		},
	}
}

func wantsCommandHelp(args []string) bool {
	for _, arg := range args {
		if arg == "-h" || arg == "--help" {
			return true
		}
	}
	return false
}

func newAuthCommand() *cobra.Command {
	cmd := passthroughCommand("auth", "Authenticate to the managed service", runAuth)
	login := passthroughCommand("login", "Log in with a browser", runAuthLogin)
	logout := passthroughCommand("logout", "Log out or revoke credentials", runAuthLogout)
	status := passthroughCommand("status", "Show authenticated account", runAuthStatus)
	apiKey := passthroughCommand("api-key", "Manage API keys", runAuthToken)
	apiKey.Aliases = []string{"api-keys", "token"}
	apiKey.AddCommand(
		passthroughCommand("create", "Create an API key", runAuthTokenCreate),
		passthroughCommand("list", "List API keys", runAuthTokenList),
		passthroughCommand("revoke <api-key-id>", "Revoke an API key", runAuthTokenRevoke),
	)
	cmd.AddCommand(login, logout, status, apiKey)
	return cmd
}

func newBillingCommand() *cobra.Command {
	cmd := passthroughCommand("billing", "Manage managed-service billing", runBilling)
	cmd.AddCommand(
		passthroughCommand("balance", "Show credit balance", func(args []string) error { return runBilling(append([]string{"balance"}, args...)) }),
		passthroughCommand("checkout", "Buy credits", func(args []string) error { return runBilling(append([]string{"checkout"}, args...)) }),
	)
	return cmd
}

func newAgentsCommand() *cobra.Command {
	cmd := passthroughCommand("agents", "Agent helper commands", runAgents)
	cmd.AddCommand(passthroughCommand("deploy", "Deploy or select docs agents", func(args []string) error {
		return runAgents(append([]string{"deploy"}, args...))
	}))
	return cmd
}

func newRunsCommand() *cobra.Command {
	cmd := passthroughCommand("runs", "Inspect managed runs", runRuns)
	cmd.AddCommand(
		passthroughCommand("status <run-id>", "Show run status", runRunsStatus),
		passthroughCommand("wait <run-id>", "Wait for a run to finish", runRunsWait),
		passthroughCommand("download <run-id> [repo]", "Download run artifacts", runRunsDownload),
		passthroughCommand("apply <run-id> [repo]", "Apply run artifacts", runRunsApply),
	)
	return cmd
}
