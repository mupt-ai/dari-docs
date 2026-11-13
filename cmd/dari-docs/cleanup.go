package main

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/ablation"
	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/github"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
	"github.com/spf13/cobra"
)

type cleanupOptions struct {
	common
	PR          int
	GitHubToken string
}

func newCleanupCommand() *cobra.Command {
	o := cleanupOptions{}
	cmd := &cobra.Command{
		Use:   "cleanup",
		Short: "Stop a pull request's comparison and delete the repo ablations its checks built",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if o.PR == 0 {
				event, ok, err := github.EventPullRequest()
				if err != nil {
					return err
				}
				if !ok {
					return errNoPR
				}
				o.PR = event.Number
			}
			return runCleanup(cmd.Context(), o)
		},
	}
	o.register(cmd)
	cmd.Flags().IntVar(&o.PR, "pr", 0, "pull request number (default: from the GitHub event)")
	cmd.Flags().StringVar(&o.GitHubToken, "github-token", os.Getenv("GITHUB_TOKEN"), "token to read and update the pull request's comment")
	return cmd
}

func runCleanup(ctx context.Context, o cleanupOptions) error {
	cfg, err := config.Load(o.ConfigPath)
	if err != nil {
		return err
	}
	repo, err := repository(ctx, o.Repo, cfg)
	if err != nil {
		return err
	}
	sb, err := client(cfg)
	if err != nil {
		return err
	}
	var gh *github.Client
	var comment *github.Comment
	if o.GitHubToken != "" {
		gh = github.FromEnv(o.GitHubToken)
		if comment, err = gh.FindComment(ctx, repo, o.PR); err != nil {
			fmt.Fprintln(os.Stderr, "dari-docs: reading pull request comments:", err)
		}
	}
	if comment != nil && comment.State.Comparison != "" {
		if err := sb.CancelComparison(ctx, repo, comment.State.Comparison); err != nil && !selfbench.IsStatus(err, 404) {
			fmt.Fprintln(os.Stderr, "dari-docs: stopping comparison:", err)
		}
	}
	envs, err := sb.Environments(ctx, repo)
	if err != nil {
		return err
	}
	prefix := ablation.PRPrefix(o.PR)
	deleted := 0
	var failed []string
	for _, e := range envs {
		if !strings.HasPrefix(e.Name, prefix) {
			continue
		}
		if err := sb.DeleteEnvironment(ctx, repo, e.ID); err != nil && !selfbench.IsStatus(err, 404) {
			failed = append(failed, fmt.Sprintf("%s: %v", e.Name, err))
			continue
		}
		deleted++
	}
	fmt.Printf("dari-docs: deleted %d repo %s for #%d\n", deleted, map[bool]string{true: "ablation", false: "ablations"}[deleted == 1], o.PR)
	if len(failed) > 0 {
		return fmt.Errorf("could not delete: %s", strings.Join(failed, "; "))
	}
	return nil
}
