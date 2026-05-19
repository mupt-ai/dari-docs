package llmoptions

const (
	ClaudeHaiku45  = "claude-haiku-4-5"
	ClaudeSonnet46 = "claude-sonnet-4-6"
	ClaudeOpus47   = "claude-opus-4-7"
	GPT5Mini       = "gpt-5-mini"
	GPT51          = "gpt-5.1"
	GPT55          = "gpt-5.5"

	ManagedDefaultEditorLLMID = ClaudeSonnet46
)

func ClaudeFeedbackLLMIDs() []string {
	return []string{ClaudeHaiku45, ClaudeSonnet46, ClaudeOpus47}
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
