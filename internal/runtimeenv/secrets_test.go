package runtimeenv

import (
	"strings"
	"testing"
)

func TestParseJSONNormalizesRuntimeSecrets(t *testing.T) {
	secrets, names, err := ParseJSON(`{" STRIPE_TEST_KEY ":"sk_test","GITHUB_TOKEN":"ghp_test"}`)
	if err != nil {
		t.Fatal(err)
	}
	if secrets["STRIPE_TEST_KEY"] != "sk_test" || secrets["GITHUB_TOKEN"] != "ghp_test" {
		t.Fatalf("secrets = %#v", secrets)
	}
	if strings.Join(names, ",") != "GITHUB_TOKEN,STRIPE_TEST_KEY" {
		t.Fatalf("names = %#v", names)
	}
}

func TestParseJSONRejectsInvalidRuntimeSecretNames(t *testing.T) {
	tests := []struct {
		name string
		raw  string
	}{
		{name: "array", raw: `[]`},
		{name: "null", raw: `null`},
		{name: "empty value", raw: `{"EMPTY":""}`},
		{name: "invalid start", raw: `{"1TOKEN":"value"}`},
		{name: "invalid character", raw: `{"API-KEY":"value"}`},
		{name: "trim duplicate", raw: `{"TOKEN":"one"," TOKEN ":"two"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := ParseJSON(tt.raw); err == nil {
				t.Fatal("expected error")
			}
		})
	}
}
