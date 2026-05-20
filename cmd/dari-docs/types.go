package main

import "github.com/mupt-ai/dari-docs/internal/runner"

func defaultFeedbackLLMIDs() []string {
	return runner.DefaultFeedbackLLMIDs()
}

func defaultClaudeFeedbackLLMIDs() []string {
	return runner.ClaudeFeedbackLLMIDs()
}

func defaultGPTFeedbackLLMIDs() []string {
	return runner.GPTFeedbackLLMIDs()
}
