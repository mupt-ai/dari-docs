package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/projectconfig"
	"github.com/mupt-ai/dari-docs/internal/publicdocs"
	"github.com/mupt-ai/dari-docs/internal/runner"
	"github.com/mupt-ai/dari-docs/internal/runtimeenv"
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
	PublicDocURLs  []string
	PublicDocsOnly bool
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
	Wait           bool
	TimeoutMinutes int
}

func newCheckOptimizeCommand(command string) *cobra.Command {
	opts := defaultCheckOptimizeOptions(command)
	use := command + " [repo]"
	if command == "check" {
		use = command + " [repo|docs-url]"
	}
	cmd := &cobra.Command{
		Use:           use,
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
	if opts.Command == "check" {
		flags.StringArrayVar(&opts.PublicDocURLs, "docs-url", nil, "public docs URL for agents to inspect with internet access; repeatable")
	}
	flags.StringVar(&opts.APIKeyEnv, "api-key-env", opts.APIKeyEnv, "env var containing Dari API key")
	flags.StringVar(&opts.APIKey, "api-key", "", "Dari API key (prefer --api-key-env)")
	flags.StringVar(&opts.APIBaseURL, "api-base-url", opts.APIBaseURL, "Dari API base URL (defaults to production)")
	flags.StringVar(&opts.FeedbackAgent, "feedback-agent", "", "tester Flue agent ID (defaults to .dari-docs/config.json)")
	if opts.Command != "check" {
		flags.StringVar(&opts.EditorAgent, "editor-agent", "", "editor Flue agent ID (defaults to .dari-docs/config.json)")
	}
	flags.StringVar(&opts.LLMID, "llm", "", "unsupported; configure the model in the Flue agent project")
	flags.StringSliceVar(&opts.FeedbackLLMRaw, "feedback-llm", nil, "unsupported; configure the tester model in the Flue agent project")
	flags.StringVar(&opts.EditorLLMIDFlag, "editor-llm", "", "unsupported; configure the editor model in the Flue agent project")
	_ = flags.MarkHidden("llm")
	_ = flags.MarkHidden("feedback-llm")
	_ = flags.MarkHidden("editor-llm")
	flags.StringVar(&opts.OutDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	flags.IntVar(&opts.Parallel, "parallel", opts.Parallel, "tester sessions per batch")
	if opts.Command != "check" {
		flags.BoolVar(&opts.Apply, "apply", false, "copy updated docs back into the repo after downloading")
	}
	flags.BoolVar(&opts.LiveVerify, "live-verify", false, "allow agents to run safe live verification using provided runtime secrets")
	flags.BoolVar(&opts.Managed, "managed", false, "unsupported; use deployed Flue agents in your Dari org")
	flags.BoolVar(&opts.Wait, "wait", false, "unsupported; Flue runs wait for sessions by default")
	_ = flags.MarkHidden("managed")
	_ = flags.MarkHidden("wait")
	flags.IntVar(&opts.TimeoutMinutes, "timeout-minutes", opts.TimeoutMinutes, "CLI wait timeout in minutes")
	cmd.MarkFlagsOneRequired("task", "tasks-file")
}

func checkOptimizeShort(command string) string {
	if command == "check" {
		return "Run Flue tester agents against your docs"
	}
	return "Run Flue tester and editor agents against your docs"
}

func runCheckOrOptimizeOptions(ctx context.Context, opts *checkOptimizeOptions) error {
	if err := opts.prepare(); err != nil {
		return err
	}
	if opts.Managed {
		return unsupportedManagedModeError()
	}
	if opts.Wait {
		return fmt.Errorf("--wait is not supported for Flue runs; check and optimize already wait for the Flue sessions to finish")
	}
	return runFlueCheckOrOptimize(ctx, *opts)
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
	if opts.Command == "optimize" && len(opts.PublicDocURLs) > 0 {
		return fmt.Errorf("public docs URLs support check only; optimize requires local docs files")
	}
	if opts.Apply && len(opts.PublicDocURLs) > 0 {
		return fmt.Errorf("--apply cannot be used with public docs URLs; review downloaded updated docs manually")
	}
	opts.BundleOptions = bundle.CreateOptions{Include: opts.BundleIncludes, Exclude: opts.BundleExcludes}
	if len(opts.PublicDocURLs) > 0 {
		urls, err := publicdocs.NormalizeURLs(opts.PublicDocURLs)
		if err != nil {
			return err
		}
		opts.PublicDocURLs = urls
		fmt.Fprintf(os.Stderr, "Using public docs source: %s\n", opts.PublicDocURLs[0])
	}
	return nil
}

func (opts *checkOptimizeOptions) resolveRepoAndOutput() error {
	if publicdocs.IsURL(opts.RepoArg) {
		opts.PublicDocURLs = append([]string{opts.RepoArg}, opts.PublicDocURLs...)
		opts.PublicDocsOnly = true
		opts.RepoArg = ""
	}
	if len(opts.PublicDocURLs) > 0 && opts.RepoArg == "" {
		opts.PublicDocsOnly = true
	}
	if opts.PublicDocsOnly {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		if opts.OutDir == "" {
			opts.OutDir = filepath.Join(cwd, ".dari-docs")
		}
		opts.RepoRoot = cwd
		return nil
	}
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
	secrets := make(map[string]string, len(opts.SecretEnvs))
	for _, rawName := range opts.SecretEnvs {
		name, err := runtimeenv.ValidateName(rawName)
		if err != nil {
			return err
		}
		if _, ok := secrets[name]; ok {
			return fmt.Errorf("runtime secret name %q is duplicated", name)
		}
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

func runFlueCheckOrOptimize(ctx context.Context, opts checkOptimizeOptions) error {
	if err := resolveFlueCheckOptimizeConfig(&opts); err != nil {
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
		PublicDocURLs:  opts.PublicDocURLs,
		PublicDocsOnly: opts.PublicDocsOnly,
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

func resolveFlueCheckOptimizeConfig(opts *checkOptimizeOptions) error {
	if opts.LLMID != "" || len(opts.FeedbackLLMRaw) > 0 || opts.EditorLLMIDFlag != "" {
		return fmt.Errorf("Flue agents do not support --llm, --feedback-llm, or --editor-llm; configure the model in the Flue agent project, then run `dari-docs init --deploy`")
	}
	if c, ok, err := projectconfig.Load(opts.RepoRoot); err != nil {
		return err
	} else if ok {
		if c.AgentRuntime != "flue" {
			return fmt.Errorf("this repo's .dari-docs/config.json was not created for Flue agents; run `dari-docs init --deploy` to deploy the bundled Flue agents")
		}
		if opts.FeedbackAgent == "" {
			opts.FeedbackAgent = c.TesterAgentID
		}
		if opts.EditorAgent == "" {
			opts.EditorAgent = c.EditorAgentID
		}
	}
	opts.FeedbackLLMIDs = []string{""}
	opts.EditorLLMID = ""
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
	if res.BundlePath != "" {
		fmt.Printf("Bundle: %s\n", res.BundlePath)
	}
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if command != "check" {
		fmt.Printf("Editor session: %s\n", res.EditorSessionID)
		fmt.Printf("Updated docs: %s\n", res.UpdatedDir)
		if !apply {
			fmt.Printf("Review and apply manually, or rerun with --apply.\n")
		}
	}
}
