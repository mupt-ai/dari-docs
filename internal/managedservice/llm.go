package managedservice

func managedLLMIDOrDefault(llmID string) string {
	if llmID == "" {
		return managedDefaultLLMID
	}
	return llmID
}
