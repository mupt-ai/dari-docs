package main

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/agenttemplates"
	appconfig "github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/runner"
)

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "dari-docs: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cmd := "optimize"
	args := os.Args[1:]
	if len(args) > 0 {
		switch args[0] {
		case "init", "optimize", "check", "help", "-h", "--help":
			cmd = args[0]
			args = args[1:]
		}
	}
	if cmd == "help" || cmd == "-h" || cmd == "--help" {
		usage()
		return nil
	}
	if cmd == "init" {
		return runInit(args)
	}
	return runCheckOrOptimize(cmd, args)
}

func runCheckOrOptimize(cmd string, args []string) error {
	fs := flag.NewFlagSet("dari-docs "+cmd, flag.ExitOnError)
	var tasks repeated
	var taskFiles repeated
	var secretEnvs repeated
	var apiKeyEnv string
	var apiKey string
	var apiBase string
	var feedbackAgent string
	var editorAgent string
	var outDir string
	var parallel int
	var apply bool
	var liveVerify bool
	var timeoutMinutes int
	fs.Var(&tasks, "task", "implementation task/prompt to test; repeatable")
	fs.Var(&taskFiles, "tasks-file", "file containing tasks, one per paragraph or bullet; repeatable")
	fs.Var(&secretEnvs, "secret-env", "runtime product/API secret env var to pass to sessions; repeatable")
	fs.StringVar(&apiKeyEnv, "api-key-env", "DARI_API_KEY", "env var containing Dari API key")
	fs.StringVar(&apiKey, "api-key", "", "Dari API key (prefer --api-key-env)")
	fs.StringVar(&apiBase, "api-base", "https://api.dari.dev", "Dari API base URL")
	fs.StringVar(&feedbackAgent, "feedback-agent", "", "Dari docs user-test agent ID (defaults to .dari-docs/config.json)")
	fs.StringVar(&editorAgent, "editor-agent", "", "Dari docs editor agent ID (defaults to .dari-docs/config.json)")
	fs.StringVar(&outDir, "out", "", "output directory (default: <repo>/.dari-docs)")
	fs.IntVar(&parallel, "parallel", 4, "number of feedback sessions to run concurrently")
	fs.BoolVar(&apply, "apply", false, "copy updated docs back into the repo after downloading")
	fs.BoolVar(&liveVerify, "live-verify", false, "allow agents to run safe live verification using provided runtime secrets")
	fs.IntVar(&timeoutMinutes, "timeout-minutes", 15, "per-session timeout in minutes")
	if cmd == "check" {
		fs.Bool("remote-editor", false, "ignored for check")
	}
	repoArg := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repoArg = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}

	repo := "."
	if repoArg != "" {
		repo = repoArg
	} else if fs.NArg() > 0 {
		repo = fs.Arg(0)
	}
	absRepo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	if outDir == "" {
		outDir = filepath.Join(absRepo, ".dari-docs")
	}
	if c, ok, err := appconfig.Load(absRepo); err != nil {
		return err
	} else if ok {
		if feedbackAgent == "" {
			feedbackAgent = c.TesterAgentID
		}
		if editorAgent == "" {
			editorAgent = c.EditorAgentID
		}
	}
	if feedbackAgent == "" || (cmd != "check" && editorAgent == "") {
		return fmt.Errorf("missing agent IDs; run `dari-docs init --deploy` in this repo, or pass --feedback-agent and --editor-agent")
	}

	allTasks := append([]string{}, tasks...)
	for _, p := range taskFiles {
		more, err := readTasksFile(p)
		if err != nil {
			return err
		}
		allTasks = append(allTasks, more...)
	}
	if len(allTasks) == 0 {
		return fmt.Errorf("provide at least one --task or --tasks-file")
	}
	if apiKey == "" && apiKeyEnv != "" {
		apiKey = os.Getenv(apiKeyEnv)
	}
	if apiKey == "" {
		return fmt.Errorf("missing Dari API key; set %s or pass --api-key", apiKeyEnv)
	}
	secrets := map[string]string{}
	for _, name := range secretEnvs {
		val := os.Getenv(name)
		if val == "" {
			return fmt.Errorf("--secret-env %s requested but env var is empty", name)
		}
		secrets[name] = val
	}

	cfg := runner.Config{
		RepoRoot: absRepo, OutDir: outDir, APIBaseURL: apiBase, APIKey: apiKey,
		FeedbackAgent: feedbackAgent, EditorAgent: editorAgent, Tasks: allTasks, LiveVerify: liveVerify,
		RuntimeSecrets: secrets, Parallel: parallel, Apply: apply, SkipEditor: cmd == "check", Timeout: time.Duration(timeoutMinutes) * time.Minute,
	}
	res, err := runner.Run(context.Background(), cfg)
	if err != nil {
		return err
	}
	fmt.Println("\nDone.")
	fmt.Printf("Bundle: %s\n", res.BundlePath)
	fmt.Printf("Feedback: %s\n", filepath.Join(outDir, "aggregate-feedback.md"))
	if cmd != "check" {
		fmt.Printf("Editor session: %s\n", res.EditorSessionID)
		fmt.Printf("Updated docs: %s\n", res.UpdatedDir)
		if !apply {
			fmt.Printf("Review and apply manually, or rerun with --apply.\n")
		}
	}
	return nil
}

func runInit(args []string) error {
	fs := flag.NewFlagSet("dari-docs init", flag.ExitOnError)
	var deploy bool
	var apiKeyEnv, apiKey, apiBase, llmAPIKeySecret, agentsDir string
	fs.BoolVar(&deploy, "deploy", false, "deploy bundled agents into the current Dari org")
	fs.StringVar(&apiKeyEnv, "api-key-env", "DARI_API_KEY", "env var containing Dari API key for deploy")
	fs.StringVar(&apiKey, "api-key", "", "Dari API key for deploy (prefer --api-key-env)")
	fs.StringVar(&apiBase, "api-base", "https://api.dari.dev", "Dari API base URL for deploy")
	fs.StringVar(&llmAPIKeySecret, "llm-api-key-secret", "", "optional stored Dari credential name for BYOK LLM at agent publish time; omit to use platform-managed LLM")
	fs.StringVar(&agentsDir, "agents-dir", "", "where to extract agent templates (default: <repo>/.dari-docs/agents)")
	repoArg := "."
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		repoArg = args[0]
		args = args[1:]
	}
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		repoArg = fs.Arg(0)
	}
	absRepo, err := filepath.Abs(repoArg)
	if err != nil {
		return err
	}
	if agentsDir == "" {
		agentsDir = filepath.Join(absRepo, ".dari-docs", "agents")
	}
	if err := agenttemplates.Extract(agentsDir); err != nil {
		return err
	}
	fmt.Printf("Extracted bundled agents to %s\n", agentsDir)

	cfg := appconfig.Config{AgentsDir: agentsDir, LLMMode: "platform-managed", LLMAPIKeySecret: llmAPIKeySecret}
	if llmAPIKeySecret != "" {
		cfg.LLMMode = "byok-publish-time"
		if err := setLLMAPIKeySecret(filepath.Join(agentsDir, "docs-user-tester-agent", "dari.yml"), llmAPIKeySecret); err != nil {
			return err
		}
		if err := setLLMAPIKeySecret(filepath.Join(agentsDir, "docs-editor-agent", "dari.yml"), llmAPIKeySecret); err != nil {
			return err
		}
	}
	if deploy {
		if apiKey == "" && apiKeyEnv != "" {
			apiKey = os.Getenv(apiKeyEnv)
		}
		if apiKey == "" {
			return fmt.Errorf("missing Dari API key for deploy; set %s or pass --api-key", apiKeyEnv)
		}
		env := append(os.Environ(), "DARI_API_URL="+apiBase, "DARI_API_KEY="+apiKey)
		ensureCredential(env, "DARI_DOCS_RUNTIME_SECRETS_JSON", "{}")
		testerID, err := deployAgent(env, filepath.Join(agentsDir, "docs-user-tester-agent"))
		if err != nil {
			return err
		}
		editorID, err := deployAgent(env, filepath.Join(agentsDir, "docs-editor-agent"))
		if err != nil {
			return err
		}
		cfg.TesterAgentID = testerID
		cfg.EditorAgentID = editorID
		fmt.Printf("Deployed tester agent: %s\n", testerID)
		fmt.Printf("Deployed editor agent: %s\n", editorID)
	}
	if err := appconfig.Save(absRepo, cfg); err != nil {
		return err
	}
	fmt.Printf("Wrote %s\n", appconfig.Path(absRepo))
	if !deploy {
		fmt.Println("Run `dari-docs init --deploy` to deploy these agents into your Dari org.")
	}
	return nil
}

func ensureCredential(env []string, name, value string) {
	cmd := exec.Command("dari", "credentials", "add", name, value)
	cmd.Env = env
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "warning: could not create credential %s (it may already exist): %s\n", name, strings.TrimSpace(out.String()))
	}
}

func deployAgent(env []string, dir string) (string, error) {
	cmd := exec.Command("dari", "deploy", "--quiet", ".")
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("deploy %s: %w\n%s", dir, err, stderr.String()+stdout.String())
	}
	var resp struct {
		AgentID string `json:"agent_id"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil || resp.AgentID == "" {
		return "", fmt.Errorf("deploy %s: could not parse agent_id from output: %s", dir, stdout.String())
	}
	return resp.AgentID, nil
}

func setLLMAPIKeySecret(path, secret string) error {
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	text := string(b)
	if strings.Contains(text, "api_key_secret:") {
		return nil
	}
	old := "llm:\n  model: openai/gpt-5.5\n"
	newText := "llm:\n  model: openai/gpt-5.5\n  api_key_secret: " + secret + "\n"
	if !strings.Contains(text, old) {
		return fmt.Errorf("could not find llm block in %s", path)
	}
	return os.WriteFile(path, []byte(strings.Replace(text, old, newText, 1)), 0o644)
}

func readTasksFile(path string) ([]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var tasks []string
	var cur []string
	s := bufio.NewScanner(f)
	for s.Scan() {
		line := strings.TrimSpace(s.Text())
		if line == "" {
			if len(cur) > 0 {
				tasks = append(tasks, strings.Join(cur, "\n"))
				cur = nil
			}
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimPrefix(line, "* ")
		cur = append(cur, line)
	}
	if len(cur) > 0 {
		tasks = append(tasks, strings.Join(cur, "\n"))
	}
	return tasks, s.Err()
}

func usage() {
	fmt.Print(`dari-docs runs lightweight user-test sessions, feeds the results into a hosted editor, and pulls updated docs back to your repo.

Usage:
  dari-docs init [repo] --deploy
  dari-docs optimize [repo] --task "Implement auth" [--task "Set up webhooks"] [flags]
  dari-docs check [repo] --task "Implement auth" [flags]

Setup:
  export DARI_API_KEY=...
  dari-docs init --deploy

Important flags:
  --task TEXT                 task/prompt to test; repeatable
  --tasks-file PATH           tasks file; repeatable
  --live-verify               permit safe credential-dependent checks
  --secret-env NAME           pass runtime product/API key from env var; repeatable
  --apply                     copy downloaded updated docs back into repo
  --parallel N                tester sessions in parallel

Outputs:
  .dari-docs/config.json
  .dari-docs/agents/
  .dari-docs/input-docs-bundle.tar.gz
  .dari-docs/runs/feedback-*.md
  .dari-docs/aggregate-feedback.md
  .dari-docs/updated-docs-workspace.zip
  .dari-docs/updated/
`)
}
