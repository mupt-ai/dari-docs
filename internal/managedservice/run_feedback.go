package managedservice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/dari"
)

type runFeedbackResult struct {
	SessionID string `json:"session_id"`
	TaskIndex int    `json:"task_index"`
	LLMID     string `json:"llm_id"`
	Report    string `json:"report"`
}

func (s *Server) completedTesterFeedbackResults(ctx context.Context, runID string) ([]runFeedbackResult, error) {
	rows, err := s.db.Query(ctx, `
SELECT session_id,
       task_index,
       coalesce(nullif(llm_id,''), $3)
FROM run_sessions
WHERE run_id=$1 AND kind='tester' AND status=$2
ORDER BY task_index, llm_id, created_at
`, runID, statusCompleted, defaultManagedEditorLLMID())
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	results := []runFeedbackResult{}
	for rows.Next() {
		var result runFeedbackResult
		if err := rows.Scan(&result.SessionID, &result.TaskIndex, &result.LLMID); err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range results {
		tr, err := s.dari.GetTranscript(ctx, results[i].SessionID)
		if err != nil {
			return nil, fmt.Errorf("%w: get transcript %s: %v", errRunFeedbackLoad, results[i].SessionID, err)
		}
		results[i].Report = dari.FinalAssistantText(tr)
	}
	return results, nil
}

func managedRunAggregateFeedbackMarkdown(status runStatusResponse, results []runFeedbackResult) string {
	var body strings.Builder
	currentTaskIndex := 0
	for _, result := range results {
		report := strings.TrimSpace(result.Report)
		if report == "" {
			continue
		}
		taskIndex := result.TaskIndex
		if taskIndex <= 0 {
			taskIndex = 1
		}
		if taskIndex != currentTaskIndex {
			if body.Len() > 0 {
				body.WriteString("\n")
			}
			body.WriteString(fmt.Sprintf("## Task %d\n\n", taskIndex))
			if taskIndex <= len(status.Tasks) && strings.TrimSpace(status.Tasks[taskIndex-1]) != "" {
				body.WriteString(strings.TrimSpace(status.Tasks[taskIndex-1]) + "\n\n")
			}
			currentTaskIndex = taskIndex
		}
		llmID := strings.TrimSpace(result.LLMID)
		if llmID == "" {
			llmID = "default"
		}
		body.WriteString(fmt.Sprintf("### %s Feedback\n\n%s\n\n", llmID, report))
	}
	if body.Len() == 0 {
		return ""
	}

	var out strings.Builder
	out.WriteString("# Dari Docs Feedback\n\n")
	out.WriteString("Run: " + status.ID + "\n")
	out.WriteString("Status: " + status.Status + "\n")
	out.WriteString("Type: " + status.Mode + "\n")
	if status.CompletedAt != nil {
		out.WriteString("Completed: " + status.CompletedAt.UTC().Format(time.RFC3339) + "\n")
	}
	out.WriteString("\n")
	out.WriteString(strings.TrimRight(body.String(), "\n"))
	out.WriteString("\n")
	return out.String()
}
