package llmoptions

const (
	DumbClaude   = "dumb-claude"
	MediumClaude = "medium-claude"
	SmartClaude  = "smart-claude"
	DumbGPT      = "dumb-gpt"
	MediumGPT    = "medium-gpt"
	SmartGPT     = "smart-gpt"
)

func ClaudeFeedbackLLMIDs() []string {
	return []string{DumbClaude, MediumClaude, SmartClaude}
}

func GPTFeedbackLLMIDs() []string {
	return []string{DumbGPT, MediumGPT, SmartGPT}
}

func DefaultFeedbackLLMIDs() []string {
	return append(ClaudeFeedbackLLMIDs(), GPTFeedbackLLMIDs()...)
}

func ManagedDefaultFeedbackLLMIDs() []string {
	return ClaudeFeedbackLLMIDs()
}

func ManagedAllowedLLMIDs() []string {
	return DefaultFeedbackLLMIDs()
}
