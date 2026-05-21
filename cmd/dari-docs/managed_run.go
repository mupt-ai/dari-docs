package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/managed"
	"github.com/mupt-ai/dari-docs/internal/runner"
	"github.com/mupt-ai/dari-docs/internal/workspace"
)

type managedRunConfig struct {
	Command        string
	RepoRoot       string
	OutDir         string
	Tasks          []string
	FeedbackLLMIDs []string
	EditorLLMID    string
	LiveVerify     bool
	RuntimeSecrets map[string]string
	PublicDocURLs  []string
	PublicDocsOnly bool
	Apply          bool
	Wait           bool
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
	bundlePath := ""
	submittedBundlePath := ""
	if !cfg.PublicDocsOnly {
		bundlePath = filepath.Join(cfg.OutDir, "input-docs-bundle.tar.gz")
		cfg.BundleOptions.MaxFileBytes = runCfg.BundleMaxFileBytes
		b, err := bundle.CreateWithOptions(cfg.RepoRoot, bundlePath, cfg.BundleOptions)
		if err != nil {
			return err
		}
		bundle.WriteSummary(os.Stderr, b)
		submittedBundlePath = bundlePath
	}

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

	created, err := client.CreateRun(ctx, cfg.Command, cfg.Tasks, submittedBundlePath, managed.CreateRunOptions{
		LiveVerify:     cfg.LiveVerify,
		RuntimeSecrets: cfg.RuntimeSecrets,
		PublicDocURLs:  cfg.PublicDocURLs,
		FeedbackLLMIDs: feedbackLLMIDs,
		EditorLLMID:    editorLLMID,
	})
	if err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Managed run: %s\n", created.RunID)
	fmt.Fprintf(os.Stderr, "Reserved: %s\n", formatCents(reserve))
	if !cfg.Wait {
		printManagedRunSubmitted(created.RunID, created.Status, cfg.Command)
		return nil
	}
	status, err := waitForManagedRun(ctx, client, created.RunID, cfg.Timeout)
	if err != nil {
		return err
	}
	if err := writeManagedFeedback(cfg.OutDir, status); err != nil {
		return err
	}
	if status.Status == "failed" {
		return fmt.Errorf("managed run %s failed: %s", status.ID, status.Error)
	}
	fmt.Println("\nDone.")
	if bundlePath != "" {
		fmt.Printf("Bundle: %s\n", bundlePath)
	}
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
		updatedDir, err := downloadManagedUpdatedDocs(ctx, client, status.ID, cfg.OutDir)
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

func printManagedRunSubmitted(runID string, status string, command string) {
	fmt.Println("\nSubmitted managed run.")
	fmt.Printf("Managed run: %s\n", runID)
	if status != "" {
		fmt.Printf("Status: %s\n", status)
	}
	fmt.Printf("Wait: dari-docs runs wait %s\n", runID)
	fmt.Printf("Download feedback after completion: dari-docs runs download %s\n", runID)
	if command != "check" {
		fmt.Printf("Apply updated docs after completion: dari-docs runs apply %s\n", runID)
	}
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

func waitForManagedRun(ctx context.Context, client *managed.Client, runID string, timeout time.Duration) (managed.RunStatus, error) {
	deadline := time.Now().Add(timeout)
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		status, err := client.GetRun(ctx, runID)
		if err != nil {
			return managed.RunStatus{}, err
		}
		if isTerminalManagedRunStatus(status.Status) {
			return status, nil
		}
		if time.Now().After(deadline) {
			return managed.RunStatus{}, fmt.Errorf("timeout waiting for managed run %s status=%q", runID, status.Status)
		}
		select {
		case <-ctx.Done():
			return managed.RunStatus{}, ctx.Err()
		case <-ticker.C:
		}
	}
}

func isTerminalManagedRunStatus(status string) bool {
	return status == "completed" || status == "failed"
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
		feedbackLLMIDs = append([]string{}, cfg.DefaultFeedbackLLMIDs...)
		if len(feedbackLLMIDs) == 0 {
			feedbackLLMIDs = append([]string{}, allowed...)
		}
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

func writeManagedFeedback(outDir string, status managed.RunStatus) error {
	if err := os.MkdirAll(filepath.Join(outDir, "runs"), 0o755); err != nil {
		return err
	}
	for i, report := range status.FeedbackReports {
		path := filepath.Join(outDir, "runs", fmt.Sprintf("feedback-%03d.md", i+1))
		if err := os.WriteFile(path, []byte(report+"\n"), 0o644); err != nil {
			return err
		}
	}
	return os.WriteFile(filepath.Join(outDir, "aggregate-feedback.md"), []byte(managedRunFeedbackMarkdown(status)), 0o644)
}

func managedRunFeedbackMarkdown(status managed.RunStatus) string {
	if status.AggregateFeedback != "" {
		return status.AggregateFeedback
	}
	if len(status.FeedbackReports) == 0 {
		return runner.AggregateFeedback(nil)
	}
	groups := managedRunFeedbackGroups(status)
	if len(groups) == 0 {
		return runner.AggregateFeedback(status.FeedbackReports)
	}
	var sb strings.Builder
	sb.WriteString("# Dari docs aggregate feedback\n")
	for _, group := range groups {
		sb.WriteString(fmt.Sprintf("\n\n---\n\n## Task %d\n", group.taskIndex))
		if group.task != "" {
			sb.WriteString("\n")
			sb.WriteString(group.task)
			sb.WriteString("\n")
		}
		for _, result := range group.results {
			if result.llmID != "" {
				sb.WriteString("\n### Tester LLM: ")
				sb.WriteString(result.llmID)
				sb.WriteString("\n")
			}
			sb.WriteString("\n")
			sb.WriteString(result.report)
			sb.WriteString("\n")
		}
	}
	return strings.TrimRight(sb.String(), "\n")
}

type managedFeedbackGroup struct {
	taskIndex int
	task      string
	results   []managedFeedbackResult
}

type managedFeedbackResult struct {
	llmID  string
	report string
}

func managedRunFeedbackGroups(status managed.RunStatus) []managedFeedbackGroup {
	completedSessions := make([]managed.RunSessionSummary, 0, len(status.Sessions))
	for _, session := range status.Sessions {
		if session.Kind == "tester" && session.Status == "completed" {
			completedSessions = append(completedSessions, session)
		}
	}
	if len(completedSessions) == 0 {
		return nil
	}
	groupsByTask := map[int]*managedFeedbackGroup{}
	for i, session := range completedSessions {
		if i >= len(status.FeedbackReports) {
			break
		}
		taskIndex := session.TaskIndex
		if taskIndex <= 0 {
			taskIndex = 1
		}
		group := groupsByTask[taskIndex]
		if group == nil {
			group = &managedFeedbackGroup{taskIndex: taskIndex, task: managedTaskLabel(status.Tasks, taskIndex)}
			groupsByTask[taskIndex] = group
		}
		group.results = append(group.results, managedFeedbackResult{llmID: session.LLMID, report: status.FeedbackReports[i]})
	}
	if len(groupsByTask) == 0 {
		return nil
	}
	indexes := make([]int, 0, len(groupsByTask))
	for taskIndex := range groupsByTask {
		indexes = append(indexes, taskIndex)
	}
	sort.Ints(indexes)
	groups := make([]managedFeedbackGroup, 0, len(indexes))
	for _, taskIndex := range indexes {
		groups = append(groups, *groupsByTask[taskIndex])
	}
	return groups
}

func managedTaskLabel(tasks []string, taskIndex int) string {
	if taskIndex > 0 && taskIndex <= len(tasks) {
		return strings.TrimSpace(tasks[taskIndex-1])
	}
	return ""
}

func downloadManagedRunArtifacts(ctx context.Context, client *managed.Client, status managed.RunStatus, outDir string) (string, error) {
	if !isTerminalManagedRunStatus(status.Status) {
		return "", fmt.Errorf("managed run %s is %s; artifacts are available after the run finishes", status.ID, status.Status)
	}
	if err := writeManagedFeedback(outDir, status); err != nil {
		return "", err
	}
	// Failed terminal runs can still have tester feedback; updated docs only exist for completed optimize runs.
	if status.Status != "completed" {
		return "", nil
	}
	if status.Mode == "check" {
		return "", nil
	}
	if status.Mode != "optimize" {
		return "", fmt.Errorf("managed run %s has unsupported mode %q", status.ID, status.Mode)
	}
	if !status.UpdatedDocsAvailable {
		return "", fmt.Errorf("updated docs are not available for managed run %s", status.ID)
	}
	return downloadManagedUpdatedDocs(ctx, client, status.ID, outDir)
}

func downloadManagedUpdatedDocs(ctx context.Context, client *managed.Client, runID, outDir string) (string, error) {
	zipPath := filepath.Join(outDir, "updated-docs-workspace.zip")
	if err := client.DownloadUpdatedDocs(ctx, runID, zipPath); err != nil {
		return "", err
	}
	extractDir := filepath.Join(outDir, "updated")
	_ = os.RemoveAll(extractDir)
	if err := dari.ExtractZip(zipPath, extractDir); err != nil {
		return "", err
	}
	return workspace.FindUpdatedDocsFilesDir(extractDir)
}

func applyManagedRunArtifacts(ctx context.Context, client *managed.Client, status managed.RunStatus, repoRoot, outDir string) error {
	if status.Mode != "optimize" {
		return fmt.Errorf("apply is only available for completed optimize runs")
	}
	if status.Status != "completed" {
		return fmt.Errorf("apply is only available for completed optimize runs")
	}
	updatedDir, err := downloadManagedRunArtifacts(ctx, client, status, outDir)
	if err != nil {
		return err
	}
	if updatedDir == "" {
		return fmt.Errorf("updated docs are not available for managed run %s", status.ID)
	}
	if err := workspace.CopyTree(updatedDir, repoRoot); err != nil {
		return fmt.Errorf("apply updated docs: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Applied updated docs into %s\n", repoRoot)
	return nil
}
