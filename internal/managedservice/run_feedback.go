package managedservice

import (
	"context"
	"fmt"
	"strings"

	"github.com/mupt-ai/dari-docs/internal/dari"
	"github.com/mupt-ai/dari-docs/internal/runner"
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

func managedAggregateFeedback(results []runFeedbackResult, tasks []string) string {
	if len(results) == 0 {
		return ""
	}
	return runner.AggregateFeedbackByTask(runnerFeedbackResults(results, tasks))
}

func runnerFeedbackResults(results []runFeedbackResult, tasks []string) []runner.FeedbackResult {
	out := make([]runner.FeedbackResult, 0, len(results))
	for _, result := range results {
		out = append(out, runner.FeedbackResult{
			TaskIndex: result.TaskIndex,
			Task:      taskLabel(tasks, result.TaskIndex),
			LLMID:     result.LLMID,
			Report:    result.Report,
		})
	}
	return out
}

func taskLabel(tasks []string, taskIndex int) string {
	if taskIndex > 0 && taskIndex <= len(tasks) {
		return strings.TrimSpace(tasks[taskIndex-1])
	}
	return ""
}
