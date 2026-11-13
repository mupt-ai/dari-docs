package github

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseEventMarksForks(t *testing.T) {
	event := func(head string) []byte {
		return []byte(`{"action":"synchronize","pull_request":{"number":7,"base":{"sha":"b","repo":{"full_name":"acme/sdk"}},"head":{"sha":"h","repo":{"full_name":"` + head + `"}}}}`)
	}
	pr, ok, err := ParseEvent(event("acme/sdk"))
	if err != nil || !ok || pr != (PullRequest{Repo: "acme/sdk", Number: 7, BaseSHA: "b", HeadSHA: "h", Action: "synchronize"}) {
		t.Fatalf("same-repo pull request = %+v %v %v", pr, ok, err)
	}
	if pr, _, _ := ParseEvent(event("someone/sdk")); !pr.Fork {
		t.Error("a pull request from another repository is not marked as a fork, so its job would fail on the missing secret")
	}
	if _, ok, _ := ParseEvent([]byte(`{"ref":"refs/heads/main"}`)); ok {
		t.Error("a push event read as a pull request")
	}
}

// The next run finds the bot's comment, ignores a user's copy of the marker, reads the comparison it
// left, and edits that comment in place.
func TestCommentCarriesStateToTheNextRun(t *testing.T) {
	var comments []map[string]any
	var patched, posted int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			_ = json.NewEncoder(w).Encode(comments)
		case "PATCH":
			patched++
			_, _ = w.Write([]byte(`{"id":2}`))
		case "POST":
			posted++
			_, _ = w.Write([]byte(`{"id":3}`))
		}
	}))
	defer server.Close()
	gh := &Client{APIURL: server.URL, Token: "t"}
	ctx := context.Background()

	comments = []map[string]any{
		{"id": 1, "body": Body("spoofed", State{Comparison: "evil"}), "user": map[string]string{"type": "User"}},
	}
	found, err := gh.FindComment(ctx, "acme/sdk", 7)
	if err != nil || found != nil {
		t.Fatalf("found %+v, %v: a user's comment must not supply state", found, err)
	}
	if _, err := gh.UpsertComment(ctx, "acme/sdk", 7, found, "x"); err != nil || posted != 1 {
		t.Fatalf("no existing comment: posted %d, %v", posted, err)
	}

	comments = append(comments, map[string]any{"id": 2, "body": Body("### running", State{Comparison: "c-1"}), "user": map[string]string{"type": "Bot"}})
	found, err = gh.FindComment(ctx, "acme/sdk", 7)
	if err != nil || found == nil || found.ID != 2 || found.State.Comparison != "c-1" {
		t.Fatalf("found %+v, %v; want the bot's comment with comparison c-1", found, err)
	}
	if _, err := gh.UpsertComment(ctx, "acme/sdk", 7, found, "y"); err != nil || patched != 1 || posted != 1 {
		t.Fatalf("existing comment: patched %d, posted %d, %v", patched, posted, err)
	}
}
