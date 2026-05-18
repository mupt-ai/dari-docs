package managedservice

import (
	"encoding/json"
	"fmt"
	"strings"
)

var (
	managedDefaultTesterLLMIDs = []string{"dumb-claude", "medium-claude", "smart-claude"}
	managedAllowedLLMIDs       = []string{"dumb-claude", "medium-claude", "smart-claude", "dumb-gpt", "medium-gpt", "smart-gpt"}
)

func managedLLMIDOrDefault(llmID string) string {
	if llmID == "" {
		return managedDefaultLLMID
	}
	return llmID
}

func allowedManagedLLMIDs() []string {
	return append([]string{}, managedAllowedLLMIDs...)
}

func defaultManagedTesterLLMIDs() []string {
	return append([]string{}, managedDefaultTesterLLMIDs...)
}

func normalizeManagedLLMID(llmID string) (string, error) {
	llmID = strings.TrimSpace(llmID)
	if llmID == "" {
		return managedDefaultLLMID, nil
	}
	for _, allowed := range managedAllowedLLMIDs {
		if llmID == allowed {
			return llmID, nil
		}
	}
	return "", fmt.Errorf("managed mode supports only these LLM IDs: %s", strings.Join(managedAllowedLLMIDs, ", "))
}

func normalizeManagedLLMIDs(llmIDs []string) ([]string, error) {
	if len(llmIDs) == 0 {
		return defaultManagedTesterLLMIDs(), nil
	}
	out := make([]string, 0, len(llmIDs))
	seen := map[string]bool{}
	for _, raw := range llmIDs {
		llmID, err := normalizeManagedLLMID(raw)
		if err != nil {
			return nil, err
		}
		if seen[llmID] {
			continue
		}
		seen[llmID] = true
		out = append(out, llmID)
	}
	if len(out) == 0 {
		return defaultManagedTesterLLMIDs(), nil
	}
	return out, nil
}

func parseManagedLLMIDsJSON(raw string) ([]string, error) {
	var llmIDs []string
	if err := json.Unmarshal([]byte(raw), &llmIDs); err != nil {
		return nil, fmt.Errorf("feedback_llm_ids_json must be a JSON string array")
	}
	return normalizeManagedLLMIDs(llmIDs)
}
