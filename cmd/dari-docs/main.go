// Command dari-docs checks whether a docs change helps coding agents get real tasks done. It is a
// thin client of SelfBench: SelfBench builds and runs the tasks, and dari-docs turns a pull
// request's docs change into two repo ablations and reports the difference on the pull request.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
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

// login is what `self-bench login` saved, which dari-docs uses when nothing else names a key.
type login struct {
	APIURL string `json:"apiUrl"`
	APIKey string `json:"apiKey"`
	Org    string `json:"org"`
}

func readLogin() login {
	dir := os.Getenv("XDG_CONFIG_HOME")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return login{}
		}
		dir = filepath.Join(home, ".config")
	}
	var l login
	b, err := os.ReadFile(filepath.Join(dir, "self-bench", "config.json"))
	if err != nil || json.Unmarshal(b, &l) != nil {
		return login{}
	}
	l.APIURL = strings.TrimRight(firstNonEmpty(l.APIURL, config.DefaultAPIURL), "/")
	return l
}

// connection is where SelfBench is and who to call it as: SELFBENCH_API_URL, SELFBENCH_API_KEY, and
// SELFBENCH_ORG first, then the config, then the self-bench CLI's login for the same deployment.
func connection(cfg config.Config) (*selfbench.Client, error) {
	l := readLogin()
	base := strings.TrimRight(firstNonEmpty(os.Getenv("SELFBENCH_API_URL"), cfg.APIURL, l.APIURL, config.DefaultAPIURL), "/")
	if l.APIURL != base {
		// A login for another deployment holds a key this one won't take.
		l = login{}
	}
	key := firstNonEmpty(os.Getenv("SELFBENCH_API_KEY"), l.APIKey)
	if key == "" {
		return nil, fmt.Errorf("no SelfBench API key: run `self-bench login`, or set SELFBENCH_API_KEY (%s/settings/api-keys)", base)
	}
	org := firstNonEmpty(os.Getenv("SELFBENCH_ORG"), cfg.Org, l.Org)
	if org == "" {
		return nil, errors.New("no SelfBench workspace: set `org` in the config or SELFBENCH_ORG")
	}
	return &selfbench.Client{BaseURL: base, APIKey: key, Org: org}, nil
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
