package check

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/mupt-ai/dari-docs/internal/config"
	"github.com/mupt-ai/dari-docs/internal/docsdiff"
	"github.com/mupt-ai/dari-docs/internal/report"
	"github.com/mupt-ai/dari-docs/internal/selfbench"
)

// fakeSelfBench serves the SelfBench routes a check uses, with three approved tasks: t1 and t2
// build with both versions of the docs; t3's build fails after the change.
type fakeSelfBench struct {
	mu           sync.Mutex
	envs         []selfbench.Environment
	reads        map[int64]int
	created      []map[string]any
	comparisons  map[string]map[string]any
	comparePosts int
	polls        int
	// passes says whether a task passes on a side: "before:t1".
	passes map[string]bool
}

func newFake() *fakeSelfBench {
	return &fakeSelfBench{reads: map[int64]int{}, comparisons: map[string]map[string]any{}, passes: map[string]bool{
		"before:t1": true, "after:t1": false, // the change broke t1
		"before:t2": false, "after:t2": true, // and fixed t2
	}}
}

func (f *fakeSelfBench) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if r.Header.Get("Authorization") != "Bearer key" {
		http.Error(w, `{"error":"no key"}`, 401)
		return
	}
	const repo = "/api/orgs/acme/repos/acme/sdk"
	path := strings.TrimPrefix(r.URL.Path, repo)
	send := func(v any) { _ = json.NewEncoder(w).Encode(v) }
	switch {
	case path == "/environments" && r.Method == "GET":
		send(map[string]any{"environments": f.envs})
	case path == "/environments" && r.Method == "POST":
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.created = append(f.created, body)
		env := body["environment"].(map[string]any)
		e := selfbench.Environment{ID: int64(len(f.envs) + 1), Name: env["name"].(string), Script: env["script"].(string), Tasks: selfbench.Coverage{Checking: 3, Total: 3}}
		f.envs = append(f.envs, e)
		w.WriteHeader(201)
		send(map[string]any{"environment": e})
	case strings.HasPrefix(path, "/environments/") && r.Method == "GET":
		var id int64
		fmt.Sscanf(strings.TrimPrefix(path, "/environments/"), "%d", &id)
		e := f.envs[id-1]
		f.reads[id]++
		tasks := []selfbench.EnvironmentTask{}
		if f.reads[id] == 1 { // still building on the first read
			e.Tasks = selfbench.Coverage{Ready: 1, Checking: 2, Total: 3}
		} else {
			after := strings.Contains(e.Name, " after ")
			e.Tasks = selfbench.Coverage{Ready: 3, Total: 3}
			t3 := selfbench.EnvironmentTask{Task: selfbench.TaskRef{RunID: "r", TaskID: "t3"}, State: "accepted"}
			if after {
				e.Tasks = selfbench.Coverage{Ready: 2, Failed: 1, Total: 3}
				t3.State, t3.ReasonSummary = "failed", "npm install failed"
			}
			tasks = append(tasks,
				selfbench.EnvironmentTask{Task: selfbench.TaskRef{RunID: "r", TaskID: "t1"}, State: "accepted"},
				selfbench.EnvironmentTask{Task: selfbench.TaskRef{RunID: "r", TaskID: "t2"}, State: "accepted"},
				t3)
		}
		send(map[string]any{"environment": e, "tasks": tasks})
	case path == "/evaluations/comparisons" && r.Method == "POST":
		f.comparePosts++
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		id := body["id"].(string)
		if _, ok := f.comparisons[id]; ok {
			w.WriteHeader(400)
			send(map[string]any{"error": "comparison exists"})
			return
		}
		f.comparisons[id] = body
		w.WriteHeader(202)
		send(f.progress(id, "running"))
	case strings.HasPrefix(path, "/evaluations/comparisons/") && r.Method == "GET":
		id := strings.TrimPrefix(path, "/evaluations/comparisons/")
		if _, ok := f.comparisons[id]; !ok {
			w.WriteHeader(404)
			send(map[string]any{"error": "Comparison not found"})
			return
		}
		f.polls++
		send(f.progress(id, "completed"))
	case strings.HasPrefix(path, "/evaluations/") && r.Method == "GET":
		side := strings.TrimPrefix(path, "/evaluations/eval-")
		env := f.envs[0]
		if side == "after" {
			env = f.envs[1]
		}
		var trials []map[string]any
		for _, task := range []string{"t1", "t2"} {
			reward := 0
			if f.passes[side+":"+task] {
				reward = 1
			}
			trials = append(trials, map[string]any{"runId": "r", "taskId": task, "harness": "claude-code", "status": "completed",
				"rewards": map[string]int{"reward": reward}, "apiCostUsd": 0.5, "failureSummary": map[string]string{"text": "could not find the install step"}})
		}
		send(map[string]any{"id": "eval-" + side, "modelLabel": "Sonnet", "status": "completed", "trials": trials,
			"condition": map[string]any{"name": "Docs " + side, "environment": map[string]any{"id": env.ID, "name": env.Name}}})
	default:
		http.Error(w, `{"error":"unexpected `+r.Method+" "+path+`"}`, 404)
	}
}

func (f *fakeSelfBench) progress(id, status string) map[string]any {
	return map[string]any{"id": id, "runs": []map[string]any{
		{"id": "eval-before", "model": "Sonnet", "condition": "Docs before", "harnesses": []string{"claude-code"}, "status": status, "completed": 2, "trials": 2},
		{"id": "eval-after", "model": "Sonnet", "condition": "Docs after", "harnesses": []string{"claude-code"}, "status": status, "completed": 2, "trials": 2},
	}}
}

func options() Options {
	cfg := config.Config{Models: []config.Model{{CatalogID: "anthropic/sonnet", CredentialID: "managed-model", Harnesses: []string{"claude-code"}}}, Sandbox: "managed", SandboxCredentialID: "managed-sandbox", TimeoutMinutes: 5}
	return Options{
		Repo:   "acme/sdk",
		PR:     7,
		Config: cfg,
		Change: docsdiff.Change{
			Before: docsdiff.Snapshot{Files: []docsdiff.File{{Path: "docs/quickstart.md", Content: []byte("old")}}},
			After:  docsdiff.Snapshot{Files: []docsdiff.File{{Path: "docs/quickstart.md", Content: []byte("new")}}},
		},
		Poll:  time.Millisecond,
		Sleep: func(context.Context, time.Duration) error { return nil },
	}
}

func TestCheckComparesTheTwoVersionsOfTheDocs(t *testing.T) {
	fake := newFake()
	server := httptest.NewServer(fake)
	defer server.Close()
	api := &selfbench.Client{BaseURL: server.URL, APIKey: "key", Org: "acme"}

	result, err := Run(context.Background(), api, options())
	if err != nil {
		t.Fatal(err)
	}

	if len(fake.created) != 2 {
		t.Fatalf("created %d repo ablations, want one per side", len(fake.created))
	}
	if len(fake.comparisons) != 1 {
		t.Fatalf("started %d comparisons", len(fake.comparisons))
	}
	var body map[string]any
	for _, b := range fake.comparisons {
		body = b
	}
	if body["baseline"] != false {
		t.Errorf("baseline = %v: the tasks as authored run neither version of the docs", body["baseline"])
	}
	conds, _ := json.Marshal(body["conditions"])
	if string(conds) != `[{"environment":1,"name":"Docs before"},{"environment":2,"name":"Docs after"}]` {
		t.Errorf("conditions = %s", conds)
	}
	tasks, _ := json.Marshal(body["tasks"])
	if string(tasks) != `[{"runId":"r","taskId":"t1"},{"runId":"r","taskId":"t2"}]` {
		t.Errorf("tasks = %s: only tasks built with both versions can be compared", tasks)
	}

	if !result.Report.Regressed() {
		t.Error("t1 passed before and fails after, but no regression was reported")
	}
	flips := result.Report.Flips()
	if len(flips) != 2 || flips[0].Task != "r/t1" || flips[0].Fixed || flips[1].Task != "r/t2" || !flips[1].Fixed {
		t.Errorf("flips = %+v, want t1 regressed then t2 fixed", flips)
	}
	if got := result.Report.Unbuilt["r/t3"]; !strings.Contains(got, "npm install failed") {
		t.Errorf("t3 left out with %q, want the build's reason", got)
	}
	md := result.Report.Markdown(reportContext())
	for _, want := range []string{"| Sonnet · claude-code | 1/2 | 1/2 | 0 | $1.00 → $1.00 |", "❌ now fails `r/t1`", "✅ now passes `r/t2`"} {
		if !strings.Contains(md, want) {
			t.Errorf("comment lacks %q:\n%s", want, md)
		}
	}

	// A rerun of the same check reuses both builds and the finished comparison.
	if _, err := Run(context.Background(), api, options()); err != nil {
		t.Fatal(err)
	}
	if len(fake.created) != 2 || len(fake.comparisons) != 1 {
		t.Errorf("rerun created %d ablations and %d comparisons, want none new", len(fake.created)-2, len(fake.comparisons)-1)
	}
}

func TestCheckExplainsAWorkspaceWithNoApprovedTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"environments":[]}`))
			return
		}
		w.WriteHeader(409)
		_, _ = w.Write([]byte(`{"error":"Approve a task first: an environment builds every approved task."}`))
	}))
	defer server.Close()
	_, err := Run(context.Background(), &selfbench.Client{BaseURL: server.URL, APIKey: "key", Org: "acme"}, options())
	if err == nil || !strings.Contains(err.Error(), "dari-docs tasks add") {
		t.Fatalf("err = %v, want a pointer to adding tasks", err)
	}
}

func reportContext() report.Context {
	return report.Context{Paths: []string{"docs/quickstart.md"}, Head: "abcdef1234"}
}
