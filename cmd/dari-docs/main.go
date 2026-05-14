package main

import (
	"bufio"
	"bytes"
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/agentbundle"
	"github.com/mupt-ai/dari-docs/internal/agenttemplates"
	"github.com/mupt-ai/dari-docs/internal/bundle"
	appconfig "github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/managed"
	"github.com/mupt-ai/dari-docs/internal/platformauth"
	"github.com/mupt-ai/dari-docs/internal/runner"
	"github.com/mupt-ai/dari-docs/internal/workspace"
)

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func defaultFeedbackLLMIDs() []string {
	return runner.DefaultFeedbackLLMIDs()
}

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
		case "init", "optimize", "check", "auth", "billing", "agents", "help", "-h", "--help":
			cmd = args[0]
			args = args[1:]
		}
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		usage()
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
	fs.Var(&feedbackLLMIDs, "feedback-llm", "manifest LLM option ID for feedback/tester sessions; repeat or comma-separate (default: all bundled tester LLMs; overrides --llm)")
	fs.StringVar(&editorLLMID, "editor-llm", "", "manifest LLM option ID for the editor session (overrides --llm)")
	fs.StringVar(&outDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	fs.IntVar(&parallel, "parallel", 4, "number of feedback sessions to run concurrently")
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
	feedbackLLMExplicit := len(feedbackLLMList) > 0
	if editorLLMID == "" {
		editorLLMID = llmID
	}
	if managedMode {
		if apiKey != "" || apiBaseURL != "" || feedbackAgent != "" || editorAgent != "" || llmID != "" || feedbackLLMExplicit || editorLLMID != "" {
			return fmt.Errorf("--managed cannot be combined with --api-key, --api-base-url, --feedback-agent, --editor-agent, or LLM selection flags")
		}
		c, ok, err := appconfig.Load(absRepo)
		if err != nil {
			return err
		}
		if !ok || c.ManagedAgentSetID == "" {
			return fmt.Errorf("missing managed agent set; run `dari-docs agents deploy --managed`")
		}
		return runManagedCheckOrOptimize(context.Background(), managedRunConfig{
			Command: cmd, RepoRoot: absRepo, OutDir: outDir,
			AgentSetID: c.ManagedAgentSetID, Tasks: allTasks, Apply: apply, LiveVerify: liveVerify, RuntimeSecrets: secrets, Timeout: time.Duration(timeoutMinutes) * time.Minute,
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
	AgentSetID     string
	Tasks          []string
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
	token, err := managed.LoadToken(managed.DefaultBaseURL)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("not logged in to managed service; run `dari-docs auth login`")
	}
	client := managed.New(managed.DefaultBaseURL, token)
	runCfg, err := client.RunConfig(ctx)
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
	reserve := managedRunReserveCents(cfg.Command, len(cfg.Tasks), runCfg)
	fmt.Fprintln(os.Stderr, "\nManaged run estimate:")
	fmt.Fprintf(os.Stderr, "  Balance: %s\n", formatCents(bal.BalanceCents))
	fmt.Fprintf(os.Stderr, "  Sessions: %s\n", managedSessionSummary(cfg.Command, len(cfg.Tasks)))
	fmt.Fprintf(os.Stderr, "  Reserved before start: %s\n", formatCents(reserve))
	fmt.Fprintln(os.Stderr, "  Final charge reconciles to actual session cost after completion.")

	runtimeSecretJSON := ""
	if cfg.LiveVerify && len(cfg.RuntimeSecrets) > 0 {
		b, _ := json.Marshal(cfg.RuntimeSecrets)
		runtimeSecretJSON = string(b)
	}
	created, err := client.CreateRun(ctx, cfg.Command, cfg.Tasks, bundlePath, managed.CreateRunOptions{
		AgentSetID:         cfg.AgentSetID,
		LiveVerify:         cfg.LiveVerify,
		RuntimeSecretsJSON: runtimeSecretJSON,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Managed run: %s\n", created.RunID)
	fmt.Fprintf(os.Stderr, "Reserved: %s\n", formatCents(reserve))
	totalSessions := len(cfg.Tasks)
	if cfg.Command != "check" {
		totalSessions++
	}
	deadline := time.Now().Add(cfg.Timeout * time.Duration(totalSessions))
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
		case <-time.After(5 * time.Second):
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

func managedRunReserveCents(command string, taskCount int, cfg managed.RunConfig) int64 {
	reserve := int64(taskCount) * cfg.TesterSessionReserveCents
	if command != "check" {
		reserve += cfg.EditorSessionReserveCents
	}
	return reserve
}

func managedSessionSummary(command string, taskCount int) string {
	tester := fmt.Sprintf("%d tester", taskCount)
	if taskCount != 1 {
		tester += " sessions"
	} else {
		tester += " session"
	}
	if command == "check" {
		return tester
	}
	return tester + " + 1 editor session"
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
		return fmt.Errorf("usage: dari-docs auth [login|logout]")
	}
	switch args[0] {
	case "login":
		return runAuthLogin(args[1:])
	case "logout":
		return runAuthLogout(args[1:])
	default:
		return fmt.Errorf("usage: dari-docs auth [login|logout]")
	}
}

func runAuthLogin(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth login", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	ctx := context.Background()
	authConfig, err := platformauth.FetchConfig(ctx, "https://api.dari.dev")
	if err != nil {
		return err
	}
	session, err := platformauth.LoginWithBrowser(ctx, authConfig, os.Stdin, os.Stderr)
	if err != nil {
		return err
	}
	client := managed.New(managed.DefaultBaseURL, "")
	verified, err := client.ExchangeDariToken(ctx, session.AccessToken)
	if err != nil {
		return err
	}
	if err := managed.SaveToken(managed.DefaultBaseURL, verified.Token); err != nil {
		return err
	}
	fmt.Printf("Logged in to %s as %s\n", managed.DefaultBaseURL, verified.Email)
	return nil
}

func runAuthLogout(args []string) error {
	fs := flag.NewFlagSet("dari-docs auth logout", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	if err := client.Logout(context.Background()); err != nil {
		return err
	}
	if err := managed.DeleteToken(managed.DefaultBaseURL); err != nil {
		return err
	}
	fmt.Printf("Logged out of %s\n", managed.DefaultBaseURL)
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
		return fmt.Errorf("usage: dari-docs agents deploy --managed")
	}
	fs := flag.NewFlagSet("dari-docs agents deploy", flag.ExitOnError)
	var managedMode bool
	var forceNew bool
	var resumeOnly bool
	fs.BoolVar(&managedMode, "managed", false, "deploy local dari-docs agents through the managed service")
	fs.BoolVar(&forceNew, "force-new", false, "queue a new managed deploy even if a matching deploy is pending")
	fs.BoolVar(&resumeOnly, "resume", false, "resume the pending managed deploy for this repo")
	if err := fs.Parse(args[1:]); err != nil {
		return err
	}
	if !managedMode {
		return fmt.Errorf("usage: dari-docs agents deploy --managed")
	}
	if forceNew && resumeOnly {
		return fmt.Errorf("--force-new cannot be combined with --resume")
	}
	repo := "."
	if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	cfg, ok, err := appconfig.Load(absRepo)
	if err != nil {
		return err
	}
	if !ok || cfg.AgentsDir == "" {
		return fmt.Errorf("missing local agents; run `dari-docs init` first")
	}
	testerDir := filepath.Join(cfg.AgentsDir, "docs-user-tester-agent")
	editorDir := filepath.Join(cfg.AgentsDir, "docs-editor-agent")
	if _, err := os.Stat(filepath.Join(testerDir, "dari.yml")); err != nil {
		return fmt.Errorf("missing tester agent project at %s", filepath.Join(testerDir, "dari.yml"))
	}
	if _, err := os.Stat(filepath.Join(editorDir, "dari.yml")); err != nil {
		return fmt.Errorf("missing editor agent project at %s", filepath.Join(editorDir, "dari.yml"))
	}
	testerBundle, err := agentbundle.Build(testerDir)
	if err != nil {
		return fmt.Errorf("bundle tester agent: %w", err)
	}
	editorBundle, err := agentbundle.Build(editorDir)
	if err != nil {
		return fmt.Errorf("bundle editor agent: %w", err)
	}
	if err := agentbundle.ValidateManagedBundle(testerBundle.Content, "tester agent"); err != nil {
		return err
	}
	if err := agentbundle.ValidateManagedBundle(editorBundle.Content, "editor agent"); err != nil {
		return err
	}
	token, err := managed.LoadToken(managed.DefaultBaseURL)
	if err != nil {
		return err
	}
	if token == "" {
		return fmt.Errorf("not logged in to managed service; run `dari-docs auth login`")
	}
	tokenHash := managedTokenHash(token)
	state, _ := loadLocalState(absRepo)
	var retryPending *pendingManagedDeploy
	if pending := state.PendingManagedDeploy; pending != nil && !forceNew {
		switch {
		case pending.TokenHash != tokenHash:
			fmt.Fprintf(os.Stderr, "Ignoring pending managed deploy for a different login.\n")
			clearPendingManagedDeploy(absRepo, pending.DeployRequestID, pending.DeployID)
		case pending.TesterSHA256 == testerBundle.SHA256 && pending.EditorSHA256 == editorBundle.SHA256:
			if pending.DeployID == "" {
				retryPending = pending
				break
			}
			client := managed.New(managed.DefaultBaseURL, token)
			return waitForManagedAgentDeploy(context.Background(), absRepo, cfg, client, *pending)
		case resumeOnly:
			if pending.DeployID == "" {
				return fmt.Errorf("pending managed deploy did not reach the service and agent files changed; rerun with --force-new")
			}
			client := managed.New(managed.DefaultBaseURL, token)
			return waitForManagedAgentDeploy(context.Background(), absRepo, cfg, client, *pending)
		default:
			return fmt.Errorf("a previous managed deploy is pending for this repo but the agent files changed; rerun with --resume to wait for it or --force-new to queue a new deploy")
		}
	}
	if resumeOnly {
		return fmt.Errorf("no pending managed deploy found for this repo")
	}
	tmpDir, err := os.MkdirTemp("", "dari-docs-agent-bundles-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)
	testerPath := filepath.Join(tmpDir, "docs-user-tester-agent.tar.gz")
	editorPath := filepath.Join(tmpDir, "docs-editor-agent.tar.gz")
	if err := os.WriteFile(testerPath, testerBundle.Content, 0o600); err != nil {
		return err
	}
	if err := os.WriteFile(editorPath, editorBundle.Content, 0o600); err != nil {
		return err
	}
	client := managed.New(managed.DefaultBaseURL, token)
	pending := pendingManagedDeploy{}
	if retryPending != nil {
		pending = *retryPending
	} else {
		pending = pendingManagedDeploy{
			DeployRequestID: newManagedDeployRequestID(),
			AgentSetID:      cfg.ManagedAgentSetID,
			TesterSHA256:    testerBundle.SHA256,
			EditorSHA256:    editorBundle.SHA256,
			TokenHash:       tokenHash,
			CreatedAt:       time.Now().UTC(),
		}
	}
	if err := savePendingManagedDeploy(absRepo, pending); err != nil {
		return err
	}
	resp, err := client.CreateAgentSetDeploy(context.Background(), managed.CreateAgentSetOptions{
		ExistingAgentSetID: cfg.ManagedAgentSetID,
		DeployRequestID:    pending.DeployRequestID,
		TesterBundlePath:   testerPath,
		EditorBundlePath:   editorPath,
	})
	if err != nil {
		return err
	}
	pending.DeployID = resp.DeployID
	pending.AgentSetID = resp.ID
	if err := savePendingManagedDeploy(absRepo, pending); err != nil {
		return err
	}
	return waitForManagedAgentDeploy(context.Background(), absRepo, cfg, client, pending)
}

type localState struct {
	PendingManagedDeploy *pendingManagedDeploy `json:"pending_managed_deploy,omitempty"`
}

type pendingManagedDeploy struct {
	DeployID        string    `json:"deploy_id,omitempty"`
	DeployRequestID string    `json:"deploy_request_id"`
	AgentSetID      string    `json:"agent_set_id,omitempty"`
	TesterSHA256    string    `json:"tester_sha256"`
	EditorSHA256    string    `json:"editor_sha256"`
	TokenHash       string    `json:"token_hash"`
	CreatedAt       time.Time `json:"created_at"`
}

func waitForManagedAgentDeploy(ctx context.Context, repoRoot string, cfg appconfig.Config, client *managed.Client, pending pendingManagedDeploy) error {
	if pending.DeployRequestID == "" {
		return fmt.Errorf("pending managed deploy is missing deploy_request_id")
	}
	if pending.DeployID == "" {
		return fmt.Errorf("pending managed deploy did not reach the service; rerun with --force-new")
	}
	fmt.Fprintf(os.Stderr, "Managed agent deploy: %s\n", pending.DeployID)
	var resp managed.AgentSetResponse
	for {
		var err error
		resp, err = client.GetAgentSetDeploy(ctx, pending.DeployID)
		if err != nil {
			var httpErr *managed.HTTPError
			if errors.As(err, &httpErr) && (httpErr.StatusCode == http.StatusUnauthorized || httpErr.StatusCode == http.StatusForbidden || httpErr.StatusCode == http.StatusNotFound) {
				clearPendingManagedDeploy(repoRoot, pending.DeployRequestID, pending.DeployID)
			}
			return err
		}
		if resp.Status == "completed" || resp.Status == "failed" {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(3 * time.Second):
		}
	}
	if resp.Status == "failed" {
		clearPendingManagedDeploy(repoRoot, pending.DeployRequestID, pending.DeployID)
		return fmt.Errorf("managed agent deploy %s failed: %s", pending.DeployID, resp.Error)
	}
	if !resp.Applied {
		clearPendingManagedDeploy(repoRoot, pending.DeployRequestID, pending.DeployID)
		fmt.Printf("Managed agent deploy %s completed but was superseded by a newer deploy.\n", pending.DeployID)
		return nil
	}
	if !pendingManagedDeployMatches(repoRoot, pending.DeployRequestID, pending.DeployID) {
		fmt.Printf("Managed agent deploy %s completed, but a newer local deploy is pending; leaving config unchanged.\n", pending.DeployID)
		return nil
	}
	cfg.ManagedAgentSetID = resp.ID
	if err := appconfig.Save(repoRoot, cfg); err != nil {
		return err
	}
	clearPendingManagedDeploy(repoRoot, pending.DeployRequestID, pending.DeployID)
	fmt.Printf("Managed agent set: %s\n", resp.ID)
	fmt.Printf("Tester agent: %s\n", resp.TesterAgentID)
	fmt.Printf("Tester version: %s\n", resp.TesterVersionID)
	fmt.Printf("Editor agent: %s\n", resp.EditorAgentID)
	fmt.Printf("Editor version: %s\n", resp.EditorVersionID)
	return nil
}

func statePath(repoRoot string) string {
	return filepath.Join(repoRoot, ".dari-docs", "state.json")
}

func loadLocalState(repoRoot string) (localState, error) {
	var st localState
	b, err := os.ReadFile(statePath(repoRoot))
	if os.IsNotExist(err) {
		return st, nil
	}
	if err != nil {
		return st, err
	}
	if len(bytes.TrimSpace(b)) == 0 {
		return st, nil
	}
	err = json.Unmarshal(b, &st)
	return st, err
}

func saveLocalState(repoRoot string, st localState) error {
	if err := os.MkdirAll(filepath.Join(repoRoot, ".dari-docs"), 0o755); err != nil {
		return err
	}
	if st.PendingManagedDeploy == nil {
		if err := os.Remove(statePath(repoRoot)); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	b, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(statePath(repoRoot), append(b, '\n'), 0o600)
}

func savePendingManagedDeploy(repoRoot string, pending pendingManagedDeploy) error {
	st, _ := loadLocalState(repoRoot)
	st.PendingManagedDeploy = &pending
	return saveLocalState(repoRoot, st)
}

func clearPendingManagedDeploy(repoRoot, deployRequestID, deployID string) {
	st, err := loadLocalState(repoRoot)
	if err != nil || st.PendingManagedDeploy == nil {
		return
	}
	pending := st.PendingManagedDeploy
	if pending.DeployRequestID != deployRequestID {
		return
	}
	if deployID != "" && pending.DeployID != "" && pending.DeployID != deployID {
		return
	}
	st.PendingManagedDeploy = nil
	_ = saveLocalState(repoRoot, st)
}

func pendingManagedDeployMatches(repoRoot, deployRequestID, deployID string) bool {
	st, err := loadLocalState(repoRoot)
	if err != nil || st.PendingManagedDeploy == nil {
		return false
	}
	pending := st.PendingManagedDeploy
	if pending.DeployRequestID != deployRequestID {
		return false
	}
	return deployID == "" || pending.DeployID == deployID
}

func newManagedDeployRequestID() string {
	b := make([]byte, 18)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return "mdr_" + base64.RawURLEncoding.EncodeToString(b)
}

func managedTokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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
	token, err := managed.LoadToken(managed.DefaultBaseURL)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, fmt.Errorf("not logged in to managed service; run `dari-docs auth login`")
	}
	return managed.New(managed.DefaultBaseURL, token), nil
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

func runInit(args []string) error {
	fs := flag.NewFlagSet("dari-docs init", flag.ExitOnError)
	var deploy bool
	var apiKeyEnv, apiKey, llmAPIKeySecret, agentsDir string
	fs.BoolVar(&deploy, "deploy", false, "deploy bundled agents into the current Dari org")
	fs.StringVar(&apiKeyEnv, "api-key-env", "DARI_API_KEY", "env var containing Dari API key for deploy")
	fs.StringVar(&apiKey, "api-key", "", "Dari API key for deploy (prefer --api-key-env)")
	fs.StringVar(&llmAPIKeySecret, "llm-api-key-secret", "", "optional stored Dari credential name for BYOK LLM at agent publish time; omit to use platform-managed LLM")
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

func setLLMAPIKeySecret(path, secret string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(b)
	lines := strings.SplitAfter(text, "\n")
	llmStart := -1
	for i := range lines {
		if strings.TrimSpace(lines[i]) == "llm:" {
			llmStart = i
			break
		}
	}
	if llmStart == -1 {
		return fmt.Errorf("could not find llm block in %s", path)
	}
	insertions := map[int]string{}
	for i := llmStart + 1; i < len(lines); i++ {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && !strings.HasPrefix(line, "\t") {
			break
		}
		if !strings.HasPrefix(trimmed, "model:") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		alreadyHasSecret := false
		for j := i + 1; j < len(lines); j++ {
			next := lines[j]
			nextTrimmed := strings.TrimSpace(next)
			if nextTrimmed == "" {
				continue
			}
			if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
				break
			}
			nextIndent := next[:len(next)-len(strings.TrimLeft(next, " \t"))]
			if len(nextIndent) < len(indent) {
				break
			}
			if len(nextIndent) == len(indent) && strings.HasSuffix(nextTrimmed, ":") {
				break
			}
			if len(nextIndent) == len(indent) && strings.HasPrefix(nextTrimmed, "api_key_secret:") {
				alreadyHasSecret = true
				break
			}
		}
		if !alreadyHasSecret {
			insertions[i+1] = indent + "api_key_secret: " + secret + "\n"
		}
	}
	if len(insertions) == 0 {
		if strings.Contains(text, "api_key_secret:") {
			return nil
		}
		return fmt.Errorf("could not find llm.model in %s", path)
	}
	var out strings.Builder
	for i, line := range lines {
		out.WriteString(line)
		if insert, ok := insertions[i+1]; ok {
			out.WriteString(insert)
		}
	}
	return os.WriteFile(path, []byte(out.String()), 0o644)
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
  dari-docs auth login
  dari-docs init [repo]
  dari-docs agents deploy --managed [--resume|--force-new]
  dari-docs billing balance
  dari-docs optimize [repo] --task "Implement auth" [--task "Set up webhooks"] [flags]
  dari-docs check [repo] --task "Implement auth" [flags]

Managed setup:
  dari-docs auth login
  dari-docs init
  dari-docs agents deploy --managed

Self-managed setup:
  export DARI_API_KEY=...
  dari-docs init --deploy

Important flags:
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key from env var; repeatable
  --managed                   use the managed dari-docs service instead of your Dari org
  --resume                    resume a pending managed agent deploy
  --force-new                 queue a new managed agent deploy
  --bundle-include GLOB       include extra repo-relative docs bundle paths; repeatable
  --bundle-exclude GLOB       exclude repo-relative docs bundle paths; repeatable
  --apply                     copy downloaded updated docs back into repo
  --api-base-url URL          Dari API base URL; self-managed only
  --parallel N                tester sessions in parallel; self-managed only
  --llm ID                    select a manifest LLM option for all self-managed sessions
  --feedback-llm ID           select tester LLM option(s); default is all bundled tester LLMs
  --editor-llm ID             select a manifest LLM option for the editor session

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
