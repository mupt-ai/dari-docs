package runner

import (
	"strings"
	"testing"
)

func TestAggregateFeedbackByTaskGroupsInterleavedResults(t *testing.T) {
	got := AggregateFeedbackByTask([]FeedbackResult{
		{TaskIndex: 1, Task: "First task", LLMID: "llm-a", Report: "task 1 from a"},
		{TaskIndex: 2, Task: "Second task", LLMID: "llm-b", Report: "task 2 from b"},
		{TaskIndex: 1, Task: "First task", LLMID: "llm-c", Report: "task 1 from c"},
	})
	if count := strings.Count(got, "## Task 1"); count != 1 {
		t.Fatalf("Task 1 heading count = %d, want 1:\n%s", count, got)
	}
	if count := strings.Count(got, "## Task 2"); count != 1 {
		t.Fatalf("Task 2 heading count = %d, want 1:\n%s", count, got)
	}
	if strings.Index(got, "task 1 from a") > strings.Index(got, "## Task 2") || strings.Index(got, "task 1 from c") > strings.Index(got, "## Task 2") {
		t.Fatalf("Task 1 reports were not grouped before Task 2:\n%s", got)
	}
}
