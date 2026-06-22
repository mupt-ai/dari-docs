package flueclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/mupt-ai/dari-docs/internal/bundle"
	"github.com/mupt-ai/dari-docs/internal/runner"
	"github.com/mupt-ai/dari-docs/internal/runtimeenv"
	"github.com/mupt-ai/dari-docs/internal/workspace"
)

type Config struct {
	RepoRoot       string
	OutDir         string
	TesterURL      string
	EditorURL      string
	Tasks          []string
	FeedbackModels []string
	EditorModel    string
	LiveVerify     bool
	RuntimeSecrets map[string]string
	PublicDocURLs  []string
	PublicDocsOnly bool
	SkipEditor     bool
	Timeout        time.Duration
	Parallel       int
	BundleOptions  bundle.CreateOptions
}

type Result struct {
	BundlePath      string
	FeedbackReports []string
	UpdatedDir      string
}

type docFile struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type testerPayload struct {
	Task           string            `json:"task"`
	Model          string            `json:"model,omitempty"`
	Files          []docFile         `json:"files,omitempty"`
	PublicDocURLs  []string          `json:"publicDocUrls,omitempty"`
	LiveVerify     bool              `json:"liveVerify,omitempty"`
	RuntimeSecrets map[string]string `json:"runtimeSecrets,omitempty"`
}

type testerResult struct {
	Feedback string `json:"feedback"`
}

type editorPayload struct {
	Files          []docFile         `json:"files"`
	Feedback       string            `json:"feedback"`
	Model          string            `json:"model,omitempty"`
	LiveVerify     bool              `json:"liveVerify,omitempty"`
	RuntimeSecrets map[string]string `json:"runtimeSecrets,omitempty"`
}

type editorResult struct {
	Files     []docFile `json:"files"`
	Changelog string    `json:"changelog"`
}

type workflowResponse[T any] struct {
	Result T `json:"result"`
}

type httpError struct {
	Status int
	Body   string
}

func (e *httpError) Error() string {
	return fmt.Sprintf("flue workflow http %d: %s", e.Status, e.Body)
}

func Run(ctx context.Context, cfg Config) (Result, error) {
	if cfg.OutDir == "" {
		cfg.OutDir = filepath.Join(cfg.RepoRoot, ".dari-docs")
	}
	if cfg.Timeout == 0 {
		cfg.Timeout = 30 * time.Minute
	}
	if len(cfg.Tasks) == 0 {
		return Result{}, fmt.Errorf("at least one --task or --tasks-file entry is required")
	}
	if strings.TrimSpace(cfg.TesterURL) == "" {
		return Result{}, fmt.Errorf("missing Flue tester URL; pass --tester-url or save it in .dari-docs/config.json")
	}
	if !cfg.SkipEditor && strings.TrimSpace(cfg.EditorURL) == "" {
		return Result{}, fmt.Errorf("missing Flue editor URL; pass --editor-url or save it in .dari-docs/config.json")
	}
	if err := os.MkdirAll(cfg.OutDir, 0o755); err != nil {
		return Result{}, err
	}

	bundlePath := ""
	var b bundle.Result
	var files []docFile
	if cfg.PublicDocsOnly {
		b = publicDocsBundleResult(cfg.PublicDocURLs)
	} else {
		bundlePath = filepath.Join(cfg.OutDir, "input-docs-bundle.tar.gz")
		var err error
		b, err = bundle.CreateWithOptions(cfg.RepoRoot, bundlePath, cfg.BundleOptions)
		if err != nil {
			return Result{}, err
		}
		bundle.WriteSummary(os.Stderr, b)
		files, err = readBundleFiles(cfg.RepoRoot, b.Manifest.Files)
		if err != nil {
			return Result{}, err
		}
	}

	secrets, err := runtimeSecrets(cfg)
	if err != nil {
		return Result{}, err
	}
	feedbackModels := feedbackModelsOrDefault(cfg.FeedbackModels)
	client := &http.Client{Timeout: cfg.Timeout}
	reports, err := runTesterWorkflows(ctx, client, cfg, files, secrets, feedbackModels)
	if err != nil {
		return Result{}, err
	}
	if err := writeAggregate(cfg.OutDir, reports); err != nil {
		return Result{}, err
	}
	res := Result{BundlePath: bundlePath, FeedbackReports: reports}
	if cfg.SkipEditor {
		return res, nil
	}
	aggregate := runner.AggregateFeedback(reports)
	edited, err := callWorkflow[editorResult](ctx, client, cfg.EditorURL, "edit", editorPayload{
		Files:          files,
		Feedback:       aggregate,
		Model:          cfg.EditorModel,
		LiveVerify:     cfg.LiveVerify,
		RuntimeSecrets: secrets,
	})
	if err != nil {
		return res, err
	}
	updatedDir := filepath.Join(cfg.OutDir, "updated")
	_ = os.RemoveAll(updatedDir)
	if err := writeUpdatedFiles(updatedDir, edited); err != nil {
		return res, err
	}
	res.UpdatedDir = updatedDir
	fmt.Fprintf(os.Stderr, "Downloaded updated docs to: %s\n", updatedDir)
	return res, nil
}

type testerRunSpec struct {
	Index     int
	TaskIndex int
	TaskCount int
	Task      string
	Model     string
}

type testerRunOutput struct {
	Index  int
	Report string
	Err    error
}

func runTesterWorkflows(ctx context.Context, client *http.Client, cfg Config, files []docFile, secrets map[string]string, feedbackModels []string) ([]string, error) {
	specs := make([]testerRunSpec, 0, len(cfg.Tasks)*len(feedbackModels))
	for i, task := range cfg.Tasks {
		for _, model := range feedbackModels {
			specs = append(specs, testerRunSpec{
				Index:     len(specs),
				TaskIndex: i,
				TaskCount: len(cfg.Tasks),
				Task:      task,
				Model:     model,
			})
		}
	}
	parallel := cfg.Parallel
	if parallel < 1 {
		parallel = 1
	}
	if parallel > len(specs) {
		parallel = len(specs)
	}
	if len(specs) > 1 {
		fmt.Fprintf(os.Stderr, "Running %d tester workflow(s) with parallel=%d\n", len(specs), parallel)
	}

	jobs := make(chan testerRunSpec)
	results := make(chan testerRunOutput)
	var wg sync.WaitGroup
	wg.Add(parallel)
	for range parallel {
		go func() {
			defer wg.Done()
			for spec := range jobs {
				result, err := callWorkflow[testerResult](ctx, client, cfg.TesterURL, "test", testerPayload{
					Task:           spec.Task,
					Model:          spec.Model,
					Files:          files,
					PublicDocURLs:  cfg.PublicDocURLs,
					LiveVerify:     cfg.LiveVerify,
					RuntimeSecrets: secrets,
				})
				if err != nil {
					results <- testerRunOutput{Index: spec.Index, Err: fmt.Errorf("feedback task %d model %q: %w", spec.TaskIndex+1, displayModel(spec.Model), err)}
					continue
				}
				report := formatFeedbackReport(spec.TaskIndex, spec.TaskCount, spec.Model, result.Feedback)
				if err := writeFeedbackReport(cfg.OutDir, spec.Index, spec.Model, report); err != nil {
					results <- testerRunOutput{Index: spec.Index, Err: err}
					continue
				}
				results <- testerRunOutput{Index: spec.Index, Report: report}
			}
		}()
	}
	go func() {
		defer close(jobs)
		for _, spec := range specs {
			select {
			case jobs <- spec:
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		wg.Wait()
		close(results)
	}()

	reports := make([]string, len(specs))
	var firstErr error
	for result := range results {
		if result.Err != nil {
			if firstErr == nil {
				firstErr = result.Err
			}
			continue
		}
		reports[result.Index] = result.Report
	}
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return reports, nil
}

func callWorkflow[T any](ctx context.Context, client *http.Client, baseURL, workflow string, payload any) (T, error) {
	var zero T
	body, err := json.Marshal(payload)
	if err != nil {
		return zero, err
	}
	u, err := workflowURL(baseURL, workflow)
	if err != nil {
		return zero, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return zero, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(req)
	if err != nil {
		return zero, err
	}
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return zero, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return zero, &httpError{Status: resp.StatusCode, Body: strings.TrimSpace(string(respBody))}
	}
	var out workflowResponse[T]
	if err := json.Unmarshal(respBody, &out); err != nil {
		return zero, fmt.Errorf("decode response: %w; body=%s", err, string(respBody))
	}
	return out.Result, nil
}

func workflowURL(baseURL, workflow string) (string, error) {
	baseURL = strings.TrimSpace(baseURL)
	if baseURL == "" {
		return "", fmt.Errorf("missing Flue URL")
	}
	u, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	u.Path = strings.TrimRight(u.Path, "/") + "/workflows/" + url.PathEscape(workflow)
	q := u.Query()
	q.Set("wait", "result")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func readBundleFiles(repoRoot string, records []bundle.FileRecord) ([]docFile, error) {
	files := make([]docFile, 0, len(records))
	for _, rec := range records {
		if err := bundle.ValidateRelativePath(rec.Path); err != nil {
			return nil, err
		}
		path := filepath.Join(repoRoot, filepath.FromSlash(rec.Path))
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		files = append(files, docFile{Path: rec.Path, Content: string(b)})
	}
	return files, nil
}

func runtimeSecrets(cfg Config) (map[string]string, error) {
	if !cfg.LiveVerify || len(cfg.RuntimeSecrets) == 0 {
		return nil, nil
	}
	secrets, _, err := runtimeenv.NormalizeMap(cfg.RuntimeSecrets)
	return secrets, err
}

func publicDocsBundleResult(urls []string) bundle.Result {
	h := sha256.New()
	for _, u := range urls {
		fmt.Fprintln(h, u)
	}
	return bundle.Result{
		SHA256:   hex.EncodeToString(h.Sum(nil)),
		Manifest: bundle.Manifest{Files: []bundle.FileRecord{}},
	}
}

func feedbackModelsOrDefault(models []string) []string {
	out := make([]string, 0, len(models))
	seen := map[string]bool{}
	for _, raw := range models {
		model := strings.TrimSpace(raw)
		if model == "" || seen[model] {
			continue
		}
		seen[model] = true
		out = append(out, model)
	}
	if len(out) == 0 {
		return []string{""}
	}
	return out
}

func displayModel(model string) string {
	if strings.TrimSpace(model) == "" {
		return "default"
	}
	return model
}

func formatFeedbackReport(taskIndex int, taskCount int, model string, report string) string {
	report = strings.TrimSpace(report)
	if taskCount <= 1 && strings.TrimSpace(model) == "" {
		return report
	}
	var header []string
	header = append(header, fmt.Sprintf("Task index: %d", taskIndex+1))
	if strings.TrimSpace(model) != "" {
		header = append(header, "Tester model: "+model)
	}
	return strings.Join(header, "\n") + "\n\n" + report
}

func writeFeedbackReport(outDir string, idx int, model string, report string) error {
	filename := fmt.Sprintf("feedback-%03d.md", idx+1)
	if strings.TrimSpace(model) != "" {
		filename = fmt.Sprintf("feedback-%03d-%s.md", idx+1, safeFilenamePart(model))
	}
	path := filepath.Join(outDir, "runs", filename)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.TrimSpace(report)+"\n"), 0o644)
}

func writeAggregate(outDir string, reports []string) error {
	return os.WriteFile(filepath.Join(outDir, "aggregate-feedback.md"), []byte(runner.AggregateFeedback(reports)+"\n"), 0o644)
}

func writeUpdatedFiles(updatedDir string, edited editorResult) error {
	if err := os.MkdirAll(updatedDir, 0o755); err != nil {
		return err
	}
	for _, f := range edited.Files {
		relPath := normalizeEditedPath(f.Path)
		if err := bundle.ValidateRelativePath(relPath); err != nil {
			return err
		}
		path := filepath.Join(updatedDir, filepath.FromSlash(relPath))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(path, []byte(f.Content), 0o644); err != nil {
			return err
		}
	}
	if strings.TrimSpace(edited.Changelog) != "" {
		if err := os.WriteFile(filepath.Join(updatedDir, "CHANGELOG.md"), []byte(strings.TrimSpace(edited.Changelog)+"\n"), 0o644); err != nil {
			return err
		}
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

func normalizeEditedPath(path string) string {
	path = filepath.ToSlash(strings.TrimSpace(path))
	path = strings.TrimPrefix(path, "./")
	path = strings.TrimPrefix(path, "input-docs/files/")
	return path
}

func ApplyUpdatedDocs(updatedDir, repoRoot string) error {
	return workspace.CopyTree(updatedDir, repoRoot)
}
