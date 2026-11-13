package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/ablation"
	"github.com/mupt-ai/dari-docs/internal/check"
	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/docsdiff"
	"github.com/mupt-ai/dari-docs/internal/github"
	"github.com/mupt-ai/dari-docs/internal/report"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
	"github.com/spf13/cobra"
)

type checkOptions struct {
	common
	Base        string
	Head        string
	PR          int
	DryRun      bool
	Fresh       bool
	Poll        time.Duration
	GitHubToken string
	NoComment   bool
}

func newCheckCommand() *cobra.Command {
	o := checkOptions{}
	cmd := &cobra.Command{
		Use:   "check",
		Short: "Run SelfBench's tasks with the docs before and after a change, and report what changed",
		Long: `check compares the docs at the merge base of --base and --head with --head's. SelfBench builds
every approved task twice, once with each version of the changed docs (two repo ablations), runs the
configured models on both, and dari-docs reports the tasks whose results changed.

In a GitHub Actions pull_request job it reads the pull request from the event, keeps one comment on it
up to date, and on a closed pull request cleans up instead (see cleanup).`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error { return runCheck(cmd.Context(), o) },
	}
	o.register(cmd)
	cmd.Flags().StringVar(&o.Base, "base", "", "base revision (default: the pull request's base, else origin/main)")
	cmd.Flags().StringVar(&o.Head, "head", "", "head revision (default: the pull request's head, else HEAD)")
	cmd.Flags().IntVar(&o.PR, "pr", 0, "pull request number (default: from the GitHub event)")
	cmd.Flags().BoolVar(&o.DryRun, "dry-run", false, "print the changed docs, the two ablation scripts, and the comparison, without calling SelfBench")
	cmd.Flags().BoolVar(&o.Fresh, "fresh", false, "start a new comparison even if this exact check already ran")
	cmd.Flags().DurationVar(&o.Poll, "poll", 30*time.Second, "how often to read SelfBench while waiting")
	cmd.Flags().StringVar(&o.GitHubToken, "github-token", os.Getenv("GITHUB_TOKEN"), "token to comment on the pull request")
	cmd.Flags().BoolVar(&o.NoComment, "no-comment", false, "don't comment on the pull request")
	return cmd
}

func runCheck(ctx context.Context, o checkOptions) error {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return err
	}
	event, inPR, err := github.EventPullRequest()
	if err != nil {
		return err
	}
	if inPR {
		if event.Action == "closed" {
			return runCleanup(ctx, cleanupOptions{common: o.common, PR: event.Number, GitHubToken: o.GitHubToken})
		}
		o.Base = firstNonEmpty(o.Base, event.BaseSHA)
		o.Head = firstNonEmpty(o.Head, event.HeadSHA)
		if o.PR == 0 {
			o.PR = event.Number
		}
		if o.Repo == "" && cfg.Repo == "" {
			o.Repo = event.Repo
		}
	}
	o.Base = firstNonEmpty(o.Base, "origin/main")
	o.Head = firstNonEmpty(o.Head, "HEAD")
	repo, err := repository(ctx, o.Repo, cfg)
	if err != nil {
		return err
	}

	change, err := docsdiff.Git{Dir: "."}.Diff(ctx, o.Base, o.Head, cfg.Docs)
	if err != nil {
		return err
	}
	ctxReport := report.Context{Paths: change.Paths(), Skipped: change.Skipped, Head: change.Head}
	if o.DryRun {
		return dryRun(o, cfg, repo, change)
	}

	if inPR && event.Fork && os.Getenv("SELFBENCH_API_KEY") == "" {
		fmt.Println("dari-docs: a pull request from a fork gets no secrets, so there is no SelfBench key; skipping.")
		return nil
	}
	var gh *github.Client
	var comment *github.Comment
	commenting := inPR && o.PR > 0 && o.GitHubToken != "" && !o.NoComment && event.Repo == repo
	if commenting {
		gh = github.FromEnv(o.GitHubToken)
		if comment, err = gh.FindComment(ctx, repo, o.PR); err != nil {
			fmt.Fprintln(os.Stderr, "dari-docs: reading pull request comments:", err)
			commenting = false
		}
	}
	state := github.State{}
	if comment != nil {
		state = comment.State
	}
	post := func(markdown string) {
		if !commenting {
			return
		}
		updated, err := gh.UpsertComment(ctx, repo, o.PR, comment, github.Body(markdown, state))
		if err != nil {
			fmt.Fprintln(os.Stderr, "dari-docs: commenting:", err)
			return
		}
		comment = updated
	}

	if change.Empty() {
		fmt.Println("dari-docs: this change touches no docs; nothing to check.")
		// Only replace an earlier result, which this change may have made stale.
		if comment != nil {
			post("### dari-docs: no docs changes\n\nThis pull request no longer changes any docs, so there is nothing to check.\n")
		}
		return nil
	}

	sb, err := client(cfg)
	if err != nil {
		return err
	}
	previous := state.Comparison
	opts := check.Options{
		Repo:   repo,
		PR:     o.PR,
		Config: cfg,
		Change: change,
		Fresh:  o.Fresh,
		Poll:   o.Poll,
		Log:    func(format string, args ...any) { fmt.Fprintf(os.Stderr, "dari-docs: "+format+"\n", args...) },
		OnProgress: func(p check.Progress) {
			if p.Comparison != nil && p.Comparison.ID != state.Comparison {
				// A newer push supersedes the comparison an earlier run started.
				if previous != "" && previous != p.Comparison.ID {
					if err := sb.CancelComparison(ctx, repo, previous); err != nil && !selfbench.IsStatus(err, 404) {
						fmt.Fprintln(os.Stderr, "dari-docs: stopping the previous comparison:", err)
					}
					previous = ""
				}
				state.Comparison = p.Comparison.ID
			}
			footer := ctxReport
			if p.Comparison != nil {
				footer.ComparisonURL = sb.WebURL(repo, "comparisons/"+p.Comparison.ID)
			}
			post(check.RunningMarkdown(p, report.Footer(footer)))
		},
	}
	result, err := check.Run(ctx, sb, opts)
	if result.ComparisonID != "" {
		ctxReport.ComparisonURL = sb.WebURL(repo, "comparisons/"+result.ComparisonID)
	}
	if err != nil {
		post(fmt.Sprintf("### dari-docs: the check did not finish\n\n%s\n\n%s", quoteBlock(err.Error()), report.Footer(ctxReport)))
		return err
	}
	markdown := result.Report.Markdown(ctxReport)
	post(markdown)
	if err := github.StepSummary(markdown); err != nil {
		fmt.Fprintln(os.Stderr, "dari-docs: writing the job summary:", err)
	}
	fmt.Println(markdown)
	if cfg.FailOnRegression && result.Report.Regressed() {
		fmt.Fprintln(os.Stderr, "dari-docs: some tasks pass with the old docs and fail with the new ones")
		return exitError{code: 2}
	}
	return nil
}

func dryRun(o checkOptions, cfg config.Config, repo string, change docsdiff.Change) error {
	fmt.Printf("repository: %s\nmerge base: %s\nhead:       %s\n", repo, change.Base, change.Head)
	if change.Empty() {
		fmt.Println("no docs changed")
		return nil
	}
	fmt.Println("changed docs:")
	for _, p := range change.Paths() {
		fmt.Println("  " + p)
	}
	for _, p := range change.Skipped {
		fmt.Println("  " + p + " (skipped: not a regular file)")
	}
	for _, side := range []struct {
		name string
		snap docsdiff.Snapshot
	}{{check.BeforeName, change.Before}, {check.AfterName, change.After}} {
		script, err := ablation.Script(side.snap)
		if err != nil {
			return err
		}
		fmt.Printf("\nrepo ablation %q (%d bytes):\n%s", ablation.Name(o.PR, side.name, script), len(script), truncate(script, 2000))
	}
	models, _ := json.MarshalIndent(cfg.Models, "", "  ")
	fmt.Printf("\nmodels:\n%s\n", models)
	return nil
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + fmt.Sprintf("… (%d more bytes)\n", len(s)-n)
}

func quoteBlock(s string) string {
	lines := strings.Split(strings.TrimSpace(s), "\n")
	for i, l := range lines {
		lines[i] = "> " + l
	}
	return strings.Join(lines, "\n")
}

var errNoPR = errors.New("name the pull request with --pr, or run in a pull_request job")
