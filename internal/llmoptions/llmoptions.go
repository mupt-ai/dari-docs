package llmoptions

const (
	ClaudeHaiku45  = "claude-haiku-4-5"
	ClaudeSonnet47 = "claude-sonnet-4-7"
	ClaudeOpus46   = "claude-opus-4-6"
	GPT5Mini       = "gpt-5-mini"
	GPT51          = "gpt-5.1"
	GPT55          = "gpt-5.5"

	ManagedDefaultEditorLLMID = ClaudeSonnet47
)

func ClaudeFeedbackLLMIDs() []string {
	return []string{ClaudeHaiku45, ClaudeSonnet47, ClaudeOpus46}
}

func GPTFeedbackLLMIDs() []string {
	return []string{GPT5Mini, GPT51, GPT55}
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
