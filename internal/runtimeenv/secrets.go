package runtimeenv

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func ValidateName(raw string) (string, error) {
	name := strings.TrimSpace(raw)
	if name == "" {
		return "", fmt.Errorf("runtime secret names must be non-empty")
	}
	if !isEnvNameStart(name[0]) {
		return "", fmt.Errorf("runtime secret name %q must start with a letter or underscore", name)
	}
	for i := 1; i < len(name); i++ {
		if !isEnvNamePart(name[i]) {
			return "", fmt.Errorf("runtime secret name %q must contain only letters, digits, and underscores", name)
		}
	}
	return name, nil
}

func NormalizeMap(values map[string]string) (map[string]string, []string, error) {
	secrets := make(map[string]string, len(values))
	names := make([]string, 0, len(values))
	for rawName, value := range values {
		name, err := ValidateName(rawName)
		if err != nil {
			return nil, nil, err
		}
		if _, ok := secrets[name]; ok {
			return nil, nil, fmt.Errorf("runtime secret name %q is duplicated after normalization", name)
		}
		if value == "" {
			return nil, nil, fmt.Errorf("runtime secret values must be non-empty")
		}
		secrets[name] = value
		names = append(names, name)
	}
	sort.Strings(names)
	return secrets, names, nil
}

func ParseJSON(raw string) (map[string]string, []string, error) {
	var values map[string]string
	if err := json.Unmarshal([]byte(raw), &values); err != nil {
		return nil, nil, fmt.Errorf("runtime_secrets_json must be a JSON object")
	}
	if values == nil {
		return nil, nil, fmt.Errorf("runtime_secrets_json must be a JSON object")
	}
	return NormalizeMap(values)
}

func isEnvNameStart(b byte) bool {
	return b == '_' || (b >= 'A' && b <= 'Z') || (b >= 'a' && b <= 'z')
}

func isEnvNamePart(b byte) bool {
	return isEnvNameStart(b) || (b >= '0' && b <= '9')
}
