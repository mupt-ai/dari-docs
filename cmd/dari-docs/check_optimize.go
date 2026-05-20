package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	appconfig "github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/runner"
	"github.com/mupt-ai/dari-docs/internal/workspace"
	"github.com/spf13/cobra"
)

type checkOptimizeOptions struct {
	Command string
	RepoArg string

	RepoRoot string
	OutDir   string

	TaskInputs []string
	TaskFiles  []string
	Tasks      []string

	SecretEnvs     []string
	RuntimeSecrets map[string]string

	BundleIncludes []string
	BundleExcludes []string
	BundleOptions  bundle.CreateOptions

	APIKeyEnv     string
	APIKey        string
	APIBaseURL    string
	FeedbackAgent string
	EditorAgent   string

	LLMID           string
	FeedbackLLMRaw  []string
	FeedbackLLMIDs  []string
	EditorLLMID     string
	EditorLLMIDFlag string

	Parallel       int
	Apply          bool
	LiveVerify     bool
	Managed        bool
	TimeoutMinutes int
}

func newCheckOptimizeCommand(command string) *cobra.Command {
	opts := defaultCheckOptimizeOptions(command)
	cmd := &cobra.Command{
		Use:           command + " [repo]",
		Short:         checkOptimizeShort(command),
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				opts.RepoArg = args[0]
			}
			return runCheckOrOptimizeOptions(cmd.Context(), opts)
		},
	}
	bindCheckOptimizeFlags(cmd, opts)
	return cmd
}

func defaultCheckOptimizeOptions(command string) *checkOptimizeOptions {
	return &checkOptimizeOptions{
		Command:        command,
		APIKeyEnv:      "DARI_API_KEY",
		APIBaseURL:     os.Getenv("DARI_API_BASE_URL"),
		Parallel:       4,
		TimeoutMinutes: 30,
	}
}

func bindCheckOptimizeFlags(cmd *cobra.Command, opts *checkOptimizeOptions) {
	flags := cmd.Flags()
	flags.StringArrayVar(&opts.TaskInputs, "task", nil, "implementation task/prompt to test; repeatable")
	flags.StringArrayVar(&opts.TaskFiles, "tasks-file", nil, "file containing tasks, one per paragraph or bullet; repeatable")
	flags.StringArrayVar(&opts.SecretEnvs, "secret-env", nil, "runtime product/API secret env var to pass to sessions; repeatable")
	flags.StringArrayVar(&opts.BundleIncludes, "bundle-include", nil, "repo-relative glob to include in the docs bundle in addition to defaults; repeatable")
	flags.StringArrayVar(&opts.BundleExcludes, "bundle-exclude", nil, "repo-relative glob to exclude from the docs bundle; repeatable")
	flags.StringVar(&opts.APIKeyEnv, "api-key-env", opts.APIKeyEnv, "env var containing Dari API key")
	flags.StringVar(&opts.APIKey, "api-key", "", "Dari API key (prefer --api-key-env)")
	flags.StringVar(&opts.APIBaseURL, "api-base-url", opts.APIBaseURL, "Dari API base URL (defaults to production)")
	flags.StringVar(&opts.FeedbackAgent, "feedback-agent", "", "Dari docs user-test agent ID (defaults to .dari-docs/config.json)")
	flags.StringVar(&opts.EditorAgent, "editor-agent", "", "Dari docs editor agent ID (defaults to .dari-docs/config.json)")
	flags.StringVar(&opts.LLMID, "llm", "", "manifest LLM option ID to use for all sessions")
	flags.StringSliceVar(&opts.FeedbackLLMRaw, "feedback-llm", nil, "manifest LLM option ID or group for feedback/tester sessions; repeat or comma-separate (groups: all, claude, gpt; overrides --llm)")
	flags.StringVar(&opts.EditorLLMIDFlag, "editor-llm", "", "manifest LLM option ID for the editor session (overrides --llm)")
	flags.StringVar(&opts.OutDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	flags.IntVar(&opts.Parallel, "parallel", opts.Parallel, "number of feedback sessions per self-managed batch")
	flags.BoolVar(&opts.Apply, "apply", false, "copy updated docs back into the repo after downloading")
	flags.BoolVar(&opts.LiveVerify, "live-verify", false, "allow agents to run safe live verification using provided runtime secrets")
	flags.BoolVar(&opts.Managed, "managed", false, "run through the managed dari-docs service instead of a self-managed Dari org")
	flags.IntVar(&opts.TimeoutMinutes, "timeout-minutes", opts.TimeoutMinutes, "managed CLI wait timeout in minutes")
	if opts.Command == "check" {
		flags.Bool("remote-editor", false, "ignored for check")
	}
	cmd.MarkFlagsOneRequired("task", "tasks-file")
}

func checkOptimizeShort(command string) string {
	if command == "check" {
		return "Run docs checks"
	}
	return "Run docs checks and propose edits"
}

func runCheckOrOptimizeOptions(ctx context.Context, opts *checkOptimizeOptions) error {
	if err := opts.prepare(); err != nil {
		return err
	}
	if opts.Managed {
		return runManagedCheckOrOptimizeFromOptions(ctx, *opts)
	}
	return runSelfManagedCheckOrOptimize(ctx, *opts)
}

func (opts *checkOptimizeOptions) prepare() error {
	if err := opts.resolveRepoAndOutput(); err != nil {
		return err
	}
	if err := opts.loadTasks(); err != nil {
		return err
	}
	if err := opts.loadRuntimeSecrets(); err != nil {
		return err
	}
	opts.resolveLLMFlags()
	opts.BundleOptions = bundle.CreateOptions{Include: opts.BundleIncludes, Exclude: opts.BundleExcludes}
	return nil
}

func (opts *checkOptimizeOptions) resolveRepoAndOutput() error {
	repo := opts.RepoArg
	if repo == "" {
		repo = "."
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	opts.RepoRoot = absRepo
	if opts.OutDir == "" {
		opts.OutDir = filepath.Join(absRepo, ".dari-docs")
	}
	return nil
}

func (opts *checkOptimizeOptions) loadTasks() error {
	tasks := append([]string{}, opts.TaskInputs...)
	for _, path := range opts.TaskFiles {
		more, err := readTasksFile(path)
		if err != nil {
			return err
		}
		tasks = append(tasks, more...)
	}
	if len(tasks) == 0 {
		return fmt.Errorf("provide at least one non-empty --task or --tasks-file")
	}
	opts.Tasks = tasks
	return nil
}

func (opts *checkOptimizeOptions) loadRuntimeSecrets() error {
	secrets := map[string]string{}
	for _, name := range opts.SecretEnvs {
		val := os.Getenv(name)
		if val == "" {
			return fmt.Errorf("--secret-env %s requested but env var is empty", name)
		}
		secrets[name] = val
	}
	if len(opts.SecretEnvs) > 0 && !opts.LiveVerify {
		return fmt.Errorf("--secret-env requires --live-verify")
	}
	opts.RuntimeSecrets = secrets
	return nil
}

func (opts *checkOptimizeOptions) resolveLLMFlags() {
	opts.FeedbackLLMIDs = expandFeedbackLLMList(opts.FeedbackLLMRaw)
	opts.EditorLLMID = opts.EditorLLMIDFlag
	if opts.EditorLLMID == "" {
		opts.EditorLLMID = opts.LLMID
	}
}

func runManagedCheckOrOptimizeFromOptions(ctx context.Context, opts checkOptimizeOptions) error {
	if opts.APIKey != "" || opts.APIBaseURL != "" || opts.FeedbackAgent != "" || opts.EditorAgent != "" {
		return fmt.Errorf("--managed cannot be combined with --api-key, --api-base-url, --feedback-agent, or --editor-agent")
	}
	if _, err := loadManagedToken(); err != nil {
		return err
	}
	feedbackLLMIDs := opts.FeedbackLLMIDs
	if len(feedbackLLMIDs) == 0 && opts.LLMID != "" {
		feedbackLLMIDs = []string{opts.LLMID}
	}
	return runManagedCheckOrOptimize(ctx, managedRunConfig{
		Command:        opts.Command,
		RepoRoot:       opts.RepoRoot,
		OutDir:         opts.OutDir,
		Tasks:          opts.Tasks,
		FeedbackLLMIDs: feedbackLLMIDs,
		EditorLLMID:    opts.EditorLLMID,
		Apply:          opts.Apply,
		LiveVerify:     opts.LiveVerify,
		RuntimeSecrets: opts.RuntimeSecrets,
		Timeout:        time.Duration(opts.TimeoutMinutes) * time.Minute,
		BundleOptions:  opts.BundleOptions,
	})
}

func runSelfManagedCheckOrOptimize(ctx context.Context, opts checkOptimizeOptions) error {
	if err := resolveSelfManagedCheckOptimizeConfig(&opts); err != nil {
		return err
	}
	res, err := runner.Run(ctx, runner.Config{
		RepoRoot:       opts.RepoRoot,
		OutDir:         opts.OutDir,
		APIKey:         opts.APIKey,
		APIBaseURL:     opts.APIBaseURL,
		FeedbackAgent:  opts.FeedbackAgent,
		EditorAgent:    opts.EditorAgent,
		FeedbackLLMIDs: opts.FeedbackLLMIDs,
		EditorLLMID:    opts.EditorLLMID,
		Tasks:          opts.Tasks,
		LiveVerify:     opts.LiveVerify,
		RuntimeSecrets: opts.RuntimeSecrets,
		Parallel:       opts.Parallel,
		SkipEditor:     opts.Command == "check",
		Timeout:        time.Duration(opts.TimeoutMinutes) * time.Minute,
		BundleOptions:  opts.BundleOptions,
	})
	if err != nil {
		return err
	}
	if opts.Command != "check" && opts.Apply {
		if err := workspace.CopyTree(res.UpdatedDir, opts.RepoRoot); err != nil {
			return fmt.Errorf("apply updated docs: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Applied updated docs into %s\n", opts.RepoRoot)
	}
	printCheckOptimizeResult(opts.Command, opts.OutDir, opts.Apply, res)
	return nil
}

func resolveSelfManagedCheckOptimizeConfig(opts *checkOptimizeOptions) error {
	if len(opts.FeedbackLLMIDs) == 0 {
		if opts.LLMID != "" {
			opts.FeedbackLLMIDs = []string{opts.LLMID}
		} else {
			opts.FeedbackLLMIDs = runner.DefaultFeedbackLLMIDs()
		}
	}
	if c, ok, err := appconfig.Load(opts.RepoRoot); err != nil {
		return err
	} else if ok {
		if opts.FeedbackAgent == "" {
			opts.FeedbackAgent = c.TesterAgentID
		}
		if opts.EditorAgent == "" {
			opts.EditorAgent = c.EditorAgentID
		}
	}
	if opts.FeedbackAgent == "" {
		return fmt.Errorf("missing tester agent ID; run `dari-docs init --deploy` or pass --feedback-agent")
	}
	if opts.Command != "check" && opts.EditorAgent == "" {
		return fmt.Errorf("missing tester/editor agent IDs; run `dari-docs init --deploy` or pass --feedback-agent and --editor-agent")
	}
	if opts.APIKey == "" && opts.APIKeyEnv != "" {
		opts.APIKey = os.Getenv(opts.APIKeyEnv)
	}
	if opts.APIKey == "" {
		return fmt.Errorf("missing Dari API key; set %s or pass --api-key", opts.APIKeyEnv)
	}
	return nil
}

func printCheckOptimizeResult(command, outDir string, apply bool, res runner.Result) {
	fmt.Println("\nDone.")
	fmt.Printf("Bundle: %s\n", res.BundlePath)
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if command != "check" {
		fmt.Printf("Editor session: %s\n", res.EditorSessionID)
		fmt.Printf("Updated docs: %s\n", res.UpdatedDir)
		if !apply {
			fmt.Printf("Review and apply manually, or rerun with --apply.\n")
		}
	}
}
