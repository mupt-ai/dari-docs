package runner

import (
	"bytes"
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/workspace"
)

const RuntimeSecretsName = "DARI_DOCS_RUNTIME_SECRETS_JSON"

func DefaultFeedbackLLMIDs() []string {
	return []string{"dumb-claude", "medium-claude", "smart-claude", "dumb-gpt", "medium-gpt", "smart-gpt"}
}

//go:embed prompts/*.md
var promptFS embed.FS

var (
	feedbackPromptTemplate = template.Must(template.ParseFS(promptFS, "prompts/feedback.md"))
	editorPromptTemplate   = template.Must(template.ParseFS(promptFS, "prompts/editor.md"))
)

type Config struct {
	RepoRoot       string
	OutDir         string
	APIKey         string
	APIBaseURL     string
	LLMAPIKey      string
	FeedbackLLMIDs []string
	EditorLLMID    string
	FeedbackAgent  string
	EditorAgent    string
	Tasks          []string
	LiveVerify     bool
	RuntimeSecrets map[string]string
	Parallel       int
	Apply          bool
	SkipEditor     bool
	Timeout        time.Duration
	BundleOptions  bundle.CreateOptions
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
	b, err := bundle.CreateWithOptions(cfg.RepoRoot, bundlePath, cfg.BundleOptions)
	if err != nil {
		return Result{}, err
	}
	bundle.WriteSummary(os.Stderr, b)

	up, err := client.UploadFile(ctx, bundlePath)
	if err != nil {
		return Result{}, fmt.Errorf("upload docs bundle: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Uploaded docs bundle: %s\n", up.ID)

	baseSessionReq := dari.CreateSessionRequest{LLMAPIKey: cfg.LLMAPIKey}
	if cfg.LiveVerify && len(cfg.RuntimeSecrets) > 0 {
		secretJSON, _ := json.Marshal(cfg.RuntimeSecrets)
		baseSessionReq.Secrets = map[string]string{RuntimeSecretsName: string(secretJSON)}
	}
	feedbackSessionReq := baseSessionReq
	editorSessionReq := baseSessionReq
	editorSessionReq.LLMID = cfg.EditorLLMID

	reports, err := runFeedback(ctx, client, cfg, feedbackSessionReq, up.ID, b)
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

	editorSession, err := runEditor(ctx, client, cfg, editorSessionReq, up.ID, reports)
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
	res.UpdatedDir, err = workspace.UpdatedRoot(extractDir)
	if err != nil {
		return res, err
	}
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
		idx       int
		taskIndex int
		task      string
		llmID     string
	}
	type out struct {
		idx    int
		report string
		err    error
	}
	llmIDs := cfg.FeedbackLLMIDs
	if len(llmIDs) == 0 {
		llmIDs = DefaultFeedbackLLMIDs()
	}
	total := len(cfg.Tasks) * len(llmIDs)
	jobs := make(chan job)
	outs := make(chan out)
	workers := cfg.Parallel
	if workers > total {
		workers = total
	}
	var wg sync.WaitGroup
	for w := 0; w < workers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := range jobs {
				req := sessionReq
				req.LLMID = j.llmID
				report, err := oneFeedback(ctx, client, cfg, req, fileID, b, j.idx, j.taskIndex, j.task, j.llmID, total)
				outs <- out{idx: j.idx, report: report, err: err}
			}
		}()
	}
	go func() {
		idx := 0
		for i, t := range cfg.Tasks {
			for _, llmID := range llmIDs {
				jobs <- job{idx: idx, taskIndex: i, task: t, llmID: llmID}
				idx++
			}
		}
		close(jobs)
		wg.Wait()
		close(outs)
	}()
	reports := make([]string, total)
	for o := range outs {
		if o.err != nil {
			return nil, o.err
		}
		reports[o.idx] = o.report
	}
	return reports, nil
}

func oneFeedback(ctx context.Context, client *dari.Client, cfg Config, sessionReq dari.CreateSessionRequest, fileID string, b bundle.Result, idx int, taskIndex int, task string, llmID string, total int) (string, error) {
	s, err := client.CreateSession(ctx, cfg.FeedbackAgent, sessionReq)
	if err != nil {
		return "", fmt.Errorf("create feedback session %d: %w", idx+1, err)
	}
	label := ""
	if llmID != "" {
		label = " llm=" + llmID
	}
	fmt.Fprintf(os.Stderr, "Feedback session %d/%d%s: %s\n", idx+1, total, label, s.ID)
	prompt := FeedbackPrompt(task, b, cfg.LiveVerify, cfg.RuntimeSecrets)
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
	if llmID != "" || len(cfg.Tasks) > 1 {
		header := fmt.Sprintf("Task index: %d", taskIndex+1)
		if llmID != "" {
			header += "\nTester LLM: " + llmID
		}
		report = header + "\n\n" + report
	}
	filename := fmt.Sprintf("feedback-%03d.md", idx+1)
	if llmID != "" {
		filename = fmt.Sprintf("feedback-%03d-%s.md", idx+1, safeFilenamePart(llmID))
	}
	path := filepath.Join(cfg.OutDir, "runs", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(report+"\n"), 0o644); err != nil {
		return "", err
	}
	return report, nil
}

func safeFilenamePart(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return "default"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	return b.String()
}

func FeedbackPrompt(task string, b bundle.Result, live bool, secrets map[string]string) string {
	var names []string
	for k := range secrets {
		names = append(names, k)
	}
	liveText := "Live verification is disabled unless the docs provide a safe no-credential smoke test."
	if live {
		liveText = "Live verification is enabled. Runtime secrets, if present, are provided inside DARI_DOCS_RUNTIME_SECRETS_JSON as JSON. Available secret names: " + strings.Join(names, ", ") + ". Never print values. Only run safe/test-mode/read-only checks unless explicitly instructed otherwise."
	}
	return executePrompt(feedbackPromptTemplate, "feedback.md", map[string]any{
		"Task":      task,
		"FileCount": len(b.Manifest.Files),
		"SHA256":    b.SHA256,
		"LiveText":  liveText,
	})
}

func AggregateFeedback(reports []string) string {
	var sb strings.Builder
	sb.WriteString("# Dari docs aggregate feedback\n\n")
	for i, r := range reports {
		sb.WriteString(fmt.Sprintf("\n\n---\n\n## Run %03d\n\n%s\n", i+1, r))
	}
	return sb.String()
}

func writeAggregate(outDir string, reports []string) error {
	return os.WriteFile(filepath.Join(outDir, "aggregate-feedback.md"), []byte(AggregateFeedback(reports)), 0o644)
}

func runEditor(ctx context.Context, client *dari.Client, cfg Config, sessionReq dari.CreateSessionRequest, fileID string, reports []string) (string, error) {
	s, err := client.CreateSession(ctx, cfg.EditorAgent, sessionReq)
	if err != nil {
		return "", fmt.Errorf("create editor session: %w", err)
	}
	fmt.Fprintf(os.Stderr, "Editor session: %s\n", s.ID)
	prompt := EditorPrompt(reports)
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

func EditorPrompt(reports []string) string {
	var feedback strings.Builder
	for i, r := range reports {
		feedback.WriteString(fmt.Sprintf("\n\n---\n\n## Feedback run %03d\n\n%s\n", i+1, r))
	}
	return executePrompt(editorPromptTemplate, "editor.md", map[string]any{
		"Feedback": feedback.String(),
	})
}

func executePrompt(t *template.Template, name string, data any) string {
	var buf bytes.Buffer
	if err := t.ExecuteTemplate(&buf, name, data); err != nil {
		panic(fmt.Sprintf("render prompt template %s: %v", name, err))
	}
	return strings.TrimSuffix(buf.String(), "\n")
}
