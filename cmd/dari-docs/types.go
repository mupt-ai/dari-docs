package main

import (
	"strings"

	"github.com/mupt-ai/dari-docs/internal/runner"
)

type repeated []string

func (r *repeated) String() string     { return strings.Join(*r, ",") }
func (r *repeated) Set(v string) error { *r = append(*r, v); return nil }

func defaultFeedbackLLMIDs() []string {
	return runner.DefaultFeedbackLLMIDs()
}

func defaultClaudeFeedbackLLMIDs() []string {
	return runner.ClaudeFeedbackLLMIDs()
}

func defaultGPTFeedbackLLMIDs() []string {
	return runner.GPTFeedbackLLMIDs()
}
