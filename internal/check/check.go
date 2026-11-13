// Package check runs a docs check on SelfBench: it builds the repository's approved tasks with the
// changed docs as they were before and after the change (two repo ablations), runs the configured
// models on both, and scores the difference.
package check

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/ablation"
	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/docsdiff"
	"github.com/mupt-ai/dari-docs/internal/report"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
)

// API is the part of SelfBench a check uses.
type API interface {
	Environments(ctx context.Context, repo string) ([]selfbench.Environment, error)
	CreateEnvironment(ctx context.Context, repo, name, script string, generation json.RawMessage) (selfbench.Environment, error)
	Environment(ctx context.Context, repo string, id int64) (selfbench.Environment, []selfbench.EnvironmentTask, error)
	StartComparison(ctx context.Context, repo string, req selfbench.ComparisonRequest) (selfbench.Comparison, error)
	Comparison(ctx context.Context, repo, id string) (selfbench.Comparison, error)
	ResumeComparison(ctx context.Context, repo, id string) (selfbench.Comparison, error)
	Evaluation(ctx context.Context, repo, id string) (selfbench.Evaluation, error)
}

// Stage is where a running check is, for progress updates.
type Stage string

const (
	StageBuilding Stage = "building"
	StageRunning  Stage = "running"
)

// Progress is a running check's state, reported as it changes.
type Progress struct {
	Stage        Stage
	Before       selfbench.Environment
	After        selfbench.Environment
	Comparison   *selfbench.Comparison
	Tasks        int
	Environments []string
}

// Options are one check's inputs.
type Options struct {
	Repo   string
	PR     int
	Config config.Config
	Change docsdiff.Change
	// Fresh starts a new comparison even when this exact check already ran.
	Fresh bool
	// Poll is how long to wait between reads of SelfBench.
	Poll       time.Duration
	Sleep      func(context.Context, time.Duration) error
	OnProgress func(Progress)
	Log        func(format string, args ...any)
}

// Result is a finished check.
type Result struct {
	Report       report.Report
	ComparisonID string
	Before       selfbench.Environment
	After        selfbench.Environment
}

// Side names, as they appear in environment names and conditions.
const (
	BeforeName = "before"
	AfterName  = "after"
)

// Run performs a docs check. The change must touch at least one docs file.
func Run(ctx context.Context, api API, o Options) (Result, error) {
	if o.Change.Empty() {
		return Result{}, errors.New("the change touches no docs")
	}
	if o.Poll <= 0 {
		o.Poll = 30 * time.Second
	}
	if o.Sleep == nil {
		o.Sleep = sleep
	}
	if o.Log == nil {
		o.Log = func(string, ...any) {}
	}
	progress := func(p Progress) {
		if o.OnProgress != nil {
			o.OnProgress(p)
		}
	}
	ctx, cancel := context.WithTimeout(ctx, time.Duration(o.Config.TimeoutMinutes)*time.Minute)
	defer cancel()

	beforeScript, err := ablation.Script(o.Change.Before)
	if err != nil {
		return Result{}, err
	}
	afterScript, err := ablation.Script(o.Change.After)
	if err != nil {
		return Result{}, err
	}
	if beforeScript == afterScript {
		return Result{}, errors.New("the docs are the same before and after the change")
	}
	before, err := ensureEnvironment(ctx, api, o, ablation.Name(o.PR, BeforeName, beforeScript), beforeScript)
	if err != nil {
		return Result{}, err
	}
	after, err := ensureEnvironment(ctx, api, o, ablation.Name(o.PR, AfterName, afterScript), afterScript)
	if err != nil {
		return Result{}, err
	}
	progress(Progress{Stage: StageBuilding, Before: before, After: after})

	before, beforeTasks, err := waitBuilt(ctx, api, o, before)
	if err != nil {
		return Result{}, err
	}
	after, afterTasks, err := waitBuilt(ctx, api, o, after)
	if err != nil {
		return Result{}, err
	}
	tasks, unbuilt := runnable(beforeTasks, afterTasks, o.Config.Tasks)
	if len(tasks) == 0 {
		r := report.Report{Unbuilt: unbuilt}
		return Result{Report: r, Before: before, After: after}, fmt.Errorf("no task built with both versions of the docs (%d failed); see %q and %q in SelfBench", len(unbuilt), before.Name, after.Name)
	}

	req := comparisonRequest(o, tasks, before.ID, after.ID, beforeScript, afterScript)
	o.Log("starting comparison %s on %d tasks", req.ID, len(tasks))
	comparison, err := api.StartComparison(ctx, o.Repo, req)
	if err != nil {
		return Result{}, fmt.Errorf("starting the comparison: %w", err)
	}
	progress(Progress{Stage: StageRunning, Before: before, After: after, Comparison: &comparison, Tasks: len(tasks)})
	for !comparison.Done() {
		if comparison.SubmissionError != "" {
			o.Log("resuming comparison: %s", comparison.SubmissionError)
			if resumed, err := api.ResumeComparison(ctx, o.Repo, comparison.ID); err == nil {
				comparison = resumed
			}
		}
		if err := o.Sleep(ctx, o.Poll); err != nil {
			return Result{ComparisonID: comparison.ID}, fmt.Errorf("waiting for comparison %s: %w", comparison.ID, err)
		}
		next, err := api.Comparison(ctx, o.Repo, comparison.ID)
		if err != nil {
			o.Log("reading comparison: %v", err)
			continue
		}
		if changed(comparison, next) {
			progress(Progress{Stage: StageRunning, Before: before, After: after, Comparison: &next, Tasks: len(tasks)})
		}
		comparison = next
	}

	var evaluations []selfbench.Evaluation
	for _, run := range comparison.Runs {
		e, err := api.Evaluation(ctx, o.Repo, run.ID)
		if err != nil {
			return Result{ComparisonID: comparison.ID}, fmt.Errorf("reading evaluation %s: %w", run.ID, err)
		}
		if e.Condition == nil && run.Condition != "" {
			e.Condition = &selfbench.RunCondition{Name: run.Condition}
		}
		evaluations = append(evaluations, e)
	}
	sideOf := func(e selfbench.Evaluation) (report.Side, bool) {
		if e.Condition == nil {
			return 0, false
		}
		if env := e.Condition.Environment; env != nil {
			switch env.ID {
			case before.ID:
				return report.Before, true
			case after.ID:
				return report.After, true
			}
			return 0, false
		}
		switch e.Condition.Name {
		case conditionName(BeforeName), before.Name:
			return report.Before, true
		case conditionName(AfterName), after.Name:
			return report.After, true
		}
		return 0, false
	}
	r := report.Build(tasks, evaluations, sideOf)
	r.Unbuilt = unbuilt
	return Result{Report: r, ComparisonID: comparison.ID, Before: before, After: after}, nil
}

// ensureEnvironment reuses the repo ablation that already makes this exact change, or creates it.
func ensureEnvironment(ctx context.Context, api API, o Options, name, script string) (selfbench.Environment, error) {
	existing, err := api.Environments(ctx, o.Repo)
	if err != nil {
		if selfbench.IsStatus(err, 404) {
			return selfbench.Environment{}, fmt.Errorf("%s is not connected to SelfBench workspace: connect it at app.selfbench.dev first (%w)", o.Repo, err)
		}
		return selfbench.Environment{}, err
	}
	for _, e := range existing {
		if e.Name == name && (e.Script == "" || e.Script == script) {
			o.Log("reusing repo ablation %q (%d)", name, e.ID)
			return e, nil
		}
	}
	o.Log("creating repo ablation %q", name)
	e, err := api.CreateEnvironment(ctx, o.Repo, name, script, o.Config.Generation)
	if err != nil {
		if selfbench.IsStatus(err, 409) {
			return selfbench.Environment{}, fmt.Errorf("SelfBench has no approved task to check the docs with: add some with `dari-docs tasks add`, approve them in the app, then rerun (%w)", err)
		}
		return selfbench.Environment{}, fmt.Errorf("creating repo ablation %q: %w", name, err)
	}
	return e, nil
}

// waitBuilt waits until no task in the environment is still building.
func waitBuilt(ctx context.Context, api API, o Options, e selfbench.Environment) (selfbench.Environment, []selfbench.EnvironmentTask, error) {
	for {
		env, tasks, err := api.Environment(ctx, o.Repo, e.ID)
		if err != nil {
			o.Log("reading repo ablation %d: %v", e.ID, err)
		} else if env.Tasks.Checking == 0 && env.Tasks.Total > 0 {
			return env, tasks, nil
		} else {
			o.Log("%s: %d/%d built, %d building", env.Name, env.Tasks.Ready, env.Tasks.Total, env.Tasks.Checking)
		}
		if err := o.Sleep(ctx, o.Poll); err != nil {
			return e, nil, fmt.Errorf("waiting for repo ablation %q to build: %w", e.Name, err)
		}
	}
}

// runnable are the tasks built with both versions of the docs, limited to the configured ones, and
// why each other task was left out.
func runnable(before, after []selfbench.EnvironmentTask, only []string) ([]selfbench.TaskRef, map[string]string) {
	state := func(tasks []selfbench.EnvironmentTask) map[string]selfbench.EnvironmentTask {
		m := map[string]selfbench.EnvironmentTask{}
		for _, t := range tasks {
			m[t.Task.String()] = t
		}
		return m
	}
	b, a := state(before), state(after)
	wanted := map[string]bool{}
	for _, t := range only {
		wanted[t] = true
	}
	keys := map[string]bool{}
	for k := range b {
		keys[k] = true
	}
	for k := range a {
		keys[k] = true
	}
	unbuilt := map[string]string{}
	var tasks []selfbench.TaskRef
	for k := range keys {
		if len(wanted) > 0 && !wanted[k] {
			continue
		}
		bt, bok := b[k]
		at, aok := a[k]
		switch {
		case bok && aok && bt.State == "accepted" && at.State == "accepted":
			tasks = append(tasks, bt.Task)
		case !bok || bt.State != "accepted":
			unbuilt[k] = "before the change: " + reason(bt)
		default:
			unbuilt[k] = "after the change: " + reason(at)
		}
	}
	for k := range wanted {
		if !keys[k] {
			unbuilt[k] = "not approved in SelfBench yet"
		}
	}
	sort.Slice(tasks, func(i, j int) bool { return tasks[i].String() < tasks[j].String() })
	return tasks, unbuilt
}

func reason(t selfbench.EnvironmentTask) string {
	if t.ReasonSummary != "" {
		return t.ReasonSummary
	}
	if t.State == "" {
		return "not built"
	}
	return t.State
}

func conditionName(side string) string { return "Docs " + side }

func comparisonRequest(o Options, tasks []selfbench.TaskRef, beforeID, afterID int64, beforeScript, afterScript string) selfbench.ComparisonRequest {
	models := make([]selfbench.ComparisonModel, 0, len(o.Config.Models))
	for _, m := range o.Config.Models {
		models = append(models, selfbench.ComparisonModel{
			CatalogID:    m.CatalogID,
			CredentialID: m.CredentialID,
			Harnesses:    m.Harnesses,
			Thinking:     m.Thinking,
		})
	}
	noBaseline := false
	req := selfbench.ComparisonRequest{
		Models:              models,
		Sandbox:             o.Config.Sandbox,
		SandboxCredentialID: o.Config.SandboxCredentialID,
		Tasks:               tasks,
		JudgeCredentialID:   o.Config.JudgeCredentialID,
		Conditions: []selfbench.Condition{
			{Name: conditionName(BeforeName), Environment: beforeID},
			{Name: conditionName(AfterName), Environment: afterID},
		},
		// The tasks as they were authored run neither version of the change; only the two sides count.
		Baseline: &noBaseline,
	}
	seed := []string{o.Repo, fmt.Sprint(o.PR), ablation.Hash(beforeScript), ablation.Hash(afterScript)}
	if b, err := json.Marshal(req); err == nil {
		seed = append(seed, string(b))
	}
	if o.Fresh {
		seed = append(seed, time.Now().UTC().Format(time.RFC3339Nano))
	}
	req.ID = uuidFrom(strings.Join(seed, "\x00"))
	return req
}

// uuidFrom is a version 5 style UUID from a seed, so the same check names the same comparison and a
// rerun never starts it twice.
func uuidFrom(seed string) string {
	sum := sha256.Sum256([]byte(seed))
	b := sum[:16]
	b[6] = (b[6] & 0x0f) | 0x50
	b[8] = (b[8] & 0x3f) | 0x80
	h := hex.EncodeToString(b)
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

func changed(a, b selfbench.Comparison) bool {
	if len(a.Runs) != len(b.Runs) {
		return true
	}
	for i := range a.Runs {
		if a.Runs[i].Status != b.Runs[i].Status || a.Runs[i].Completed != b.Runs[i].Completed {
			return true
		}
	}
	return false
}

func sleep(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-t.C:
		return nil
	}
}

// RunningMarkdown is the comment while a check runs.
func RunningMarkdown(p Progress, footer string) string {
	var b strings.Builder
	switch p.Stage {
	case StageBuilding:
		fmt.Fprintf(&b, "### dari-docs: building tasks with the docs before and after this change\n\n")
		fmt.Fprintf(&b, "SelfBench is building each approved task twice: with the changed docs as they were (%s) and as this pull request has them (%s).\n\n", code(p.Before.Name), code(p.After.Name))
	case StageRunning:
		b.WriteString("### dari-docs: agents are trying the tasks\n\n")
		if c := p.Comparison; c != nil {
			done, total := 0, 0
			for _, r := range c.Runs {
				done += r.Completed
				total += r.Trials
			}
			fmt.Fprintf(&b, "%d of %d trials finished across %d %s.\n\n", done, total, p.Tasks, plural(p.Tasks, "task", "tasks"))
		}
	}
	b.WriteString(footer)
	return b.String()
}

func code(s string) string { return "`" + strings.ReplaceAll(s, "`", "'") + "`" }

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
