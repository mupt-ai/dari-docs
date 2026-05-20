package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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
	status, err := waitForManagedRun(ctx, client, created.RunID, cfg.Timeout)
	if err != nil {
		return err
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

func downloadManagedRunArtifacts(ctx context.Context, client *managed.Client, status managed.RunStatus, outDir string) (string, error) {
	if !isTerminalManagedRunStatus(status.Status) {
		return "", fmt.Errorf("managed run %s is %s; artifacts are available after the run finishes", status.ID, status.Status)
	}
	if err := writeManagedFeedback(outDir, status.FeedbackReports, status.AggregateFeedback); err != nil {
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
