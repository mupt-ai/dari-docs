package main

import "strings"

func uniqueTrimmedList(values []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, raw := range values {
		v := strings.TrimSpace(raw)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	return out
}

func expandFeedbackLLMList(values []string) []string {
	parts := uniqueTrimmedList(values)
	if len(parts) == 0 {
		return nil
	}
	var out []string
	seen := map[string]bool{}
	add := func(llmID string) {
		if llmID == "" || seen[llmID] {
			return
		}
		seen[llmID] = true
		out = append(out, llmID)
	}
	for _, part := range parts {
		switch strings.ToLower(part) {
		case "all":
			for _, llmID := range defaultFeedbackLLMIDs() {
				add(llmID)
			}
		case "claude":
			for _, llmID := range defaultClaudeFeedbackLLMIDs() {
				add(llmID)
			}
		case "gpt":
			for _, llmID := range defaultGPTFeedbackLLMIDs() {
				add(llmID)
			}
		default:
			add(part)
		}
	}
	return out
}
