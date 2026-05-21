package redact

import (
	"encoding/json"
	"net/url"
	"strings"
	"testing"
)

func TestSecretsRedactorMasksRawURLEncodedAndJSONEscapedValues(t *testing.T) {
	secret := `sk test/+"token"`
	jsonSecret, err := json.Marshal(secret)
	if err != nil {
		t.Fatal(err)
	}
	redactor := NewSecrets(map[string]string{"STRIPE_TEST_SECRET_KEY": secret})
	input := "raw " + secret + " url " + url.QueryEscape(secret) + " json " + string(jsonSecret[1:len(jsonSecret)-1])
	got := redactor.String(input)
	if strings.Contains(got, "sk") || strings.Contains(got, "%2F") {
		t.Fatalf("redacted string still contains secret material: %q", got)
	}
	if count := strings.Count(got, replacement); count != 3 {
		t.Fatalf("redaction count = %d, want 3 in %q", count, got)
	}
}

func TestSecretsRedactorNoopWithoutSecrets(t *testing.T) {
	redactor := NewSecrets(nil)
	if got := redactor.String("hello"); got != "hello" {
		t.Fatalf("String() = %q, want hello", got)
	}
}
