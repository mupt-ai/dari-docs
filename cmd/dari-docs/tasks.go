package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
	"github.com/mupt-ai/dari-docs/internal/tasksfile"
	"github.com/spf13/cobra"
)

func newTasksCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "tasks",
		Short: "List the tasks docs checks run, or add new ones",
	}
	cmd.AddCommand(newTasksListCommand(), newTasksAddCommand())
	return cmd
}

func newTasksListCommand() *cobra.Command {
	var c common
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List the repository's SelfBench tasks, and which ones docs checks run",
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
			sb, err := connection(cfg)
			if err != nil {
				return err
			}
			tasks, err := sb.Tasks(cmd.Context(), repo)
			if err != nil {
				return err
			}
			listed := map[string]bool{}
			for _, t := range cfg.Tasks {
				listed[t] = true
			}
			w := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
			fmt.Fprintln(w, "TASK\tSTATE\tCHECKED\tTITLE")
			for _, t := range tasks {
				ref := t.RunID + "/" + t.TaskID
				title := t.Title
				if title == "" && t.SourcePR != nil {
					title = fmt.Sprintf("PR #%d", *t.SourcePR)
				}
				// SelfBench calls an approved task "accepted"; "needs_review" passed its checks.
				approved := t.State == "accepted"
				checked := "no"
				switch {
				case (len(cfg.Tasks) == 0 || listed[ref]) && approved:
					checked = "yes"
				case listed[ref]:
					checked = "once approved"
				}
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", ref, taskState(t.State), checked, title)
			}
			if err := w.Flush(); err != nil {
				return err
			}
			if len(cfg.Tasks) == 0 {
				fmt.Println("\nThe config lists no tasks, so checks run every approved task, including ones built from merged pull requests.")
			}
			return nil
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
		Short: "Have SelfBench write tasks from what a reader of the docs should be able to do",
		Long: `add starts a repository vibe eval on SelfBench for each scenario: an authoring agent turns it into a
task with tests, from the repository at --commit. Each new task is added to the config's ` + "`tasks`" + `, so docs
checks run it once you approve it in SelfBench.

Pass one scenario as an argument, or a file with --file: one task per paragraph or bullet, the format
older dari-docs task files use. A task's title is its first line unless --title names it.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			var scenarios []tasksfile.Task
			switch {
			case file != "" && len(args) > 0:
				return fmt.Errorf("pass a scenario or --file, not both")
			case file != "":
				b, err := os.ReadFile(file)
				if err != nil {
					return err
				}
				if scenarios = tasksfile.Parse(string(b)); len(scenarios) == 0 {
					return fmt.Errorf("%s has no tasks", file)
				}
			case len(args) == 1 && strings.TrimSpace(args[0]) != "":
				// One argument is one task, whatever blank lines or bullets it holds.
				s := strings.TrimSpace(args[0])
				scenarios = []tasksfile.Task{{Title: firstNonEmpty(title, tasksfile.Title(s)), Scenario: s}}
			default:
				return fmt.Errorf("tasks add needs a scenario or --file")
			}
			if title != "" && len(scenarios) > 1 {
				return fmt.Errorf("--title names one task; titles in --file come from each task's first line")
			}

			raw, err := config.Read(c.ConfigPath)
			if err != nil {
				return err
			}
			cfg, err := config.Load(c.ConfigPath)
			if err != nil {
				return err
			}
			repo, err := repository(cmd.Context(), c.Repo, cfg)
			if err != nil {
				return err
			}
			sb, err := connection(cfg)
			if err != nil {
				return err
			}
			var added []string
			failed := 0
			for _, s := range scenarios {
				task, err := sb.CreateScenario(cmd.Context(), repo, selfbench.ScenarioRequest{
					Title:      s.Title,
					Scenario:   s.Scenario,
					Commit:     commit,
					Difficulty: difficulty,
					Generation: cfg.Generation,
				})
				if err != nil {
					failed++
					fmt.Fprintf(os.Stderr, "dari-docs: %q: %v\n", s.Title, err)
					continue
				}
				ref := task.RunID + "/" + task.TaskID
				added = append(added, ref)
				fmt.Printf("authoring %s  %s\n", ref, s.Title)
			}
			// Record what started even when some failed, so no task is authored and then forgotten.
			if len(added) > 0 {
				raw.AddTasks(added...)
				if err := config.Save(c.ConfigPath, raw); err != nil {
					return fmt.Errorf("the tasks started, but saving them to %s failed: %w (add them to `tasks` by hand: %s)", c.ConfigPath, err, strings.Join(added, ", "))
				}
				fmt.Printf("\nAdded %d to `tasks` in %s. SelfBench is writing and checking them; approve each one there when it's ready:\n  %s\n",
					len(added), c.ConfigPath, sb.WebURL(repo, ""))
			}
			if failed > 0 {
				return fmt.Errorf("%d of %d tasks did not start", failed, len(scenarios))
			}
			return nil
		},
	}
	c.register(cmd)
	cmd.Flags().StringVar(&title, "title", "", "short name for the task (default: its first line)")
	cmd.Flags().StringVar(&file, "file", "", "file with one task per paragraph or bullet")
	cmd.Flags().StringVar(&commit, "commit", "HEAD", "commit, branch, or HEAD the tasks start from")
	cmd.Flags().StringVar(&difficulty, "difficulty", "", "easy, medium, or hard")
	return cmd
}

// taskState says a SelfBench task state the way the app does.
func taskState(state string) string {
	switch state {
	case "accepted":
		return "approved"
	case "needs_review":
		return "ready to approve"
	case "in_progress":
		return "being written"
	}
	return state
}
