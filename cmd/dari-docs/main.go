// Command dari-docs checks whether a docs change helps coding agents get real tasks done. It is a
// thin client of SelfBench: SelfBench builds and runs the tasks, and dari-docs turns a pull
// request's docs change into two repo ablations and reports the difference on the pull request.
package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"strings"
	"syscall"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
	"github.com/spf13/cobra"
)

var version = "dev"

// exitError ends the process with a code and no further message.
type exitError struct{ code int }

func (e exitError) Error() string { return fmt.Sprintf("exit %d", e.code) }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	err := newRoot().ExecuteContext(ctx)
	var exit exitError
	switch {
	case errors.As(err, &exit):
		os.Exit(exit.code)
	case err != nil:
		fmt.Fprintln(os.Stderr, "dari-docs:", err)
		os.Exit(1)
	}
}

func newRoot() *cobra.Command {
	root := &cobra.Command{
		Use:           "dari-docs",
		Short:         "Check whether a docs change helps coding agents finish real tasks, on SelfBench",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.AddCommand(newCheckCommand(), newCleanupCommand(), newInitCommand(), newTasksCommand())
	return root
}

// common are the flags every SelfBench command takes.
type common struct {
	ConfigPath string
	Repo       string
}

func (c *common) register(cmd *cobra.Command) {
	cmd.Flags().StringVar(&c.ConfigPath, "config", config.Path("."), "config file")
	cmd.Flags().StringVar(&c.Repo, "repo", "", "connected repository, owner/name (default: config, then GITHUB_REPOSITORY, then the origin remote)")
}

// client is a SelfBench client from the config and environment: SELFBENCH_API_KEY, and
// SELFBENCH_ORG and SELFBENCH_API_URL over the config's.
func client(cfg config.Config) (*selfbench.Client, error) {
	key := os.Getenv("SELFBENCH_API_KEY")
	if key == "" {
		return nil, errors.New("set SELFBENCH_API_KEY (SelfBench → Settings → API Keys)")
	}
	org := firstNonEmpty(os.Getenv("SELFBENCH_ORG"), cfg.Org)
	if org == "" {
		return nil, errors.New("set the SelfBench workspace: `org` in the config or SELFBENCH_ORG")
	}
	return &selfbench.Client{
		BaseURL: firstNonEmpty(os.Getenv("SELFBENCH_API_URL"), cfg.APIURL),
		APIKey:  key,
		Org:     org,
	}, nil
}

// repository is the connected repository: the flag, the config, the Actions job's, or origin's.
func repository(ctx context.Context, flag string, cfg config.Config) (string, error) {
	if repo := firstNonEmpty(flag, cfg.Repo, os.Getenv("GITHUB_REPOSITORY")); repo != "" {
		return repo, nil
	}
	out, err := exec.CommandContext(ctx, "git", "remote", "get-url", "origin").Output()
	if err == nil {
		if repo := repoFromRemote(strings.TrimSpace(string(out))); repo != "" {
			return repo, nil
		}
	}
	return "", errors.New("name the repository with --repo owner/name or `repo` in the config")
}

// repoFromRemote reads owner/name from a GitHub remote URL.
func repoFromRemote(remote string) string {
	remote = strings.TrimSuffix(remote, ".git")
	for _, prefix := range []string{"git@github.com:", "https://github.com/", "ssh://git@github.com/"} {
		if rest, ok := strings.CutPrefix(remote, prefix); ok {
			if owner, name, ok := strings.Cut(rest, "/"); ok && owner != "" && name != "" && !strings.Contains(name, "/") {
				return owner + "/" + name
			}
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
