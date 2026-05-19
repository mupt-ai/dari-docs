package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	appconfig "github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/runner"
)

func runCheckOrOptimize(cmd string, args []string) error {
	fs := flag.NewFlagSet("dari-docs "+cmd, flag.ExitOnError)
	var tasks repeated
	var taskFiles repeated
	var secretEnvs repeated
	var bundleIncludes repeated
	var bundleExcludes repeated
	var apiKeyEnv string
	var apiKey string
	var apiBaseURL string
	var feedbackAgent string
	var editorAgent string
	var llmID string
	var feedbackLLMIDs repeated
	var editorLLMID string
	var outDir string
	var parallel int
	var apply bool
	var liveVerify bool
	var managedMode bool
	var timeoutMinutes int
	fs.Var(&tasks, "task", "implementation task/prompt to test; repeatable")
	fs.Var(&taskFiles, "tasks-file", "file containing tasks, one per paragraph or bullet; repeatable")
	fs.Var(&secretEnvs, "secret-env", "runtime product/API secret env var to pass to sessions; repeatable")
	fs.Var(&bundleIncludes, "bundle-include", "repo-relative glob to include in the docs bundle in addition to defaults; repeatable")
	fs.Var(&bundleExcludes, "bundle-exclude", "repo-relative glob to exclude from the docs bundle; repeatable")
	fs.StringVar(&apiKeyEnv, "api-key-env", "DARI_API_KEY", "env var containing Dari API key")
	fs.StringVar(&apiKey, "api-key", "", "Dari API key (prefer --api-key-env)")
	fs.StringVar(&apiBaseURL, "api-base-url", os.Getenv("DARI_API_BASE_URL"), "Dari API base URL (defaults to production)")
	fs.StringVar(&feedbackAgent, "feedback-agent", "", "Dari docs user-test agent ID (defaults to .dari-docs/config.json)")
	fs.StringVar(&editorAgent, "editor-agent", "", "Dari docs editor agent ID (defaults to .dari-docs/config.json)")
	fs.StringVar(&llmID, "llm", "", "manifest LLM option ID to use for all sessions")
	fs.Var(&feedbackLLMIDs, "feedback-llm", "manifest LLM option ID or group for feedback/tester sessions; repeat or comma-separate (groups: all, claude, gpt; overrides --llm)")
	fs.StringVar(&editorLLMID, "editor-llm", "", "manifest LLM option ID for the editor session (overrides --llm)")
	fs.StringVar(&outDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	fs.IntVar(&parallel, "parallel", 4, "number of feedback sessions per self-managed batch")
	fs.BoolVar(&apply, "apply", false, "copy updated docs back into the repo after downloading")
	fs.BoolVar(&liveVerify, "live-verify", false, "allow agents to run safe live verification using provided runtime secrets")
	fs.BoolVar(&managedMode, "managed", false, "run through the managed dari-docs service instead of a self-managed Dari org")
	fs.IntVar(&timeoutMinutes, "timeout-minutes", 30, "managed CLI wait timeout in minutes")
	if cmd == "check" {
		fs.Bool("remote-editor", false, "ignored for check")
	}
	repoArg := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repoArg = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	repo := "."
	if repoArg != "" {
		repo = repoArg
	} else if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = filepath.Join(absRepo, ".dari-docs")
	}
	allTasks := append([]string{}, tasks...)
	for _, p := range taskFiles {
		more, err := readTasksFile(p)
		if err != nil {
			return err
		}
		allTasks = append(allTasks, more...)
	}
	if len(allTasks) == 0 {
		return fmt.Errorf("provide at least one --task or --tasks-file")
	}
	secrets := map[string]string{}
	for _, name := range secretEnvs {
		val := os.Getenv(name)
		if val == "" {
			return fmt.Errorf("--secret-env %s requested but env var is empty", name)
		}
		secrets[name] = val
	}
	if len(secretEnvs) > 0 && !liveVerify {
		return fmt.Errorf("--secret-env requires --live-verify")
	}
	feedbackLLMList := expandFeedbackLLMList(feedbackLLMIDs)
	if editorLLMID == "" {
		editorLLMID = llmID
	}
	if managedMode {
		if apiKey != "" || apiBaseURL != "" || feedbackAgent != "" || editorAgent != "" {
			return fmt.Errorf("--managed cannot be combined with --api-key, --api-base-url, --feedback-agent, or --editor-agent")
		}
		if _, err := loadManagedToken(); err != nil {
			return err
		}
		if len(feedbackLLMList) == 0 && llmID != "" {
			feedbackLLMList = []string{llmID}
		}
		return runManagedCheckOrOptimize(context.Background(), managedRunConfig{
			Command: cmd, RepoRoot: absRepo, OutDir: outDir,
			Tasks: allTasks, FeedbackLLMIDs: feedbackLLMList, EditorLLMID: editorLLMID, Apply: apply,
			LiveVerify: liveVerify, RuntimeSecrets: secrets, Timeout: time.Duration(timeoutMinutes) * time.Minute,
			BundleOptions: bundle.CreateOptions{Include: bundleIncludes, Exclude: bundleExcludes},
		})
	}
	if len(feedbackLLMList) == 0 {
		if llmID != "" {
			feedbackLLMList = []string{llmID}
		} else {
			feedbackLLMList = defaultFeedbackLLMIDs()
		}
	}
	if c, ok, err := appconfig.Load(absRepo); err != nil {
		return err
	} else if ok {
		if feedbackAgent == "" {
			feedbackAgent = c.TesterAgentID
		}
		if editorAgent == "" {
			editorAgent = c.EditorAgentID
		}
	}
	if feedbackAgent == "" {
		return fmt.Errorf("missing tester agent ID; run `dari-docs init --deploy` or pass --feedback-agent")
	}
	if cmd != "check" && editorAgent == "" {
		return fmt.Errorf("missing tester/editor agent IDs; run `dari-docs init --deploy` or pass --feedback-agent and --editor-agent")
	}
	if apiKey == "" && apiKeyEnv != "" {
		apiKey = os.Getenv(apiKeyEnv)
	}
	if apiKey == "" {
		return fmt.Errorf("missing Dari API key; set %s or pass --api-key", apiKeyEnv)
	}
	cfg := runner.Config{
		RepoRoot: absRepo, OutDir: outDir, APIKey: apiKey, APIBaseURL: apiBaseURL,
		FeedbackAgent: feedbackAgent, EditorAgent: editorAgent, FeedbackLLMIDs: feedbackLLMList, EditorLLMID: editorLLMID, Tasks: allTasks, LiveVerify: liveVerify,
		RuntimeSecrets: secrets, Parallel: parallel, Apply: apply, SkipEditor: cmd == "check", Timeout: time.Duration(timeoutMinutes) * time.Minute,
		BundleOptions: bundle.CreateOptions{Include: bundleIncludes, Exclude: bundleExcludes},
	}
	res, err := runner.Run(context.Background(), cfg)
	if err != nil {
		return err
	}
	fmt.Println("\nDone.")
	fmt.Printf("Bundle: %s\n", res.BundlePath)
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if cmd != "check" {
		fmt.Printf("Editor session: %s\n", res.EditorSessionID)
		fmt.Printf("Updated docs: %s\n", res.UpdatedDir)
		if !apply {
			fmt.Printf("Review and apply manually, or rerun with --apply.\n")
		}
	}
	return nil
}
