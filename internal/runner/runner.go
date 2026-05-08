package runner

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/workspace"
)

const RuntimeSecretsName = "DARI_DOCS_RUNTIME_SECRETS_JSON"

type Config struct {
	RepoRoot       string
	OutDir         string
	APIBaseURL     string
	APIKey         string
	LLMAPIKey      string
	FeedbackAgent  string
	EditorAgent    string
	Tasks          []string
	LiveVerify     bool
	RuntimeSecrets map[string]string
	Parallel       int
	Apply          bool
	SkipEditor     bool
	Timeout        time.Duration
}

type Result struct {
	BundlePath      string
	BundleFileID    string
	FeedbackReports []string
	EditorSessionID string
	UpdatedZipPath  string
	UpdatedDir      string
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.OutDir == "" {
		cfg.OutDir = filepath.Join(cfg.RepoRoot, ".dari-docs")
	}
	if cfg.Parallel <= 0 {
		cfg.Parallel = 4
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 10 * time.Minute
	}
	if len(cfg.Tasks) == 0 {
		return Result{}, fmt.Errorf("at least one --task or --tasks-file entry is required")
	}
	if cfg.APIKey == "" {
		return Result{}, fmt.Errorf("Dari API key is required")
	}
	if cfg.FeedbackAgent == "" {
		return Result{}, fmt.Errorf("feedback agent id is required")
	}
	if cfg.EditorAgent == "" && !cfg.SkipEditor {
		return Result{}, fmt.Errorf("editor agent id is required")
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return Result{}, err
	}

	client := dari.New(cfg.APIBaseURL, cfg.APIKey)
	bundlePath := filepath.Join(cfg.OutDir, "input-docs-bundle.tar.gz")
	b, err := bundle.Create(cfg.RepoRoot, bundlePath)
	if err != nil {
		return Result{}, err
	}
	fmt.Fprintf(os.Stderr, "Built docs bundle: %s (%d files, %d bytes)\n", bundlePath, len(b.Manifest.Files), b.Bytes)

	up, err := client.UploadFile(ctx, bundlePath)
	if err != nil {
		return Result{}, fmt.Errorf("upload docs bundle: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Uploaded docs bundle: %s\n", up.ID)

	sessionReq := dari.CreateSessionRequest{LLMAPIKey: cfg.LLMAPIKey}
	if cfg.LiveVerify && len(cfg.RuntimeSecrets) > 0 {
		secretJSON, _ := json.Marshal(cfg.RuntimeSecrets)
		sessionReq.Secrets = map[string]string{RuntimeSecretsName: string(secretJSON)}
	}

	reports, err := runFeedback(ctx, client, cfg, sessionReq, up.ID, b)
	if err != nil {
		return Result{}, err
	}
	if err := writeAggregate(cfg.OutDir, reports); err != nil {
		return Result{}, err
	}
	res := Result{BundlePath: bundlePath, BundleFileID: up.ID, FeedbackReports: reports}
	if cfg.SkipEditor {
		return res, nil
	}

	editorSession, err := runEditor(ctx, client, cfg, sessionReq, up.ID, reports)
	if err != nil {
		return res, err
	}
	res.EditorSessionID = editorSession
	zipPath := filepath.Join(cfg.OutDir, "updated-docs-workspace.zip")
	if err := client.DownloadWorkspaceZip(ctx, editorSession, []string{"updated-docs"}, zipPath); err != nil {
		return res, fmt.Errorf("download editor workspace: %w", err)
	}
	res.UpdatedZipPath = zipPath
	extractDir := filepath.Join(cfg.OutDir, "updated")
	_ = os.RemoveAll(extractDir)
	if err := dari.ExtractZip(zipPath, extractDir); err != nil {
		return res, err
	}
	res.UpdatedDir = workspace.UpdatedRoot(extractDir)
	fmt.Fprintf(os.Stderr, "Downloaded updated docs to: %s\n", res.UpdatedDir)
	if cfg.Apply {
		if err := workspace.CopyTree(res.UpdatedDir, cfg.RepoRoot); err != nil {
			return res, fmt.Errorf("apply updated docs: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Applied updated docs into %s\n", cfg.RepoRoot)
	}
	return res, nil
}

func runFeedback(ctx context.Context, client *dari.Client, cfg Config, sessionReq dari.CreateSessionRequest, fileID string, b bundle.Result) ([]string, error) {
	type job struct {
		idx  int
		task string
	}
	type out struct {
		idx    int
		report string
		err    error
	}
	jobs := make(chan job)
	outs := make(chan out)
	workers := cfg.Parallel
	if workers > len(cfg.Tasks) {
		workers = len(cfg.Tasks)
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				report, err := oneFeedback(ctx, client, cfg, sessionReq, fileID, b, j.idx, j.task)
				outs <- out{idx: j.idx, report: report, err: err}
			}
		}()
	}
	go func() {
		for i, t := range cfg.Tasks {
			jobs <- job{i, t}
		}
		close(jobs)
		wg.Wait()
		close(outs)
	}()
	reports := make([]string, len(cfg.Tasks))
	for o := range outs {
		if o.err != nil {
			return nil, o.err
		}
		reports[o.idx] = o.report
	}
	return reports, nil
}

func oneFeedback(ctx context.Context, client *dari.Client, cfg Config, sessionReq dari.CreateSessionRequest, fileID string, b bundle.Result, idx int, task string) (string, error) {
	s, err := client.CreateSession(ctx, cfg.FeedbackAgent, sessionReq)
	if err != nil {
		return "", fmt.Errorf("create feedback session %d: %w", idx+1, err)
	}
	fmt.Fprintf(os.Stderr, "Feedback session %d/%d: %s\n", idx+1, len(cfg.Tasks), s.ID)
	prompt := feedbackPrompt(task, b, cfg.LiveVerify, cfg.RuntimeSecrets)
	resp, err := client.SendUserMessage(ctx, s.ID, []dari.ContentBlock{dari.TextBlock(prompt), dari.FileBlock(fileID)})
	if err != nil {
		return "", fmt.Errorf("send feedback message %d: %w", idx+1, err)
	}
	_ = resp
	final, err := client.WaitForCompletion(ctx, s.ID, 5*time.Second, cfg.Timeout)
	if err != nil {
		return "", err
	}
	if final.LastMessageStatus != nil && *final.LastMessageStatus == "failed" {
		return "", fmt.Errorf("feedback session %s failed", s.ID)
	}
	tr, err := client.GetTranscript(ctx, s.ID)
	if err != nil {
		return "", err
	}
	report := dari.FinalAssistantText(tr)
	path := filepath.Join(cfg.OutDir, "runs", fmt.Sprintf("feedback-%03d.md", idx+1))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(report+"\n"), 0o644); err != nil {
		return "", err
	}
	return report, nil
}

func feedbackPrompt(task string, b bundle.Result, live bool, secrets map[string]string) string {
	var names []string
	for k := range secrets {
		names = append(names, k)
	}
	liveText := "Live verification is disabled unless the docs provide a safe no-credential smoke test."
	if live {
		liveText = "Live verification is enabled. Runtime secrets, if present, are provided inside DARI_DOCS_RUNTIME_SECRETS_JSON as JSON. Available secret names: " + strings.Join(names, ", ") + ". Never print values. Only run safe/test-mode/read-only checks unless explicitly instructed otherwise."
	}
	return fmt.Sprintf(`You are a developer trying to complete this task using the attached docs:

%s

The attached file is input-docs-bundle.tar.gz. It contains manifest.json and docs files under files/ with repo-relative paths. Extract it, search/read the relevant docs, and actually try the task in /workspace/attempt.

Bundle summary: %d files, sha256 %s.

%s

Keep feedback brief. Report what you tried, whether it worked, where you got stuck, and the smallest docs changes that would have helped. Do not score the docs.`, task, len(b.Manifest.Files), b.SHA256, liveText)
}

func writeAggregate(outDir string, reports []string) error {
	var sb strings.Builder
	sb.WriteString("# Dari docs aggregate feedback\n\n")
	for i, r := range reports {
		sb.WriteString(fmt.Sprintf("\n\n---\n\n## Run %03d\n\n%s\n", i+1, r))
	}
	return os.WriteFile(filepath.Join(outDir, "aggregate-feedback.md"), []byte(sb.String()), 0o644)
}

func runEditor(ctx context.Context, client *dari.Client, cfg Config, sessionReq dari.CreateSessionRequest, fileID string, reports []string) (string, error) {
	s, err := client.CreateSession(ctx, cfg.EditorAgent, sessionReq)
	if err != nil {
		return "", fmt.Errorf("create editor session: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Editor session: %s\n", s.ID)
	prompt := editorPrompt(reports)
	if _, err := client.SendUserMessage(ctx, s.ID, []dari.ContentBlock{dari.TextBlock(prompt), dari.FileBlock(fileID)}); err != nil {
		return "", fmt.Errorf("send editor message: %w", err)
	}
	final, err := client.WaitForCompletion(ctx, s.ID, 5*time.Second, cfg.Timeout)
	if err != nil {
		return "", err
	}
	if final.LastMessageStatus != nil && *final.LastMessageStatus == "failed" {
		return "", fmt.Errorf("editor session %s failed", s.ID)
	}
	tr, err := client.GetTranscript(ctx, s.ID)
	if err == nil {
		_ = os.WriteFile(filepath.Join(cfg.OutDir, "editor-output.md"), []byte(dari.FinalAssistantText(tr)+"\n"), 0o644)
	}
	return s.ID, nil
}

func editorPrompt(reports []string) string {
	var sb strings.Builder
	sb.WriteString(`You are running as the remote docs editor for dari-docs.

Attached is the original docs bundle, input-docs-bundle.tar.gz. It contains manifest.json and repo-relative files under files/.

The feedback below comes from lightweight user-test agents that attempted tasks from the docs. Treat it as user research: fix the concrete blockers and confusing spots, not every possible documentation issue.

Required output contract:
1. Locate the attached tar.gz in the session workspace. Use shell tools to inspect the workspace if needed.
2. Extract it into /workspace/input-docs.
3. Copy /workspace/input-docs/files to /workspace/updated-docs/files, preserving repo-relative paths.
4. Apply documentation improvements from the aggregate feedback below by editing files inside /workspace/updated-docs/files only.
5. Write /workspace/updated-docs/CHANGELOG.md summarizing changed files and unresolved items.
6. Ensure /workspace/updated-docs exists before finishing. The local CLI will download that directory with GET /v1/sessions/{session_id}/workspace.zip?path=updated-docs.

Do not invent product facts. If source truth is missing, leave a clear TODO(owner) note or list it in CHANGELOG.md rather than fabricating schemas or behavior.

Aggregate feedback:
`)
	for i, r := range reports {
		sb.WriteString(fmt.Sprintf("\n\n---\n\n## Feedback run %03d\n\n%s\n", i+1, r))
	}
	return sb.String()
}
