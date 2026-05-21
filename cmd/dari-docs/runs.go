package main

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/managed"
	"github.com/spf13/cobra"
)

func newRunsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "runs",
		Short:         "Inspect managed runs",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(
		newRunsStatusCommand(),
		newRunsWaitCommand(),
		newRunsFeedbackCommand(),
		newRunsDownloadCommand(),
		newRunsApplyCommand(),
	)
	return cmd
}

func newRunsStatusCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "status <run-id>",
		Short:         "Show run status",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRunsStatus(cmd.Context(), args[0])
		},
	}
}

func newRunsWaitCommand() *cobra.Command {
	var timeoutMinutes int
	cmd := &cobra.Command{
		Use:           "wait <run-id>",
		Short:         "Wait for a run to finish",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			if timeoutMinutes < 0 {
				return fmt.Errorf("--timeout-minutes must be a non-negative integer")
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRunsWait(cmd.Context(), args[0], timeoutMinutes)
		},
	}
	cmd.Flags().IntVar(&timeoutMinutes, "timeout-minutes", 30, "managed CLI wait timeout in minutes")
	return cmd
}

func newRunsFeedbackCommand() *cobra.Command {
	return &cobra.Command{
		Use:           "feedback <run-id>",
		Short:         "Print run feedback",
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runRunsFeedback(cmd.Context(), cmd.OutOrStdout(), args[0])
		},
	}
}

func newRunsDownloadCommand() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:           "download <run-id> [repo]",
		Short:         "Download run artifacts",
		Args:          cobra.RangeArgs(1, 2),
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE:       validateOutFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) == 2 {
				repo = args[1]
			}
			return runRunsDownload(cmd.Context(), args[0], repo, outDir)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	return cmd
}

func newRunsApplyCommand() *cobra.Command {
	var outDir string
	cmd := &cobra.Command{
		Use:           "apply <run-id> [repo]",
		Short:         "Apply run artifacts",
		Args:          cobra.RangeArgs(1, 2),
		SilenceUsage:  true,
		SilenceErrors: true,
		PreRunE:       validateOutFlag,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo := "."
			if len(args) == 2 {
				repo = args[1]
			}
			return runRunsApply(cmd.Context(), args[0], repo, outDir)
		},
	}
	cmd.Flags().StringVar(&outDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	return cmd
}

func validateOutFlag(cmd *cobra.Command, args []string) error {
	if flag := cmd.Flags().Lookup("out"); flag != nil && flag.Changed && strings.TrimSpace(flag.Value.String()) == "" {
		return fmt.Errorf("--out requires a directory")
	}
	return nil
}

func runRunsStatus(ctx context.Context, runID string) error {
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	printManagedRunStatus(status)
	return nil
}

func runRunsWait(ctx context.Context, runID string, timeoutMinutes int) error {
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := waitForManagedRun(ctx, client, runID, time.Duration(timeoutMinutes)*time.Minute)
	if err != nil {
		return err
	}
	printManagedRunStatus(status)
	if status.Status == "failed" {
		return fmt.Errorf("managed run %s failed: %s", status.ID, status.Error)
	}
	if status.Status == "completed" {
		if status.Mode == "optimize" && status.UpdatedDocsAvailable {
			fmt.Printf("\nDownload updated docs:\n  dari-docs runs download %s\n", status.ID)
			fmt.Printf("Apply updated docs:\n  dari-docs runs apply %s\n", status.ID)
		} else {
			fmt.Printf("\nDownload feedback:\n  dari-docs runs download %s\n", status.ID)
		}
	}
	return nil
}

func runRunsFeedback(ctx context.Context, out io.Writer, runID string) error {
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	feedback, err := managedRunFeedbackMarkdown(status)
	if err != nil {
		return err
	}
	_, err = fmt.Fprint(out, feedback)
	return err
}

func runRunsDownload(ctx context.Context, runID string, repo string, outDir string) error {
	repoRoot, outDir, err := resolveRunArtifactPaths(repo, outDir)
	if err != nil {
		return err
	}
	_ = repoRoot
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	updatedDir, err := downloadManagedRunArtifacts(ctx, client, status, outDir)
	if err != nil {
		return err
	}
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if updatedDir != "" {
		fmt.Printf("Updated docs: %s\n", updatedDir)
	}
	return nil
}

func runRunsApply(ctx context.Context, runID string, repo string, outDir string) error {
	repoRoot, outDir, err := resolveRunArtifactPaths(repo, outDir)
	if err != nil {
		return err
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(ctx, runID)
	if err != nil {
		return err
	}
	if err := applyManagedRunArtifacts(ctx, client, status, repoRoot, outDir); err != nil {
		return err
	}
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	return nil
}

func managedRunFeedbackMarkdown(status managed.RunStatus) (string, error) {
	if len(status.FeedbackReports) == 0 {
		if strings.TrimSpace(status.AggregateFeedback) != "" {
			return ensureTrailingNewline(status.AggregateFeedback), nil
		}
		if !isTerminalManagedRunStatus(status.Status) {
			return "", fmt.Errorf("managed run %s is %s; feedback is available after the run finishes", status.ID, status.Status)
		}
		return "", fmt.Errorf("no feedback available for managed run %s", status.ID)
	}

	completedSessions := completedTesterSessions(status)
	var sb strings.Builder
	sb.WriteString("# Dari Docs Feedback\n\n")
	sb.WriteString("Run: " + status.ID + "\n")
	if status.Status != "" {
		sb.WriteString("Status: " + status.Status + "\n")
	}
	if status.Mode != "" {
		sb.WriteString("Type: " + status.Mode + "\n")
	}
	if status.CompletedAt != nil {
		sb.WriteString("Completed: " + formatCLITime(*status.CompletedAt) + "\n")
	}
	sb.WriteString("\n")

	lastTaskIndex := 0
	for i, report := range status.FeedbackReports {
		report = strings.TrimSpace(report)
		if report == "" {
			continue
		}
		if i < len(completedSessions) {
			session := completedSessions[i]
			taskIndex := session.TaskIndex
			if taskIndex <= 0 {
				taskIndex = 1
			}
			if taskIndex != lastTaskIndex {
				if lastTaskIndex != 0 {
					sb.WriteString("\n")
				}
				sb.WriteString(fmt.Sprintf("## Task %d\n\n", taskIndex))
				if taskIndex <= len(status.Tasks) && strings.TrimSpace(status.Tasks[taskIndex-1]) != "" {
					sb.WriteString(strings.TrimSpace(status.Tasks[taskIndex-1]) + "\n\n")
				}
				lastTaskIndex = taskIndex
			}
			label := strings.TrimSpace(session.LLMID)
			if label == "" {
				label = "default"
			}
			sb.WriteString(fmt.Sprintf("### %s Feedback\n\n", label))
			sb.WriteString(report + "\n\n")
			continue
		}
		if lastTaskIndex != 0 {
			sb.WriteString("\n")
		}
		sb.WriteString(fmt.Sprintf("## Feedback %03d\n\n%s\n", i+1, report))
		lastTaskIndex = 0
	}
	return ensureTrailingNewline(sb.String()), nil
}

func completedTesterSessions(status managed.RunStatus) []managed.RunSessionSummary {
	out := make([]managed.RunSessionSummary, 0, len(status.Sessions))
	for _, session := range status.Sessions {
		if session.Kind == "tester" && session.Status == "completed" {
			out = append(out, session)
		}
	}
	return out
}

func ensureTrailingNewline(s string) string {
	if strings.HasSuffix(s, "\n") {
		return s
	}
	return s + "\n"
}

func resolveRunArtifactPaths(repo string, outDir string) (string, string, error) {
	if repo == "" {
		repo = "."
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return "", "", err
	}
	if outDir == "" {
		outDir = filepath.Join(absRepo, ".dari-docs")
	}
	return absRepo, outDir, nil
}

func printManagedRunStatus(status managed.RunStatus) {
	fmt.Printf("Run: %s\n", status.ID)
	fmt.Printf("Status: %s\n", status.Status)
	fmt.Printf("Type: %s\n", status.Mode)
	if status.TaskCount > 0 {
		fmt.Printf("Tasks: %d\n", status.TaskCount)
	} else if len(status.Tasks) > 0 {
		fmt.Printf("Tasks: %d\n", len(status.Tasks))
	}
	if len(status.LLMs) > 0 {
		var parts []string
		for _, summary := range status.LLMs {
			if summary.LLMID == "" {
				continue
			}
			label := summary.Role + ": " + summary.LLMID
			if summary.Count > 1 {
				label += fmt.Sprintf(" (%d)", summary.Count)
			}
			parts = append(parts, label)
		}
		if len(parts) > 0 {
			fmt.Printf("LLMs: %s\n", strings.Join(parts, ", "))
		}
	}
	if !status.CreatedAt.IsZero() {
		fmt.Printf("Created: %s\n", formatCLITime(status.CreatedAt))
	}
	if status.CompletedAt != nil {
		fmt.Printf("Completed: %s\n", formatCLITime(*status.CompletedAt))
	}
	fmt.Printf("Reserved: %s\n", formatCents(status.ReservedCents))
	if status.Status == "completed" || status.Status == "failed" {
		charged := formatCents(status.ChargedCents)
		if status.Estimated {
			charged += " estimated"
		}
		fmt.Printf("Charged: %s\n", charged)
	}
	if status.Error != "" {
		fmt.Printf("Error: %s\n", status.Error)
	}
	if isTerminalManagedRunStatus(status.Status) {
		if len(status.FeedbackReports) > 0 || status.AggregateFeedback != "" {
			fmt.Println("Feedback: available")
		}
		if status.UpdatedDocsAvailable {
			fmt.Println("Updated docs: available")
		}
	}
}

func formatCLITime(t time.Time) string {
	return t.Local().Format("2006-01-02 15:04")
}
