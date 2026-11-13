// Package report scores a docs check's trials, before and after the docs change, and writes the
// pull request comment.
package report

import (
	"fmt"
	"sort"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/selfbench"
)

// Side is one arm of a docs check.
type Side int

const (
	Before Side = iota
	After
)

// Outcome is one configuration's latest trial of one task on one side.
type Outcome struct {
	Scored  bool
	Passed  bool
	Error   string
	Why     string
	CostUSD *float64
}

// Row is one configuration's results: a model on a harness.
type Row struct {
	Model   string
	Harness string
	// Outcomes by task, for each side.
	Before map[string]Outcome
	After  map[string]Outcome
}

func (r Row) name() string { return r.Model + " · " + r.Harness }

// Tally counts a side's scored and passing tasks among tasks.
func tally(outcomes map[string]Outcome, tasks []string) (passed, scored int, cost float64, priced bool) {
	priced = true
	for _, t := range tasks {
		o := outcomes[t]
		if !o.Scored {
			continue
		}
		scored++
		if o.Passed {
			passed++
		}
		if o.CostUSD != nil {
			cost += *o.CostUSD
		} else {
			priced = false
		}
	}
	return passed, scored, cost, priced && scored > 0
}

// Flip is a task whose result changed with the docs.
type Flip struct {
	Task  string
	Row   string
	Why   string
	Fixed bool
}

// Report is a whole docs check.
type Report struct {
	Tasks []string
	Rows  []Row
	// Failed are tasks the docs ablations could not build, with why.
	Unbuilt map[string]string
}

// Build groups evaluations' trials into rows. sideOf says which side an evaluation ran, or false
// for one this check didn't start.
func Build(tasks []selfbench.TaskRef, evaluations []selfbench.Evaluation, sideOf func(selfbench.Evaluation) (Side, bool)) Report {
	r := Report{Unbuilt: map[string]string{}}
	for _, t := range tasks {
		r.Tasks = append(r.Tasks, t.String())
	}
	sort.Strings(r.Tasks)
	rows := map[string]*Row{}
	var order []string
	for _, e := range evaluations {
		side, ok := sideOf(e)
		if !ok {
			continue
		}
		model := e.ModelLabel
		if model == "" {
			model = e.Model
		}
		for _, t := range e.Trials {
			if t.Status != "completed" && t.Status != "failed" {
				continue
			}
			key := model + "\x00" + t.Harness
			row := rows[key]
			if row == nil {
				row = &Row{Model: model, Harness: t.Harness, Before: map[string]Outcome{}, After: map[string]Outcome{}}
				rows[key] = row
				order = append(order, key)
			}
			o := Outcome{Scored: t.Status == "completed", Passed: t.Status == "completed" && t.Passed(), CostUSD: t.APICostUSD}
			if t.Status == "failed" {
				o.Error = t.Error
				if o.Error == "" {
					o.Error = "the trial errored"
				}
			}
			if t.FailureSummary != nil {
				o.Why = t.FailureSummary.Text
			}
			// Later trials of a task (retries) replace earlier ones.
			ref := selfbench.TaskRef{RunID: t.RunID, TaskID: t.TaskID}.String()
			if side == Before {
				row.Before[ref] = o
			} else {
				row.After[ref] = o
			}
		}
	}
	sort.Strings(order)
	for _, key := range order {
		r.Rows = append(r.Rows, *rows[key])
	}
	return r
}

// Flips are the tasks that changed, regressions first.
func (r Report) Flips() []Flip {
	var flips []Flip
	for _, row := range r.Rows {
		for _, t := range r.Tasks {
			b, a := row.Before[t], row.After[t]
			if !b.Scored || !a.Scored || b.Passed == a.Passed {
				continue
			}
			why := a.Why
			if a.Passed {
				why = b.Why
			}
			flips = append(flips, Flip{Task: t, Row: row.name(), Why: why, Fixed: a.Passed})
		}
	}
	sort.SliceStable(flips, func(i, j int) bool { return !flips[i].Fixed && flips[j].Fixed })
	return flips
}

// Regressed reports whether any task passed before the change and failed after.
func (r Report) Regressed() bool {
	for _, f := range r.Flips() {
		if !f.Fixed {
			return true
		}
	}
	return false
}

// StillFailing are tasks that fail on both sides: what the docs still don't get readers through.
func (r Report) StillFailing() []Flip {
	var out []Flip
	for _, row := range r.Rows {
		for _, t := range r.Tasks {
			b, a := row.Before[t], row.After[t]
			if b.Scored && a.Scored && !b.Passed && !a.Passed {
				out = append(out, Flip{Task: t, Row: row.name(), Why: a.Why})
			}
		}
	}
	return out
}

// Errors are trials that errored instead of being scored.
func (r Report) Errors() []string {
	var out []string
	for _, row := range r.Rows {
		for _, t := range r.Tasks {
			for _, side := range []struct {
				name string
				o    Outcome
			}{{"before", row.Before[t]}, {"after", row.After[t]}} {
				if side.o.Error != "" {
					out = append(out, fmt.Sprintf("%s, %s, %s: %s", t, row.name(), side.name, firstLine(side.o.Error)))
				}
			}
		}
	}
	return out
}

// Context is what the comment says about where the check ran.
type Context struct {
	Paths         []string
	Skipped       []string
	ComparisonURL string
	Head          string
}

// Markdown renders the finished check.
func (r Report) Markdown(c Context) string {
	var b strings.Builder
	verdict := "No change in results"
	switch {
	case r.Regressed():
		verdict = "⚠️ The docs change made some tasks fail"
	case len(r.Flips()) > 0:
		verdict = "✅ The docs change fixed some tasks"
	}
	fmt.Fprintf(&b, "### dari-docs: %s\n\n", verdict)
	fmt.Fprintf(&b, "Agents ran %d %s with the docs as they were before this change and as of %s. Only the changed docs differ between the two runs.\n\n", len(r.Tasks), plural(len(r.Tasks), "task", "tasks"), code(short(c.Head)))
	if len(r.Rows) == 0 {
		b.WriteString("No trial finished, so there is nothing to compare.\n")
	} else {
		b.WriteString("| Model · Harness | Before | After | Change | Cost before → after |\n| --- | --- | --- | --- | --- |\n")
		for _, row := range r.Rows {
			bp, bs, bc, bpriced := tally(row.Before, r.Tasks)
			ap, as, ac, apriced := tally(row.After, r.Tasks)
			fmt.Fprintf(&b, "| %s | %d/%d | %d/%d | %s | %s → %s |\n", row.name(), bp, bs, ap, as, delta(bp, bs, ap, as), usd(bc, bpriced), usd(ac, apriced))
		}
		b.WriteString("\n")
	}
	if flips := r.Flips(); len(flips) > 0 {
		b.WriteString("**Changed results**\n\n")
		for _, f := range flips {
			mark := "❌ now fails"
			if f.Fixed {
				mark = "✅ now passes"
			}
			fmt.Fprintf(&b, "- %s %s on %s", mark, code(f.Task), f.Row)
			if f.Why != "" {
				fmt.Fprintf(&b, ": %s", oneLine(f.Why))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n")
	}
	if still := r.StillFailing(); len(still) > 0 {
		fmt.Fprintf(&b, "<details><summary>Failing before and after (%d)</summary>\n\n", len(still))
		for _, f := range still {
			fmt.Fprintf(&b, "- %s on %s", code(f.Task), f.Row)
			if f.Why != "" {
				fmt.Fprintf(&b, ": %s", oneLine(f.Why))
			}
			b.WriteString("\n")
		}
		b.WriteString("\n</details>\n\n")
	}
	if errs := r.Errors(); len(errs) > 0 {
		fmt.Fprintf(&b, "<details><summary>Trials that errored and were not scored (%d)</summary>\n\n", len(errs))
		for _, e := range errs {
			fmt.Fprintf(&b, "- %s\n", e)
		}
		b.WriteString("\n</details>\n\n")
	}
	if len(r.Unbuilt) > 0 {
		fmt.Fprintf(&b, "<details><summary>Tasks left out because a docs build failed (%d)</summary>\n\n", len(r.Unbuilt))
		keys := make([]string, 0, len(r.Unbuilt))
		for k := range r.Unbuilt {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			fmt.Fprintf(&b, "- %s: %s\n", code(k), oneLine(r.Unbuilt[k]))
		}
		b.WriteString("\n</details>\n\n")
	}
	b.WriteString(Footer(c))
	return b.String()
}

// Footer lists the changed docs and links the comparison.
func Footer(c Context) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<details><summary>Changed docs (%d)</summary>\n\n", len(c.Paths))
	for _, p := range c.Paths {
		fmt.Fprintf(&b, "- %s\n", code(p))
	}
	for _, p := range c.Skipped {
		fmt.Fprintf(&b, "- %s (skipped: not a regular file)\n", code(p))
	}
	b.WriteString("\n</details>\n\n")
	if c.ComparisonURL != "" {
		fmt.Fprintf(&b, "[Open the comparison in SelfBench](%s) for transcripts and costs.\n", c.ComparisonURL)
	}
	return b.String()
}

func delta(bp, bs, ap, as int) string {
	if bs == 0 || as == 0 {
		return "–"
	}
	d := float64(ap)/float64(as)*100 - float64(bp)/float64(bs)*100
	switch {
	case d > 0.05:
		return fmt.Sprintf("+%.0f pts", d)
	case d < -0.05:
		return fmt.Sprintf("%.0f pts", d)
	}
	return "0"
}

func usd(v float64, priced bool) string {
	if !priced {
		return "–"
	}
	return fmt.Sprintf("$%.2f", v)
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func code(s string) string { return "`" + strings.ReplaceAll(s, "`", "'") + "`" }

func short(sha string) string {
	if len(sha) > 7 {
		return sha[:7]
	}
	return sha
}

func firstLine(s string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(s), "\n")
	return oneLine(line)
}

// oneLine keeps a model's explanation to one list item.
func oneLine(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 400 {
		s = string(r[:400]) + "…"
	}
	return s
}
