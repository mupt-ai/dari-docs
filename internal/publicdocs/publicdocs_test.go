package publicdocs

import (
	"strings"
	"testing"
)

func TestNormalizeURLsDeduplicatesAndStripsFragments(t *testing.T) {
	urls, err := NormalizeURLs([]string{"https://docs.dari.dev/llms.txt", "https://docs.dari.dev/llms.txt#ignored"})
	if err != nil {
		t.Fatal(err)
	}
	if len(urls) != 1 || urls[0] != "https://docs.dari.dev/llms.txt" {
		t.Fatalf("urls = %#v, want deduplicated URL without fragment", urls)
	}
}

func TestNormalizeURLsRejectsInvalidURL(t *testing.T) {
	_, err := NormalizeURLs([]string{"file:///etc/passwd"})
	if err == nil || !strings.Contains(err.Error(), "invalid public docs URL") {
		t.Fatalf("err = %v, want URL rejection", err)
	}
}

func TestIsLLMSTextURL(t *testing.T) {
	for _, raw := range []string{"https://docs.dari.dev/llms.txt", "https://docs.dari.dev/llms-full.txt"} {
		if !IsLLMSTextURL(raw) {
			t.Fatalf("IsLLMSTextURL(%q) = false", raw)
		}
	}
}
