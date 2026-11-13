package tasksfile

import (
	"reflect"
	"strings"
	"testing"
	"unicode/utf8"
)

// The example task file from the old dari-docs docs must import as the same three tasks.
func TestParseReadsOldTaskFiles(t *testing.T) {
	got := Parse(`- Install the SDK and make a first API call

- Set up authentication
  using an API key from the dashboard.

Create a webhook endpoint and verify a test event.
1. Rotate a key
2) Revoke a key
`)
	want := []Task{
		{"Install the SDK and make a first API call", "Install the SDK and make a first API call"},
		{"Set up authentication", "Set up authentication\nusing an API key from the dashboard."},
		{"Create a webhook endpoint and verify a test event", "Create a webhook endpoint and verify a test event."},
		{"Rotate a key", "Rotate a key"},
		{"Revoke a key", "Revoke a key"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Parse =\n%q\nwant\n%q", got, want)
	}
}

func TestTitlesFitSelfBench(t *testing.T) {
	long := strings.Repeat("word ", 60)
	tasks := Parse(long)
	if n := utf8.RuneCountInString(tasks[0].Title); n > MaxTitle {
		t.Errorf("title is %d characters; SelfBench refuses more than %d", n, MaxTitle)
	}
	if tasks[0].Scenario != strings.TrimSpace(long) {
		t.Error("the scenario lost text when its title was cut")
	}
}
