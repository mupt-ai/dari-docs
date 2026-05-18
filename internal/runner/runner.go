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
	"text/template"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/workspace"
)

const RuntimeSecretsName = "DARI_DOCS_RUNTIME_SECRETS_JSON"

const maxSessionBatchItems = 100

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

	var sessionSecrets map[string]string
	if cfg.LiveVerify && len(cfg.RuntimeSecrets) > 0 {
		secretJSON, err := json.Marshal(cfg.RuntimeSecrets)
		if err != nil {
			return Result{}, fmt.Errorf("encode runtime secrets: %w", err)
		}
		sessionSecrets = map[string]string{RuntimeSecretsName: string(secretJSON)}
	}

	reports, err := runFeedback(ctx, client, cfg, sessionSecrets, up.ID, b)
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

	editorSession, err := runEditor(ctx, client, cfg, sessionSecrets, up.ID, reports)
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

func runFeedback(
	ctx context.Context,
	client *dari.Client,
	cfg Config,
	secrets map[string]string,
	fileID string,
	b bundle.Result,
) ([]string, error) {
	items := feedbackItems(cfg)
	reports := make([]string, len(items))
	for _, chunk := range feedbackChunks(items, cfg.Parallel) {
		batchReq := dari.CreateSessionBatchRequest{
			IdempotencyKey: batchIdempotencyKey("dari-docs-feedback", b.SHA256),
			Items:          make([]dari.CreateSessionBatchItem, 0, len(chunk)),
		}
		for _, item := range chunk {
			metadata := map[string]string{
				"kind":       "tester",
				"task_index": fmt.Sprintf("%d", item.taskIndex+1),
				"item_index": fmt.Sprintf("%d", item.idx+1),
			}
			if item.llmID != "" {
				metadata["llm_id"] = item.llmID
			}
			prompt := FeedbackPrompt(item.task, b, cfg.LiveVerify, cfg.RuntimeSecrets)
			batchReq.Items = append(batchReq.Items, dari.CreateSessionBatchItem{
				AgentID:  cfg.FeedbackAgent,
				LLMID:    item.llmID,
				Metadata: metadata,
				Secrets:  secrets,
				Message:  dari.CreateSessionBatchMessage{Content: []dari.ContentBlock{dari.TextBlock(prompt), dari.FileBlock(fileID)}},
			})
		}
		batch, err := client.CreateSessionBatch(ctx, batchReq)
		if err != nil {
			return nil, fmt.Errorf("create feedback session batch: %w", err)
		}
		fmt.Fprintf(os.Stderr, "Feedback batch: %s (%d sessions)\n", batch.BatchID, len(batch.Sessions))
		if err := printFeedbackBatchSessions(batch, chunk, len(items)); err != nil {
			return nil, err
		}
		final, err := client.WaitForBatchCompletion(ctx, batch.BatchID, 5*time.Second, cfg.Timeout)
		if err != nil {
			return nil, err
		}
		if err := ensureBatchSucceeded("feedback", final, chunk); err != nil {
			return nil, err
		}
		if len(final.Sessions) != len(chunk) {
			return nil, fmt.Errorf("feedback batch returned %d sessions, want %d", len(final.Sessions), len(chunk))
		}
		for _, session := range final.Sessions {
			if session.Index < 0 || session.Index >= len(chunk) {
				return nil, fmt.Errorf("feedback batch returned invalid item index %d", session.Index)
			}
			if session.SessionID == "" {
				return nil, fmt.Errorf("feedback batch item %d did not return a session", session.Index+1)
			}
			item := chunk[session.Index]
			tr, err := client.GetTranscript(ctx, session.SessionID)
			if err != nil {
				return nil, err
			}
			report := dari.FinalAssistantText(tr)
			if err := writeFeedbackReport(cfg, item.idx, item.taskIndex, item.llmID, len(cfg.Tasks), report); err != nil {
				return nil, err
			}
			reports[item.idx] = formatFeedbackReport(item.taskIndex, item.llmID, len(cfg.Tasks), report)
		}
	}
	return reports, nil
}

type feedbackItem struct {
	idx       int
	taskIndex int
	task      string
	llmID     string
}

func feedbackItems(cfg Config) []feedbackItem {
	llmIDs := cfg.FeedbackLLMIDs
	if len(llmIDs) == 0 {
		llmIDs = DefaultFeedbackLLMIDs()
	}
	items := make([]feedbackItem, 0, len(cfg.Tasks)*len(llmIDs))
	idx := 0
	for i, task := range cfg.Tasks {
		for _, llmID := range llmIDs {
			items = append(items, feedbackItem{idx: idx, taskIndex: i, task: task, llmID: llmID})
			idx++
		}
	}
	return items
}

func feedbackChunks(items []feedbackItem, size int) [][]feedbackItem {
	if size <= 0 || size > len(items) {
		size = len(items)
	}
	if size > maxSessionBatchItems {
		size = maxSessionBatchItems
	}
	chunks := make([][]feedbackItem, 0, (len(items)+size-1)/size)
	for start := 0; start < len(items); start += size {
		end := start + size
		if end > len(items) {
			end = len(items)
		}
		chunks = append(chunks, items[start:end])
	}
	return chunks
}

func printFeedbackBatchSessions(batch dari.SessionBatch, items []feedbackItem, total int) error {
	for _, session := range batch.Sessions {
		if session.Index < 0 || session.Index >= len(items) {
			return fmt.Errorf("feedback batch returned invalid item index %d", session.Index)
		}
		item := items[session.Index]
		if session.Status == "failed" {
			return fmt.Errorf("feedback batch item %d failed during creation: %s", session.Index+1, session.Error)
		}
		if session.SessionID == "" {
			return fmt.Errorf("feedback batch item %d did not return a session", session.Index+1)
		}
		label := ""
		if item.llmID != "" {
			label = " llm=" + item.llmID
		}
		fmt.Fprintf(os.Stderr, "Feedback session %d/%d%s: %s\n", item.idx+1, total, label, session.SessionID)
	}
	return nil
}

func ensureBatchSucceeded(kind string, batch dari.SessionBatch, items []feedbackItem) error {
	if batch.Status == "completed" {
		return nil
	}
	for _, session := range batch.Sessions {
		if session.Status == "failed" || session.LastMessageStatus == "failed" || session.Error != "" {
			idx := session.Index + 1
			if session.Index >= 0 && session.Index < len(items) {
				idx = items[session.Index].idx + 1
			}
			if session.Error != "" {
				return fmt.Errorf("%s batch item %d failed: %s", kind, idx, session.Error)
			}
			return fmt.Errorf("%s batch item %d failed", kind, idx)
		}
	}
	return fmt.Errorf("%s batch %s ended with status %q", kind, batch.BatchID, batch.Status)
}

func batchIdempotencyKey(prefix string, bundleSHA string) string {
	short := bundleSHA
	if len(short) > 16 {
		short = short[:16]
	}
	key := fmt.Sprintf("%s-%d-%s", prefix, time.Now().UnixNano(), short)
	if len(key) > 128 {
		return key[:128]
	}
	return key
}

func formatFeedbackReport(taskIndex int, llmID string, taskCount int, report string) string {
	if llmID == "" && taskCount <= 1 {
		return report
	}
	header := fmt.Sprintf("Task index: %d", taskIndex+1)
	if llmID != "" {
		header += "\nTester LLM: " + llmID
	}
	return header + "\n\n" + report
}

func writeFeedbackReport(cfg Config, idx int, taskIndex int, llmID string, taskCount int, report string) error {
	report = formatFeedbackReport(taskIndex, llmID, taskCount, report)
	filename := fmt.Sprintf("feedback-%03d.md", idx+1)
	if llmID != "" {
		filename = fmt.Sprintf("feedback-%03d-%s.md", idx+1, safeFilenamePart(llmID))
	}
	path := filepath.Join(cfg.OutDir, "runs", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("create feedback report directory: %w", err)
	}
	if err := os.WriteFile(path, []byte(report+"\n"), 0o644); err != nil {
		return fmt.Errorf("write feedback report: %w", err)
	}
	return nil
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
		liveText = strings.Join([]string{
			"Live verification is enabled.",
			"Runtime secrets, if present, are provided inside DARI_DOCS_RUNTIME_SECRETS_JSON as JSON.",
			"Available secret names: " + strings.Join(names, ", ") + ".",
			"Never print values.",
			"Only run safe/test-mode/read-only checks unless explicitly instructed otherwise.",
		}, " ")
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
	path := filepath.Join(outDir, "aggregate-feedback.md")
	if err := os.WriteFile(path, []byte(AggregateFeedback(reports)), 0o644); err != nil {
		return fmt.Errorf("write aggregate feedback: %w", err)
	}
	return nil
}

func runEditor(
	ctx context.Context,
	client *dari.Client,
	cfg Config,
	secrets map[string]string,
	fileID string,
	reports []string,
) (string, error) {
	prompt := EditorPrompt(reports)
	metadata := map[string]string{"kind": "editor", "item_index": "1"}
	batch, err := client.CreateSessionBatch(ctx, dari.CreateSessionBatchRequest{
		IdempotencyKey: batchIdempotencyKey("dari-docs-editor", ""),
		Items: []dari.CreateSessionBatchItem{{
			AgentID:  cfg.EditorAgent,
			LLMID:    cfg.EditorLLMID,
			Metadata: metadata,
			Secrets:  secrets,
			Message:  dari.CreateSessionBatchMessage{Content: []dari.ContentBlock{dari.TextBlock(prompt), dari.FileBlock(fileID)}},
		}},
	})
	if err != nil {
		return "", fmt.Errorf("create editor session batch: %w", err)
	}
	if len(batch.Sessions) != 1 || batch.Sessions[0].SessionID == "" {
		return "", fmt.Errorf("editor batch %s did not return a session", batch.BatchID)
	}
	sessionID := batch.Sessions[0].SessionID
	fmt.Fprintf(os.Stderr, "Editor batch: %s\n", batch.BatchID)
	fmt.Fprintf(os.Stderr, "Editor session: %s\n", sessionID)
	final, err := client.WaitForBatchCompletion(ctx, batch.BatchID, 5*time.Second, cfg.Timeout)
	if err != nil {
		return "", err
	}
	if final.Status != "completed" {
		for _, session := range final.Sessions {
			if session.Error != "" {
				return "", fmt.Errorf("editor session %s failed: %s", session.SessionID, session.Error)
			}
			if session.Status == "failed" || session.LastMessageStatus == "failed" {
				return "", fmt.Errorf("editor session %s failed", session.SessionID)
			}
		}
		return "", fmt.Errorf("editor batch %s ended with status %q", final.BatchID, final.Status)
	}
	tr, err := client.GetTranscript(ctx, sessionID)
	if err != nil {
		return "", fmt.Errorf("get editor transcript: %w", err)
	}
	if err := os.WriteFile(filepath.Join(cfg.OutDir, "editor-output.md"), []byte(dari.FinalAssistantText(tr)+"\n"), 0o644); err != nil {
		return "", fmt.Errorf("write editor output: %w", err)
	}
	return sessionID, nil
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
