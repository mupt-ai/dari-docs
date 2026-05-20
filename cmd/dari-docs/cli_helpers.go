package main

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/mupt-ai/dari-docs/internal/managed"
)

func managedClientWithToken() (*managed.Client, error) {
	auth, err := loadManagedAuthToken()
	if err != nil {
		return nil, err
	}
	return managed.NewWithAuthToken(managed.DefaultBaseURL, auth), nil
}

func managedClientWithAuth() (*managed.Client, managed.AuthToken, error) {
	auth, err := loadManagedAuthToken()
	if err != nil {
		return nil, managed.AuthToken{}, err
	}
	return managed.NewWithAuthToken(managed.DefaultBaseURL, auth), auth, nil
}

func loadManagedToken() (string, error) {
	auth, err := loadManagedAuthToken()
	if err != nil {
		return "", err
	}
	return auth.Token, nil
}

func loadManagedAuthToken() (managed.AuthToken, error) {
	auth, err := managed.LoadAuthToken(managed.DefaultBaseURL)
	if err != nil {
		return managed.AuthToken{}, err
	}
	if auth.Token == "" {
		return managed.AuthToken{}, managedAuthRequiredError()
	}
	return auth, nil
}

func managedAuthRequiredError() error {
	return fmt.Errorf("not logged in to managed service\n\nFor local use:\n  dari-docs auth login\n\nFor CI:\n  dari-docs auth api-key create --name github-actions\n  Set %s in your CI secret store", managed.EnvTokenName)
}

func authSourceLabel(source string) string {
	if source == managed.AuthSourceEnv {
		return managed.EnvTokenName
	}
	return "local credentials"
}

func formatOptionalTime(t *time.Time) string {
	if t == nil || t.IsZero() {
		return "never"
	}
	return t.Local().Format("2006-01-02 15:04")
}

func parseExpiresIn(raw string) (*time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var d time.Duration
	if strings.HasSuffix(raw, "d") {
		days, err := strconv.Atoi(strings.TrimSuffix(raw, "d"))
		if err != nil || days <= 0 {
			return nil, fmt.Errorf("--expires-in must be a positive duration like 90d or 24h")
		}
		d = time.Duration(days) * 24 * time.Hour
	} else {
		parsed, err := time.ParseDuration(raw)
		if err != nil || parsed <= 0 {
			return nil, fmt.Errorf("--expires-in must be a positive duration like 90d or 24h")
		}
		d = parsed
	}
	t := time.Now().UTC().Add(d)
	return &t, nil
}

func parseDollarsToCents(v string) (int64, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0, fmt.Errorf("--amount is required")
	}
	whole, frac, ok := strings.Cut(v, ".")
	if !ok {
		n, err := strconv.ParseInt(whole, 10, 64)
		if err != nil {
			return 0, err
		}
		return n * 100, nil
	}
	n, err := strconv.ParseInt(whole, 10, 64)
	if err != nil {
		return 0, err
	}
	if len(frac) > 2 {
		return 0, fmt.Errorf("--amount can include at most two decimal places")
	}
	for len(frac) < 2 {
		frac += "0"
	}
	cents, err := strconv.ParseInt(frac, 10, 64)
	if err != nil {
		return 0, err
	}
	return n*100 + cents, nil
}

func formatCents(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s$%d.%02d", sign, cents/100, cents%100)
}

func versionLine() string {
	return "dari-docs " + version
}
