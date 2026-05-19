package main

import (
	"bytes"
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/managed"
)

func runRuns(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: dari-docs runs [status|wait|download|apply]")
	}
	switch args[0] {
	case "status":
		return runRunsStatus(args[1:])
	case "wait":
		return runRunsWait(args[1:])
	case "download":
		return runRunsDownload(args[1:])
	case "apply":
		return runRunsApply(args[1:])
	default:
		return fmt.Errorf("usage: dari-docs runs [status|wait|download|apply]")
	}
}

func runRunsStatus(args []string) error {
	fs := flag.NewFlagSet("dari-docs runs status", flag.ExitOnError)
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() != 1 {
		return fmt.Errorf("usage: dari-docs runs status <run-id>")
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(context.Background(), fs.Arg(0))
	if err != nil {
		return err
	}
	printManagedRunStatus(status)
	return nil
}

func runRunsWait(args []string) error {
	parsed, err := parseRunsWaitArgs(args)
	if err != nil {
		return err
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := waitForManagedRun(context.Background(), client, parsed.RunID, time.Duration(parsed.TimeoutMinutes)*time.Minute)
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

type runsWaitArgs struct {
	RunID          string
	TimeoutMinutes int
}

func parseRunsWaitArgs(args []string) (runsWaitArgs, error) {
	var err error
	parsed := runsWaitArgs{TimeoutMinutes: 30}
	args, parsed.TimeoutMinutes, err = extractTimeoutMinutesFlag(args, parsed.TimeoutMinutes)
	if err != nil {
		return runsWaitArgs{}, err
	}
	fs := flag.NewFlagSet("dari-docs runs wait", flag.ContinueOnError)
	var flagOutput bytes.Buffer
	fs.SetOutput(&flagOutput)
	fs.IntVar(&parsed.TimeoutMinutes, "timeout-minutes", parsed.TimeoutMinutes, "managed CLI wait timeout in minutes")
	if err := fs.Parse(args); err != nil {
		return runsWaitArgs{}, err
	}
	if fs.NArg() != 1 {
		return runsWaitArgs{}, fmt.Errorf("usage: dari-docs runs wait <run-id>")
	}
	parsed.RunID = fs.Arg(0)
	return parsed, nil
}

func runRunsDownload(args []string) error {
	runID, _, outDir, err := parseRunArtifactArgs("download", args)
	if err != nil {
		return err
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(context.Background(), runID)
	if err != nil {
		return err
	}
	updatedDir, err := downloadManagedRunArtifacts(context.Background(), client, status, outDir)
	if err != nil {
		return err
	}
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if updatedDir != "" {
		fmt.Printf("Updated docs: %s\n", updatedDir)
	}
	return nil
}

func runRunsApply(args []string) error {
	runID, repoRoot, outDir, err := parseRunArtifactArgs("apply", args)
	if err != nil {
		return err
	}
	client, err := managedClientWithToken()
	if err != nil {
		return err
	}
	status, err := client.GetRun(context.Background(), runID)
	if err != nil {
		return err
	}
	if err := applyManagedRunArtifacts(context.Background(), client, status, repoRoot, outDir); err != nil {
		return err
	}
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	return nil
}

func parseRunArtifactArgs(command string, args []string) (string, string, string, error) {
	var outDir string
	var err error
	args, outDir, err = extractOutFlag(args)
	if err != nil {
		return "", "", "", err
	}
	if len(args) < 1 || len(args) > 2 {
		return "", "", "", fmt.Errorf("usage: dari-docs runs %s <run-id> [repo] [--out DIR]", command)
	}
	repo := "."
	if len(args) == 2 {
		repo = args[1]
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return "", "", "", err
	}
	if outDir == "" {
		outDir = filepath.Join(absRepo, ".dari-docs")
	}
	return args[0], absRepo, outDir, nil
}

func extractTimeoutMinutesFlag(args []string, defaultValue int) ([]string, int, error) {
	timeoutMinutes := defaultValue
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--timeout-minutes" {
			if i+1 >= len(args) {
				return nil, 0, fmt.Errorf("--timeout-minutes requires a value")
			}
			n, err := strconv.Atoi(args[i+1])
			if err != nil || n < 0 {
				return nil, 0, fmt.Errorf("--timeout-minutes must be a non-negative integer")
			}
			timeoutMinutes = n
			i++
			continue
		}
		if strings.HasPrefix(arg, "--timeout-minutes=") {
			raw := strings.TrimSpace(strings.TrimPrefix(arg, "--timeout-minutes="))
			n, err := strconv.Atoi(raw)
			if err != nil || n < 0 {
				return nil, 0, fmt.Errorf("--timeout-minutes must be a non-negative integer")
			}
			timeoutMinutes = n
			continue
		}
		out = append(out, arg)
	}
	return out, timeoutMinutes, nil
}

func extractOutFlag(args []string) ([]string, string, error) {
	var outDir string
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--out" {
			if i+1 >= len(args) {
				return nil, "", fmt.Errorf("--out requires a directory")
			}
			outDir = args[i+1]
			i++
			continue
		}
		if strings.HasPrefix(arg, "--out=") {
			outDir = strings.TrimSpace(strings.TrimPrefix(arg, "--out="))
			if outDir == "" {
				return nil, "", fmt.Errorf("--out requires a directory")
			}
			continue
		}
		out = append(out, arg)
	}
	return out, outDir, nil
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
