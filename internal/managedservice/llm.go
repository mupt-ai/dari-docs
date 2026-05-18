package managedservice

import (
	"encoding/json"
	"fmt"
	"strings"
)

var managedAllowedLLMIDs = []string{"dumb-claude", "medium-claude", "smart-claude"}

func managedLLMIDOrDefault(llmID string) string {
	if llmID == "" {
		return managedDefaultLLMID
	}
	return llmID
}

func allowedManagedLLMIDs() []string {
	return append([]string{}, managedAllowedLLMIDs...)
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
	return "", fmt.Errorf("managed mode supports only Claude LLM IDs: %s", strings.Join(managedAllowedLLMIDs, ", "))
}

func normalizeManagedLLMIDs(llmIDs []string) ([]string, error) {
	if len(llmIDs) == 0 {
		return allowedManagedLLMIDs(), nil
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
		return allowedManagedLLMIDs(), nil
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
