package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/harborclient"
	"github.com/mupt-ai/dari-docs/internal/projectconfig"
	"github.com/mupt-ai/dari-docs/internal/publicdocs"
	"github.com/mupt-ai/dari-docs/internal/runtimeenv"
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

	TesterURL string
	EditorURL string

	Harness        string
	EditorHarness  string
	Sandbox        string
	AgentKwargs    []string
	AgentEnv       []string
	SandboxKwargs  []string
	Execution      *projectconfig.Execution
	RunnerTokenEnv string
	RunnerToken    string

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
		Parallel:       1,
		TimeoutMinutes: 60,
	}
}

func bindCheckOptimizeFlags(cmd *cobra.Command, opts *checkOptimizeOptions) {
	flags := cmd.Flags()
	flags.StringVar(&opts.Harness, "harness", "", "Harbor harness name or module:Class (server default when unset)")
	flags.StringVar(&opts.EditorHarness, "editor-harness", "", "editor Harbor harness; overrides --harness")
	flags.StringVar(&opts.Sandbox, "sandbox", "", "Harbor sandbox name or module:Class (server default when unset)")
	flags.StringArrayVar(&opts.AgentKwargs, "agent-kwarg", nil, "Harbor agent KEY=JSON_OR_STRING; repeatable")
	flags.StringArrayVar(&opts.AgentEnv, "agent-env", nil, "nonsecret agent environment NAME=VALUE; repeatable")
	flags.StringArrayVar(&opts.SandboxKwargs, "sandbox-kwarg", nil, "Harbor environment KEY=JSON_OR_STRING; repeatable")
	flags.StringVar(&opts.RunnerTokenEnv, "runner-token-env", "", "bearer token environment variable (default: DARI_DOCS_RUNNER_TOKEN if present)")
	flags.StringArrayVar(&opts.TaskInputs, "task", nil, "implementation task/prompt to test; repeatable")
	flags.StringArrayVar(&opts.TaskFiles, "tasks-file", nil, "file containing tasks, one per paragraph or bullet; repeatable")
	flags.StringArrayVar(&opts.SecretEnvs, "secret-env", nil, "runtime product/API secret env var to pass to workflow payloads; repeatable")
	flags.StringArrayVar(&opts.BundleIncludes, "bundle-include", nil, "repo-relative glob to include in the docs bundle in addition to defaults; repeatable")
	flags.StringArrayVar(&opts.BundleExcludes, "bundle-exclude", nil, "repo-relative glob to exclude from the docs bundle; repeatable")
	if opts.Command == "check" {
		flags.StringArrayVar(&opts.PublicDocURLs, "docs-url", nil, "public docs URL for the tester workflow to inspect with internet access; repeatable")
	}
	flags.StringVar(&opts.TesterURL, "tester-url", "", "base URL of the deployed tester Harbor app (defaults to .dari-docs/config.json)")
	if opts.Command != "check" {
		flags.StringVar(&opts.EditorURL, "editor-url", "", "base URL of the deployed editor Harbor app (defaults to .dari-docs/config.json)")
	}
	flags.StringVar(&opts.APIKeyEnv, "api-key-env", opts.APIKeyEnv, "unsupported; Dari API keys are not used by Harbor deployments")
	flags.StringVar(&opts.APIKey, "api-key", "", "unsupported; Dari API keys are not used by Harbor deployments")
	flags.StringVar(&opts.APIBaseURL, "api-base-url", opts.APIBaseURL, "unsupported; Dari API URLs are not used by Harbor deployments")
	flags.StringVar(&opts.FeedbackAgent, "feedback-agent", "", "unsupported; use --tester-url for Harbor deployments")
	if opts.Command != "check" {
		flags.StringVar(&opts.EditorAgent, "editor-agent", "", "unsupported; use --editor-url for Harbor deployments")
	}
	flags.StringVar(&opts.LLMID, "llm", "", "model to request for tester and editor workflows unless overridden")
	flags.StringSliceVar(&opts.FeedbackLLMRaw, "feedback-llm", nil, "tester model or group; repeat or comma-separate (groups: all, claude, gpt); overrides --llm")
	if opts.Command != "check" {
		flags.StringVar(&opts.EditorLLMIDFlag, "editor-llm", "", "editor model to request for optimize; overrides --llm")
	}
	_ = flags.MarkHidden("api-key-env")
	_ = flags.MarkHidden("api-key")
	_ = flags.MarkHidden("api-base-url")
	_ = flags.MarkHidden("feedback-agent")
	_ = flags.MarkHidden("editor-agent")
	flags.StringVar(&opts.OutDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	flags.IntVar(&opts.Parallel, "parallel", opts.Parallel, "maximum concurrent tester workflow runs")
	if opts.Command != "check" {
		flags.BoolVar(&opts.Apply, "apply", false, "copy updated docs back into the repo after downloading")
	}
	flags.BoolVar(&opts.LiveVerify, "live-verify", false, "allow agents to run safe live verification using provided runtime secrets")
	flags.BoolVar(&opts.Managed, "managed", false, "unsupported; hosted managed mode has been removed")
	flags.BoolVar(&opts.Wait, "wait", false, "unsupported; Harbor workflow runs wait by default")
	_ = flags.MarkHidden("managed")
	_ = flags.MarkHidden("wait")
	flags.IntVar(&opts.TimeoutMinutes, "timeout-minutes", opts.TimeoutMinutes, "CLI wait timeout in minutes")
	cmd.MarkFlagsOneRequired("task", "tasks-file")
}

func checkOptimizeShort(command string) string {
	if command == "check" {
		return "Run a Harbor tester app against your docs"
	}
	return "Run Harbor tester and editor apps against your docs"
}

func runCheckOrOptimizeOptions(ctx context.Context, opts *checkOptimizeOptions) error {
	if err := opts.prepare(); err != nil {
		return err
	}
	if opts.Managed {
		return unsupportedManagedModeError()
	}
	if opts.Wait {
		return fmt.Errorf("--wait is not supported for Harbor runs; check and optimize already wait for the Harbor workflow result")
	}
	return runHarborCheckOrOptimize(ctx, *opts)
}

func (opts *checkOptimizeOptions) prepare() error {
	if opts.TimeoutMinutes <= 0 || int64(opts.TimeoutMinutes) > int64((1<<63-1)/time.Minute) {
		return fmt.Errorf("--timeout-minutes must be positive and fit in a Go duration")
	}
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
	if opts.Parallel == 0 {
		opts.Parallel = 1
	}
	if opts.Parallel < 0 {
		return fmt.Errorf("--parallel must be at least 1")
	}
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
	if len(opts.FeedbackLLMIDs) == 0 && opts.LLMID != "" {
		opts.FeedbackLLMIDs = []string{opts.LLMID}
	}
	opts.EditorLLMID = opts.EditorLLMIDFlag
	if opts.EditorLLMID == "" {
		opts.EditorLLMID = opts.LLMID
	}
}

func runHarborCheckOrOptimize(ctx context.Context, opts checkOptimizeOptions) error {
	if err := resolveHarborCheckOptimizeConfig(&opts); err != nil {
		return err
	}
	res, err := harborclient.Run(ctx, harborclient.Config{
		Execution:       opts.Execution,
		EditorExecution: &projectconfig.Execution{Harness: opts.EditorHarness},
		RunnerToken:     opts.RunnerToken,
		RepoRoot:        opts.RepoRoot,
		OutDir:          opts.OutDir,
		TesterURL:       opts.TesterURL,
		EditorURL:       opts.EditorURL,
		Tasks:           opts.Tasks,
		FeedbackModels:  opts.FeedbackLLMIDs,
		EditorModel:     opts.EditorLLMID,
		LiveVerify:      opts.LiveVerify,
		RuntimeSecrets:  opts.RuntimeSecrets,
		PublicDocURLs:   opts.PublicDocURLs,
		PublicDocsOnly:  opts.PublicDocsOnly,
		SkipEditor:      opts.Command == "check",
		Timeout:         time.Duration(opts.TimeoutMinutes) * time.Minute,
		Parallel:        opts.Parallel,
		BundleOptions:   opts.BundleOptions,
	})
	if err != nil {
		return err
	}
	if opts.Command != "check" && opts.Apply {
		if err := harborclient.ApplyUpdatedDocs(res.UpdatedDir, opts.RepoRoot); err != nil {
			return fmt.Errorf("apply updated docs: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Applied updated docs into %s\n", opts.RepoRoot)
	}
	printCheckOptimizeResult(opts.Command, opts.OutDir, opts.Apply, res)
	return nil
}

func resolveHarborCheckOptimizeConfig(opts *checkOptimizeOptions) error {
	if opts.APIKey != "" || opts.APIKeyEnv != "" || opts.APIBaseURL != "" || opts.FeedbackAgent != "" || opts.EditorAgent != "" {
		return fmt.Errorf("Dari API and agent ID flags are not supported; deploy the Modal Harbor agents and pass --tester-url/--editor-url")
	}
	if c, ok, err := projectconfig.Load(opts.RepoRoot); err != nil {
		return err
	} else if ok {
		if c.AgentRuntime != "harbor" || c.LLMMode != "harbor-env" {
			return fmt.Errorf("this repo's .dari-docs/config.json was not created for Harbor deployments; run `dari-docs init` and redeploy the bundled Harbor runner")
		}
		opts.Execution = c.Execution
		if opts.TesterURL == "" {
			opts.TesterURL = c.TesterURL
		}
		if opts.EditorURL == "" {
			opts.EditorURL = c.EditorURL
		}
	}
	if err := opts.resolveExecution(); err != nil {
		return err
	}
	if opts.TesterURL == "" {
		return fmt.Errorf("missing Harbor tester URL; deploy .dari-docs/agents/modal_app.py with Modal, then pass --tester-url")
	}
	if opts.Command != "check" && opts.EditorURL == "" {
		return fmt.Errorf("missing Harbor editor URL; deploy .dari-docs/agents/modal_app.py with Modal, then pass --editor-url")
	}
	return nil
}

func printCheckOptimizeResult(command, outDir string, apply bool, res harborclient.Result) {
	fmt.Println("\nDone.")
	if res.BundlePath != "" {
		fmt.Printf("Bundle: %s\n", res.BundlePath)
	}
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if command != "check" {
		fmt.Printf("Updated docs: %s\n", res.UpdatedDir)
		if !apply {
			fmt.Printf("Review and apply manually, or rerun with --apply.\n")
		}
	}
}
