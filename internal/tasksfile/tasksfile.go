// Package tasksfile reads task lists in the format older dari-docs used: one task per paragraph or
// bullet, so existing task files carry over to SelfBench scenarios.
package tasksfile

import (
	"strings"
	"unicode/utf8"
)

// MaxTitle is the longest title SelfBench takes for a scenario.
const MaxTitle = 120

// Task is one scenario: its title and the full text.
type Task struct {
	Title    string
	Scenario string
}

// Parse splits text into tasks. A blank line ends a task, and a bullet (`- `, `* `, `+ `, or `1. `)
// starts one; other lines continue the task before them.
func Parse(text string) []Task {
	var tasks []Task
	var current []string
	flush := func() {
		if s := strings.TrimSpace(strings.Join(current, "\n")); s != "" {
			tasks = append(tasks, Task{Title: Title(s), Scenario: s})
		}
		current = nil
	}
	for _, line := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			flush()
			continue
		}
		if rest, ok := bullet(trimmed); ok {
			flush()
			current = append(current, rest)
			continue
		}
		current = append(current, trimmed)
	}
	flush()
	return tasks
}

func bullet(line string) (string, bool) {
	for _, p := range []string{"- ", "* ", "+ "} {
		if rest, ok := strings.CutPrefix(line, p); ok {
			return strings.TrimSpace(rest), true
		}
	}
	digits := 0
	for digits < len(line) && line[digits] >= '0' && line[digits] <= '9' {
		digits++
	}
	if digits > 0 && digits+1 < len(line) && (line[digits] == '.' || line[digits] == ')') && line[digits+1] == ' ' {
		return strings.TrimSpace(line[digits+2:]), true
	}
	return "", false
}

// Title is a task's first line, cut to SelfBench's limit at a word.
func Title(scenario string) string {
	first, _, _ := strings.Cut(scenario, "\n")
	first = strings.TrimRight(strings.TrimSpace(first), ".:")
	if utf8.RuneCountInString(first) <= MaxTitle {
		return first
	}
	r := []rune(first)[:MaxTitle-1]
	cut := string(r)
	if i := strings.LastIndex(cut, " "); i > MaxTitle/2 {
		cut = cut[:i]
	}
	return cut + "…"
}
