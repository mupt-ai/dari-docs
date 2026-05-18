package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/mupt-ai/dari-docs/internal/agenttemplates"
	"github.com/mupt-ai/dari-docs/internal/bundle"
	appconfig "github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/managed"
	"github.com/mupt-ai/dari-docs/internal/platformauth"
	"github.com/mupt-ai/dari-docs/internal/runner"
	"github.com/mupt-ai/dari-docs/internal/workspace"
	"gopkg.in/yaml.v3"
)

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func defaultFeedbackLLMIDs() []string {
	return runner.DefaultFeedbackLLMIDs()
}

var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dari-docs: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cmd := "optimize"
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "init", "optimize", "check", "auth", "billing", "agents", "help", "-h", "--help", "version", "-v", "--version":
			cmd = args[0]
			args = args[1:]
		}
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		usage()
		return nil
	}
	if cmd == "version" || cmd == "-v" || cmd == "--version" {
		fmt.Println(versionLine())
		return nil
	}
	if cmd == "init" {
		return runInit(args)
	}
	if cmd == "auth" {
		return runAuth(args)
	}
	if cmd == "billing" {
		return runBilling(args)
	}
	if cmd == "agents" {
		return runAgents(args)
	}
	return runCheckOrOptimize(cmd, args)
}

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
	fs.Var(&feedbackLLMIDs, "feedback-llm", "manifest LLM option ID for feedback/tester sessions; repeat or comma-separate (overrides --llm)")
	fs.StringVar(&editorLLMID, "editor-llm", "", "manifest LLM option ID for the editor session (overrides --llm)")
	fs.StringVar(&outDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	fs.IntVar(&parallel, "parallel", 4, "number of feedback sessions per self-managed batch")
	fs.BoolVar(&apply, "apply", false, "copy updated docs back into the repo after downloading")
	fs.BoolVar(&liveVerify, "live-verify", false, "allow agents to run safe live verification using provided runtime secrets")
	fs.BoolVar(&managedMode, "managed", false, "run through the managed dari-docs service instead of a self-managed Dari org")
	fs.IntVar(&timeoutMinutes, "timeout-minutes", 15, "per-session timeout in minutes")
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
	feedbackLLMList := expandCSVList(feedbackLLMIDs)
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

type managedRunConfig struct {
	Command        string
	RepoRoot       string
	OutDir         string
	Tasks          []string
	FeedbackLLMIDs []string
	EditorLLMID    string
	LiveVerify     bool
	RuntimeSecrets map[string]string
	Apply          bool
	Timeout        time.Duration
	BundleOptions  bundle.CreateOptions
}

func runManagedCheckOrOptimize(ctx context.Context, cfg managedRunConfig) error {
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return err
	}
	auth, err := loadManagedAuthToken()
	if err != nil {
		return err
	}
	client := managed.NewWithAuthToken(managed.DefaultBaseURL, auth)
	runCfg, err := client.RunConfig(ctx)
	if err != nil {
		return err
	}
	feedbackLLMIDs, editorLLMID, err := managedLLMSelection(cfg.FeedbackLLMIDs, cfg.EditorLLMID, runCfg)
	if err != nil {
		return err
	}
	bundlePath := filepath.Join(cfg.OutDir, "input-docs-bundle.tar.gz")
	cfg.BundleOptions.MaxFileBytes = runCfg.BundleMaxFileBytes
	b, err := bundle.CreateWithOptions(cfg.RepoRoot, bundlePath, cfg.BundleOptions)
	if err != nil {
		return err
	}
	bundle.WriteSummary(os.Stderr, b)

	bal, err := client.Balance(ctx)
	if err != nil {
		return err
	}
	reserve := managedRunReserveCents(cfg.Command, len(cfg.Tasks), len(feedbackLLMIDs), runCfg)
	fmt.Fprintln(os.Stderr, "\nManaged run estimate:")
	fmt.Fprintf(os.Stderr, "  Balance: %s\n", formatCents(bal.BalanceCents))
	fmt.Fprintf(os.Stderr, "  Sessions: %s\n", managedSessionSummary(cfg.Command, len(cfg.Tasks), len(feedbackLLMIDs)))
	fmt.Fprintf(os.Stderr, "  Tester LLMs: %s\n", strings.Join(feedbackLLMIDs, ", "))
	if cfg.Command != "check" {
		fmt.Fprintf(os.Stderr, "  Editor LLM: %s\n", editorLLMID)
	}
	fmt.Fprintf(os.Stderr, "  Reserved before start: %s\n", formatCents(reserve))
	fmt.Fprintln(os.Stderr, "  Final charge reconciles to actual session cost after completion.")

	runtimeSecretJSON := ""
	if cfg.LiveVerify && len(cfg.RuntimeSecrets) > 0 {
		b, err := json.Marshal(cfg.RuntimeSecrets)
		if err != nil {
			return fmt.Errorf("encode runtime secrets: %w", err)
		}
		runtimeSecretJSON = string(b)
	}
	created, err := client.CreateRun(ctx, cfg.Command, cfg.Tasks, bundlePath, managed.CreateRunOptions{
		LiveVerify:         cfg.LiveVerify,
		RuntimeSecretsJSON: runtimeSecretJSON,
		FeedbackLLMIDs:     feedbackLLMIDs,
		EditorLLMID:        editorLLMID,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Managed run: %s\n", created.RunID)
	fmt.Fprintf(os.Stderr, "Reserved: %s\n", formatCents(reserve))
	deadline := time.Now().Add(managedRunTimeout(cfg.Command, cfg.Timeout))
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	var status managed.RunStatus
	for {
		status, err = client.GetRun(ctx, created.RunID)
		if err != nil {
			return err
		}
		if status.Status == "completed" || status.Status == "failed" {
			break
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("timeout waiting for managed run %s status=%q", created.RunID, status.Status)
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
	if err := writeManagedFeedback(cfg.OutDir, status.FeedbackReports, status.AggregateFeedback); err != nil {
		return err
	}
	if status.Status == "failed" {
		return fmt.Errorf("managed run %s failed: %s", status.ID, status.Error)
	}
	fmt.Println("\nDone.")
	fmt.Printf("Bundle: %s\n", bundlePath)
	fmt.Printf("Feedback: %s\n", filepath.Join(cfg.OutDir, "aggregate-feedback.md"))
	fmt.Printf("Managed run: %s\n", status.ID)
	fmt.Printf("Charged: %s\n", formatCents(status.ChargedCents))
	if status.ReservedCents > status.ChargedCents {
		fmt.Printf("Released: %s\n", formatCents(status.ReservedCents-status.ChargedCents))
	} else if status.ChargedCents > status.ReservedCents {
		fmt.Printf("Overage: %s\n", formatCents(status.ChargedCents-status.ReservedCents))
	}
	if finalBalance, err := client.Balance(ctx); err == nil {
		fmt.Printf("Balance: %s\n", formatCents(finalBalance.BalanceCents))
	}
	if cfg.Command != "check" {
		zipPath := filepath.Join(cfg.OutDir, "updated-docs-workspace.zip")
		if err := client.DownloadUpdatedDocs(ctx, status.ID, zipPath); err != nil {
			return err
		}
		extractDir := filepath.Join(cfg.OutDir, "updated")
		_ = os.RemoveAll(extractDir)
		if err := dari.ExtractZip(zipPath, extractDir); err != nil {
			return err
		}
		updatedDir, err := workspace.UpdatedRoot(extractDir)
		if err != nil {
			return err
		}
		fmt.Printf("Updated docs: %s\n", updatedDir)
		if cfg.Apply {
			if err := workspace.CopyTree(updatedDir, cfg.RepoRoot); err != nil {
				return fmt.Errorf("apply updated docs: %w", err)
			}
			fmt.Fprintf(os.Stderr, "Applied updated docs into %s\n", cfg.RepoRoot)
		} else {
			fmt.Printf("Review and apply manually, or rerun with --apply.\n")
		}
	}
	return nil
}

func managedRunReserveCents(command string, taskCount int, testerLLMCount int, cfg managed.RunConfig) int64 {
	if testerLLMCount <= 0 {
		testerLLMCount = 1
	}
	reserve := int64(taskCount*testerLLMCount) * cfg.TesterSessionReserveCents
	if command != "check" {
		reserve += cfg.EditorSessionReserveCents
	}
	return reserve
}

func managedSessionSummary(command string, taskCount int, testerLLMCount int) string {
	if testerLLMCount <= 0 {
		testerLLMCount = 1
	}
	testerSessions := taskCount * testerLLMCount
	tester := fmt.Sprintf("%d tester", testerSessions)
	if testerSessions != 1 {
		tester += " sessions"
	} else {
		tester += " session"
	}
	if testerLLMCount > 1 {
		taskLabel := "tasks"
		if taskCount == 1 {
			taskLabel = "task"
		}
		tester += fmt.Sprintf(" (%d %s x %d LLMs)", taskCount, taskLabel, testerLLMCount)
	}
	if command == "check" {
		return tester
	}
	return tester + " + 1 editor session"
}

func managedRunTimeout(command string, base time.Duration) time.Duration {
	if command == "check" {
		return base
	}
	return 2 * base
}

func managedLLMSelection(feedbackLLMIDs []string, editorLLMID string, cfg managed.RunConfig) ([]string, string, error) {
	allowed := cfg.AllowedLLMIDs
	if len(allowed) == 0 {
		return nil, "", fmt.Errorf("managed service did not return allowed LLM IDs")
	}
	defaultLLM := strings.TrimSpace(cfg.DefaultLLMID)
	if defaultLLM == "" {
		defaultLLM = allowed[0]
	}
	if len(feedbackLLMIDs) == 0 {
		feedbackLLMIDs = append([]string{}, allowed...)
	}
	feedback, err := validateManagedLLMIDs(feedbackLLMIDs, allowed)
	if err != nil {
		return nil, "", err
	}
	if editorLLMID == "" {
		editorLLMID = defaultLLM
	}
	editor, err := validateManagedLLMID(editorLLMID, allowed)
	if err != nil {
		return nil, "", err
	}
	return feedback, editor, nil
}

func validateManagedLLMIDs(llmIDs []string, allowed []string) ([]string, error) {
	out := make([]string, 0, len(llmIDs))
	seen := map[string]bool{}
	for _, raw := range llmIDs {
		llmID, err := validateManagedLLMID(raw, allowed)
		if err != nil {
			return nil, err
		}
		if seen[llmID] {
			continue
		}
		seen[llmID] = true
		out = append(out, llmID)
	}
	return out, nil
}

func validateManagedLLMID(llmID string, allowed []string) (string, error) {
	llmID = strings.TrimSpace(llmID)
	for _, candidate := range allowed {
		if llmID == candidate {
			return llmID, nil
		}
	}
	return "", fmt.Errorf("managed mode supports only these LLM IDs: %s", strings.Join(allowed, ", "))
}

func writeManagedFeedback(outDir string, reports []string, aggregate string) error {
	if aggregate == "" {
		aggregate = runner.AggregateFeedback(reports)
	}
	if err := os.MkdirAll(filepath.Join(outDir, "runs"), 0o755); err != nil {
		return err
	}
	for i, report := range reports {
		path := filepath.Join(outDir, "runs", fmt.Sprintf("feedback-%03d.md", i+1))
		if err := os.WriteFile(path, []byte(report+"\n"), 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(outDir, "aggregate-feedback.md"), []byte(aggregate), 0o644)
}

func runAuth(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dari-docs auth [login|logout|status|token]")
	}
	switch args[0] {
	case "login":
		return runAuthLogin(args[1:])
	case "logout":
		return runAuthLogout(args[1:])
	case "status":
		return runAuthStatus(args[1:])
	case "token":
		return runAuthToken(args[1:])
	default:
		return fmt.Errorf("usage: dari-docs auth [login|logout|status|token]")
	}
}

func runAuthLogin(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth login", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	if token, err := managed.LoadToken(managed.DefaultBaseURL); err != nil {
		return err
	} else if strings.TrimSpace(token) != "" {
		client := managed.NewWithAuthToken(managed.DefaultBaseURL, managed.AuthToken{Token: token, Source: managed.AuthSourceLocal})
		me, err := client.Me(ctx)
		if err == nil {
			fmt.Printf("Already logged in to %s as %s\n", managed.DefaultBaseURL, me.Email)
			return nil
		}
		var httpErr *managed.HTTPError
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
			return err
		}
		if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
			return err
		}
	}
	verified, err := exchangeManagedBrowserLogin(ctx)
	if err != nil {
		return err
	}
	if err := managed.SaveToken(managed.DefaultBaseURL, verified.Token); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", managed.DefaultBaseURL, verified.Email)
	return nil
}

func exchangeManagedBrowserLogin(ctx context.Context) (managed.DariExchangeResponse, error) {
	authConfig, err := platformauth.FetchConfig(ctx, "https://api.dari.dev")
	if err != nil {
		return managed.DariExchangeResponse{}, err
	}
	session, err := platformauth.LoginWithBrowser(ctx, authConfig, os.Stdin, os.Stderr)
	if err != nil {
		return managed.DariExchangeResponse{}, err
	}
	client := managed.New(managed.DefaultBaseURL, "")
	return client.ExchangeDariToken(ctx, session.AccessToken)
}

func runAuthLogout(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth logout", flag.ExitOnError)
	var all bool
	var interactiveOnly bool
	var automationOnly bool
	fs.BoolVar(&all, "all", false, "revoke all managed service tokens for this account")
	fs.BoolVar(&interactiveOnly, "interactive-only", false, "with --all, revoke only browser-login tokens")
	fs.BoolVar(&automationOnly, "automation-only", false, "with --all, revoke only automation tokens")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if interactiveOnly && automationOnly {
		return fmt.Errorf("--interactive-only cannot be combined with --automation-only")
	}
	if (interactiveOnly || automationOnly) && !all {
		return fmt.Errorf("--interactive-only and --automation-only require --all")
	}
	if all {
		kind := ""
		switch {
		case interactiveOnly:
			kind = "interactive"
		case automationOnly:
			kind = "automation"
		}
		return runAuthLogoutAll(context.Background(), kind)
	}
	auth, err := managed.LoadAuthToken(managed.DefaultBaseURL)
	if err != nil {
		return err
	}
	if auth.Token == "" {
		fmt.Printf("Already logged out locally.\nTo revoke server-side tokens from other devices or deleted local sessions, run `dari-docs auth logout --all`.\n")
		return nil
	}
	client := managed.NewWithAuthToken(managed.DefaultBaseURL, auth)
	if err := client.Logout(context.Background()); err != nil {
		var httpErr *managed.HTTPError
		var invalidEnv *managed.InvalidEnvTokenError
		if errors.As(err, &invalidEnv) {
			return err
		}
		if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
			return err
		}
	}
	if auth.Source == managed.AuthSourceLocal {
		if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
			return err
		}
		fmt.Printf("Logged out of %s\n", managed.DefaultBaseURL)
	} else {
		fmt.Printf("Revoked token from %s. Unset %s to stop using it locally.\n", managed.EnvTokenName, managed.EnvTokenName)
	}
	return nil
}

func runAuthLogoutAll(ctx context.Context, kind string) error {
	auth, err := managed.LoadAuthToken(managed.DefaultBaseURL)
	if err != nil {
		return err
	}
	if auth.Token != "" {
		client := managed.NewWithAuthToken(managed.DefaultBaseURL, auth)
		if err := client.LogoutAllKind(ctx, kind); err == nil {
			if auth.Source == managed.AuthSourceLocal && kind != "automation" {
				if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
					return err
				}
			}
			fmt.Printf("%s.\n", logoutAllMessage(kind))
			return nil
		} else {
			var httpErr *managed.HTTPError
			var invalidEnv *managed.InvalidEnvTokenError
			if errors.As(err, &invalidEnv) {
				return err
			}
			if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusUnauthorized {
				return err
			}
			if auth.Source == managed.AuthSourceLocal {
				if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
					return err
				}
				fmt.Fprintln(os.Stderr, "Stored login was invalid; re-authenticating to revoke server-side tokens.")
			} else {
				return err
			}
		}
	} else {
		fmt.Fprintln(os.Stderr, "No local login found; re-authenticating to revoke server-side tokens.")
	}
	verified, err := exchangeManagedBrowserLogin(ctx)
	if err != nil {
		return err
	}
	client := managed.New(managed.DefaultBaseURL, verified.Token)
	if err := client.LogoutAllKind(ctx, kind); err != nil {
		return err
	}
	if kind == "automation" {
		if err := client.Logout(ctx); err != nil {
			return err
		}
	}
	fmt.Printf("%s for %s.\n", logoutAllMessage(kind), verified.Email)
	return nil
}

func logoutAllMessage(kind string) string {
	switch kind {
	case "interactive":
		return "Revoked all interactive Dari Docs managed tokens"
	case "automation":
		return "Revoked all automation Dari Docs managed tokens"
	default:
		return "Revoked all Dari Docs managed tokens"
	}
}

func runAuthStatus(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth status", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, auth, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	me, err := client.Me(context.Background())
	if err != nil {
		return err
	}
	fmt.Printf("Authenticated to %s\n", managed.DefaultBaseURL)
	fmt.Printf("Email: %s\n", me.Email)
	fmt.Printf("Source: %s\n", authSourceLabel(auth.Source))
	if me.Token.ID != "" {
		name := me.Token.Name
		if name == "" {
			name = me.Token.ID
		}
		fmt.Printf("Token: %s (%s)\n", name, me.Token.Kind)
	}
	if len(me.Token.Scopes) > 0 {
		fmt.Printf("Scopes: %s\n", strings.Join(me.Token.Scopes, ", "))
	}
	return nil
}

func runAuthToken(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dari-docs auth token [create|list|revoke]")
	}
	switch args[0] {
	case "create":
		return runAuthTokenCreate(args[1:])
	case "list":
		return runAuthTokenList(args[1:])
	case "revoke":
		return runAuthTokenRevoke(args[1:])
	default:
		return fmt.Errorf("usage: dari-docs auth token [create|list|revoke]")
	}
}

func runAuthTokenCreate(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth token create", flag.ExitOnError)
	var name string
	var scopes repeated
	var expiresIn string
	fs.StringVar(&name, "name", "", "automation token name, for example github-actions")
	fs.Var(&scopes, "scope", "token scope; repeatable (default: managed:read, managed:check, and managed:optimize)")
	fs.StringVar(&expiresIn, "expires-in", "", "optional expiration such as 90d or 24h")
	if err := fs.Parse(args); err != nil {
		return err
	}
	expiresAt, err := parseExpiresIn(expiresIn)
	if err != nil {
		return err
	}
	client, _, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	tokenScopes := expandCSVList(scopes)
	if len(tokenScopes) == 0 {
		tokenScopes = []string{"managed:read", "managed:check", "managed:optimize"}
	}
	resp, err := client.CreateAuthToken(context.Background(), managed.TokenCreateRequest{
		Name:      name,
		Scopes:    tokenScopes,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		return err
	}
	displayName := resp.Name
	if displayName == "" {
		displayName = resp.ID
	}
	fmt.Printf("Created automation token %q.\n\n", displayName)
	fmt.Printf("%s=%s\n\n", managed.EnvTokenName, resp.Token)
	fmt.Println("Copy this value now. It will not be shown again.")
	return nil
}

func runAuthTokenList(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth token list", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, _, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	resp, err := client.ListAuthTokens(context.Background())
	if err != nil {
		return err
	}
	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "ID\tNAME\tKIND\tSCOPES\tLAST USED\tEXPIRES")
	for _, token := range resp.Tokens {
		name := token.Name
		if name == "" {
			name = "-"
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\n",
			token.ID,
			name,
			token.Kind,
			strings.Join(token.Scopes, ","),
			formatOptionalTime(token.LastUsedAt),
			formatOptionalTime(token.ExpiresAt),
		)
	}
	return tw.Flush()
}

func runAuthTokenRevoke(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth token revoke", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: dari-docs auth token revoke <token-id>")
	}
	client, _, err := managedClientWithAuth()
	if err != nil {
		return err
	}
	tokenID := fs.Arg(0)
	if err := client.RevokeAuthToken(context.Background(), tokenID); err != nil {
		return err
	}
	fmt.Printf("Revoked token %s\n", tokenID)
	return nil
}

func runBilling(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dari-docs billing [balance|checkout]")
	}
	switch args[0] {
	case "balance":
		fs := flag.NewFlagSet("dari-docs billing balance", flag.ExitOnError)
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		client, err := managedClientWithToken()
		if err != nil {
			return err
		}
		bal, err := client.Balance(context.Background())
		if err != nil {
			return err
		}
		fmt.Printf("%s balance: %s\n", bal.Email, formatCents(bal.BalanceCents))
		return nil
	case "checkout":
		fs := flag.NewFlagSet("dari-docs billing checkout", flag.ExitOnError)
		var amount string
		fs.StringVar(&amount, "amount", "", "credit purchase amount in dollars, for example 20 or 20.00")
		if err := fs.Parse(args[1:]); err != nil {
			return err
		}
		cents, err := parseDollarsToCents(amount)
		if err != nil {
			return err
		}
		client, err := managedClientWithToken()
		if err != nil {
			return err
		}
		checkout, err := client.CreateCheckout(context.Background(), cents)
		if err != nil {
			return err
		}
		if err := openBrowserURL(checkout.CheckoutURL); err != nil {
			fmt.Fprintf(os.Stderr, "Could not open browser automatically: %v\n", err)
		}
		fmt.Printf("Checkout URL: %s\n", checkout.CheckoutURL)
		return nil
	default:
		return fmt.Errorf("unknown billing command %q", args[0])
	}
}

func runAgents(args []string) error {
	if len(args) == 0 || args[0] != "deploy" {
		return fmt.Errorf("managed mode uses hosted Dari Docs agents automatically. For self-managed agents, run `dari-docs init --deploy`")
	}
	fs := flag.NewFlagSet("dari-docs agents deploy", flag.ExitOnError)
	var managedMode bool
	fs.BoolVar(&managedMode, "managed", false, "use hosted Dari Docs managed agents")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !managedMode {
		return fmt.Errorf("for self-managed agents, run `dari-docs init --deploy`")
	}
	fmt.Println("Managed mode uses hosted Dari Docs agents automatically.")
	return nil
}

func openBrowserURL(url string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", url).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
	default:
		return exec.Command("xdg-open", url).Start()
	}
}

func managedClientWithToken() (*managed.Client, error) {
	auth, err := loadManagedAuthToken()
	if err != nil {
		return nil, err
	}
	return managed.NewWithAuthToken(managed.DefaultBaseURL, auth), nil
}

func managedClientWithAuth() (*managed.Client, managed.AuthToken, error) {
	auth, err := loadManagedAuthToken()
	if err != nil {
		return nil, managed.AuthToken{}, err
	}
	return managed.NewWithAuthToken(managed.DefaultBaseURL, auth), auth, nil
}

func loadManagedToken() (string, error) {
	auth, err := loadManagedAuthToken()
	if err != nil {
		return "", err
	}
	return auth.Token, nil
}

func loadManagedAuthToken() (managed.AuthToken, error) {
	auth, err := managed.LoadAuthToken(managed.DefaultBaseURL)
	if err != nil {
		return managed.AuthToken{}, err
	}
	if auth.Token == "" {
		return managed.AuthToken{}, managedAuthRequiredError()
	}
	return auth, nil
}

func managedAuthRequiredError() error {
	return fmt.Errorf("not logged in to managed service\n\nFor local use:\n  dari-docs auth login\n\nFor CI:\n  dari-docs auth token create --name github-actions\n  Set %s in your CI secret store", managed.EnvTokenName)
}

func authSourceLabel(source string) string {
	if source == managed.AuthSourceEnv {
		return managed.EnvTokenName
	}
	return "local credentials"
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func parseExpiresIn(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var d time.Duration
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || days <= 0 {
			return nil, fmt.Errorf("--expires-in must be a positive duration like 90d or 24h")
		}
		d = time.Duration(days) * 24 * time.Hour
	} else {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("--expires-in must be a positive duration like 90d or 24h")
		}
		d = parsed
	}
	t := time.Now().UTC().Add(d)
	return &t, nil
}

func parseDollarsToCents(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("--amount is required")
	}
	whole, frac, ok := strings.Cut(v, ".")
	if !ok {
		n, err := strconv.ParseInt(whole, 10, 64)
		if err != nil {
			return 0, err
		}
		return n * 100, nil
	}
	n, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("--amount can include at most two decimal places")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return n*100 + cents, nil
}

func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s$%d.%02d", sign, cents/100, cents%100)
}

func versionLine() string {
	return "dari-docs " + version
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("dari-docs init", flag.ExitOnError)
	var deploy bool
	var apiKeyEnv, apiKey, llmAPIKeySecret, anthropicAPIKeySecret, openAIAPIKeySecret, agentsDir string
	fs.BoolVar(&deploy, "deploy", false, "deploy bundled agents into the current Dari org")
	fs.StringVar(&apiKeyEnv, "api-key-env", "DARI_API_KEY", "env var containing Dari API key for deploy")
	fs.StringVar(&apiKey, "api-key", "", "Dari API key for deploy (prefer --api-key-env)")
	fs.StringVar(&llmAPIKeySecret, "llm-api-key-secret", "", "optional stored Dari credential name for BYOK LLM at agent publish time; only valid when all LLM options use one provider")
	fs.StringVar(&anthropicAPIKeySecret, "anthropic-api-key-secret", "", "optional stored Dari credential name for Anthropic BYOK LLM at agent publish time")
	fs.StringVar(&openAIAPIKeySecret, "openai-api-key-secret", "", "optional stored Dari credential name for OpenAI BYOK LLM at agent publish time")
	fs.StringVar(&agentsDir, "agents-dir", "", "where to extract agent templates (default: <repo>/.dari-docs/agents)")
	repoArg := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repoArg = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		repoArg = fs.Arg(0)
	}
	absRepo, err := filepath.Abs(repoArg)
	if err != nil {
		return err
	}
	if agentsDir == "" {
		agentsDir = filepath.Join(absRepo, ".dari-docs", "agents")
	}
	if err := agenttemplates.Extract(agentsDir); err != nil {
		return err
	}
	fmt.Printf("Extracted bundled agents to %s\n", agentsDir)

	providerSecrets := map[string]string{}
	if anthropicAPIKeySecret != "" {
		providerSecrets["anthropic"] = anthropicAPIKeySecret
	}
	if openAIAPIKeySecret != "" {
		providerSecrets["openai"] = openAIAPIKeySecret
	}
	if llmAPIKeySecret != "" && len(providerSecrets) > 0 {
		return fmt.Errorf("--llm-api-key-secret cannot be combined with provider-specific LLM key secret flags")
	}

	cfg := appconfig.Config{AgentsDir: agentsDir, LLMMode: "platform-managed", LLMAPIKeySecret: llmAPIKeySecret}
	if llmAPIKeySecret != "" {
		cfg.LLMMode = "byok-publish-time"
		if err := setLLMAPIKeySecret(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), llmAPIKeySecret); err != nil {
			return err
		}
		if err := setLLMAPIKeySecret(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), llmAPIKeySecret); err != nil {
			return err
		}
	}
	if len(providerSecrets) > 0 {
		cfg.LLMMode = "byok-publish-time"
		cfg.LLMAPIKeySecrets = providerSecrets
		if err := setLLMAPIKeySecretsByProvider(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), providerSecrets); err != nil {
			return err
		}
		if err := setLLMAPIKeySecretsByProvider(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), providerSecrets); err != nil {
			return err
		}
	}
	if deploy {
		if apiKey == "" && apiKeyEnv != "" {
			apiKey = os.Getenv(apiKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("missing Dari API key for deploy; set %s or pass --api-key", apiKeyEnv)
		}
		env := append(os.Environ(), "DARI_API_URL=https://api.dari.dev", "DARI_API_KEY="+apiKey)
		ensureCredential(env, "DARI_DOCS_RUNTIME_SECRETS_JSON", "{}")
		testerID, err := deployAgent(env, filepath.Join(agentsDir, "docs-user-tester-agent"))
		if err != nil {
			return err
		}
		editorID, err := deployAgent(env, filepath.Join(agentsDir, "docs-editor-agent"))
		if err != nil {
			return err
		}
		cfg.TesterAgentID = testerID
		cfg.EditorAgentID = editorID
		fmt.Printf("Deployed tester agent: %s\n", testerID)
		fmt.Printf("Deployed editor agent: %s\n", editorID)
	}
	if err := appconfig.Save(absRepo, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", appconfig.Path(absRepo))
	if !deploy {
		fmt.Println("Run `dari-docs init --deploy` to deploy these agents into your Dari org.")
	}
	return nil
}

func ensureCredential(env []string, name, value string) {
	cmd := exec.Command("dari", "credentials", "add", name, value)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create credential %s (it may already exist): %s\n", name, strings.TrimSpace(out.String()))
	}
}

func deployAgent(env []string, dir string) (string, error) {
	cmd := exec.Command("dari", "deploy", "--quiet", ".")
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("deploy %s: %w\n%s", dir, err, stderr.String()+stdout.String())
	}
	var resp struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil || resp.AgentID == "" {
		return "", fmt.Errorf("deploy %s: could not parse agent_id from output: %s", dir, stdout.String())
	}
	return resp.AgentID, nil
}

type llmModelEntry struct {
	node     *yaml.Node
	provider string
}

type llmManifest struct {
	path    string
	doc     yaml.Node
	entries []llmModelEntry
}

func setLLMAPIKeySecret(path, secret string) error {
	manifest, err := loadLLMManifest(path)
	if err != nil {
		return err
	}
	providers := map[string]bool{}
	for _, entry := range manifest.entries {
		providers[entry.provider] = true
	}
	if len(providers) > 1 {
		return fmt.Errorf("--llm-api-key-secret cannot be applied to multiple LLM providers (%s); use --anthropic-api-key-secret and/or --openai-api-key-secret", strings.Join(providerNames(providers), ", "))
	}
	for _, entry := range manifest.entries {
		yamlSetMappingScalar(entry.node, "api_key_secret", secret)
	}
	return manifest.write()
}

func setLLMAPIKeySecretsByProvider(path string, providerSecrets map[string]string) error {
	providerSecrets = normalizeProviderSecrets(providerSecrets)
	if len(providerSecrets) == 0 {
		return nil
	}

	manifest, err := loadLLMManifest(path)
	if err != nil {
		return err
	}
	matched := map[string]bool{}
	for _, entry := range manifest.entries {
		secret, ok := providerSecrets[entry.provider]
		if !ok {
			continue
		}
		matched[entry.provider] = true
		yamlSetMappingScalar(entry.node, "api_key_secret", secret)
	}
	for provider := range providerSecrets {
		if !matched[provider] {
			return fmt.Errorf("could not find %s llm option in %s", provider, path)
		}
	}
	return manifest.write()
}

func normalizeProviderSecrets(in map[string]string) map[string]string {
	out := map[string]string{}
	for provider, secret := range in {
		provider = normalizeProvider(provider)
		secret = strings.TrimSpace(secret)
		if provider != "" && secret != "" {
			out[provider] = secret
		}
	}
	return out
}

func loadLLMManifest(path string) (*llmManifest, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	manifest := &llmManifest{path: path}
	if err := yaml.Unmarshal(b, &manifest.doc); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	entries, err := collectLLMModelEntries(path, &manifest.doc)
	if err != nil {
		return nil, err
	}
	manifest.entries = entries
	return manifest, nil
}

func (m *llmManifest) write() error {
	var out bytes.Buffer
	enc := yaml.NewEncoder(&out)
	enc.SetIndent(2)
	if err := enc.Encode(&m.doc); err != nil {
		_ = enc.Close()
		return fmt.Errorf("encode %s: %w", m.path, err)
	}
	if err := enc.Close(); err != nil {
		return fmt.Errorf("encode %s: %w", m.path, err)
	}
	return os.WriteFile(m.path, out.Bytes(), 0o644)
}

func collectLLMModelEntries(path string, doc *yaml.Node) ([]llmModelEntry, error) {
	root := yamlDocumentRoot(doc)
	llm := yamlMappingValue(root, "llm")
	if llm == nil {
		return nil, fmt.Errorf("could not find llm block in %s", path)
	}

	var entries []llmModelEntry
	if options := yamlMappingValue(llm, "options"); options != nil {
		if options.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("llm.options in %s must be a mapping", path)
		}
		for i := 1; i < len(options.Content); i += 2 {
			option := options.Content[i]
			model := yamlMappingValue(option, "model")
			if model == nil {
				continue
			}
			entries = append(entries, llmModelEntry{node: option, provider: providerForLLMNode(option, model.Value)})
		}
	} else if model := yamlMappingValue(llm, "model"); model != nil {
		entries = append(entries, llmModelEntry{node: llm, provider: providerForLLMNode(llm, model.Value)})
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("could not find llm.model in %s", path)
	}
	return entries, nil
}

func providerForLLMNode(node *yaml.Node, model string) string {
	if provider := yamlMappingValue(node, "provider"); provider != nil {
		return normalizeProvider(provider.Value)
	}
	return inferProviderFromModel(model)
}

func providerNames(providers map[string]bool) []string {
	names := make([]string, 0, len(providers))
	for provider := range providers {
		if provider == "" {
			provider = "unspecified"
		}
		names = append(names, provider)
	}
	sort.Strings(names)
	return names
}

func yamlDocumentRoot(doc *yaml.Node) *yaml.Node {
	if doc.Kind == yaml.DocumentNode && len(doc.Content) > 0 {
		return doc.Content[0]
	}
	return doc
}

func yamlMappingValue(node *yaml.Node, key string) *yaml.Node {
	if node == nil || node.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			return node.Content[i+1]
		}
	}
	return nil
}

func yamlSetMappingScalar(node *yaml.Node, key, value string) {
	if node == nil || node.Kind != yaml.MappingNode {
		return
	}
	for i := 0; i+1 < len(node.Content); i += 2 {
		if node.Content[i].Value == key {
			node.Content[i+1].Kind = yaml.ScalarNode
			node.Content[i+1].Tag = "!!str"
			node.Content[i+1].Value = value
			return
		}
	}
	node.Content = append(node.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: value},
	)
}

func normalizeProvider(provider string) string {
	return strings.ToLower(strings.TrimSpace(provider))
}

func inferProviderFromModel(model string) string {
	model = strings.ToLower(strings.TrimSpace(model))
	if strings.HasPrefix(model, "openai/") {
		return "openai"
	}
	if strings.HasPrefix(model, "anthropic/") {
		return "anthropic"
	}
	return ""
}

func expandCSVList(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range values {
		for _, part := range strings.Split(raw, ",") {
			v := strings.TrimSpace(part)
			if v == "" || seen[v] {
				continue
			}
			seen[v] = true
			out = append(out, v)
		}
	}
	return out
}

func readTasksFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tasks []string
	var cur []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			if len(cur) > 0 {
				tasks = append(tasks, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		tasks = append(tasks, strings.Join(cur, "\n"))
	}
	return tasks, s.Err()
}

func usage() {
	fmt.Print(`dari-docs runs lightweight user-test sessions, feeds the results into a hosted editor, and pulls updated docs back to your repo.

Usage:
  dari-docs --version
  dari-docs auth login
  dari-docs auth status
  dari-docs auth token create --name github-actions
  dari-docs auth token list
  dari-docs auth token revoke <token-id>
  dari-docs auth logout [--all] [--interactive-only|--automation-only]
  dari-docs init [repo]
  dari-docs billing balance
  dari-docs optimize [repo] --task "Implement auth" [--task "Set up webhooks"] [flags]
  dari-docs check [repo] --task "Implement auth" [flags]

Managed setup:
  dari-docs auth login

Self-managed setup:
  export DARI_API_KEY=...
  dari-docs init --deploy

Important flags:
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key from env var; repeatable
  --managed                   use the managed dari-docs service instead of your Dari org
  --bundle-include GLOB       include extra repo-relative docs bundle paths; repeatable
  --bundle-exclude GLOB       exclude repo-relative docs bundle paths; repeatable
  --apply                     copy downloaded updated docs back into repo
  --api-base-url URL          Dari API base URL; self-managed only
  --parallel N                tester sessions per batch; self-managed only
  --llm ID                    select an LLM option for all sessions
  --feedback-llm ID           select tester LLM option(s); repeat or comma-separate
  --editor-llm ID             select a manifest LLM option for the editor session
  --anthropic-api-key-secret  stored Dari credential name for Anthropic BYOK deploys
  --openai-api-key-secret     stored Dari credential name for OpenAI BYOK deploys

Outputs:
  .dari-docs/config.json
  .dari-docs/agents/
  .dari-docs/input-docs-bundle.tar.gz
  .dari-docs/runs/feedback-*.md
  .dari-docs/aggregate-feedback.md
  .dari-docs/updated-docs-workspace.zip
  .dari-docs/updated/
`)
}
