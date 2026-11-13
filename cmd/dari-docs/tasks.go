package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
	"github.com/spf13/cobra"
)

func newTasksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List the repository's SelfBench tasks, or add one from a scenario",
	}
	cmd.AddCommand(newTasksListCommand(), newTasksAddCommand())
	return cmd
}

func newTasksListCommand() *cobra.Command {
	var c common
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the repository's SelfBench tasks; checks run the approved ones",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load(c.ConfigPath)
			if err != nil {
				return err
			}
			repo, err := repository(cmd.Context(), c.Repo, cfg)
			if err != nil {
				return err
			}
			sb, err := client(cfg)
			if err != nil {
				return err
			}
			tasks, err := sb.Tasks(cmd.Context(), repo)
			if err != nil {
				return err
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "TASK\tSTATE\tTITLE")
			for _, t := range tasks {
				title := t.Title
				if title == "" && t.SourcePR != nil {
					title = fmt.Sprintf("PR #%d", *t.SourcePR)
				}
				fmt.Fprintf(w, "%s/%s\t%s\t%s\n", t.RunID, t.TaskID, t.State, title)
			}
			return w.Flush()
		},
	}
	c.register(cmd)
	return cmd
}

func newTasksAddCommand() *cobra.Command {
	var c common
	var title, file, commit, difficulty string
	cmd := &cobra.Command{
		Use:   "add [scenario]",
		Short: "Have SelfBench author a task from what a reader of the docs should be able to do",
		Long: `add starts a repository vibe eval on SelfBench: an authoring agent turns the scenario into a task
with tests, from the repository at --commit. Review and approve it in SelfBench; docs checks run every
approved task. Pass the scenario as an argument or with --file.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load(c.ConfigPath)
			if err != nil {
				return err
			}
			scenario := ""
			if len(args) == 1 {
				scenario = args[0]
			}
			if file != "" {
				b, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				scenario = string(b)
			}
			scenario = strings.TrimSpace(scenario)
			if scenario == "" || title == "" {
				return fmt.Errorf("tasks add needs --title and a scenario")
			}
			repo, err := repository(cmd.Context(), c.Repo, cfg)
			if err != nil {
				return err
			}
			sb, err := client(cfg)
			if err != nil {
				return err
			}
			task, err := sb.CreateScenario(cmd.Context(), repo, selfbench.ScenarioRequest{
				Title:      title,
				Scenario:   scenario,
				Commit:     commit,
				Difficulty: difficulty,
				Generation: cfg.Generation,
			})
			if err != nil {
				return err
			}
			fmt.Printf("authoring %s/%s; approve it in SelfBench when it is ready: %s\n", task.RunID, task.TaskID, sb.WebURL(repo, fmt.Sprintf("tasks/%s/%s", task.RunID, task.TaskID)))
			return nil
		},
	}
	c.register(cmd)
	cmd.Flags().StringVar(&title, "title", "", "short name for the task")
	cmd.Flags().StringVar(&file, "file", "", "read the scenario from this file")
	cmd.Flags().StringVar(&commit, "commit", "HEAD", "commit, branch, or HEAD the task starts from")
	cmd.Flags().StringVar(&difficulty, "difficulty", "", "easy, medium, or hard")
	return cmd
}
