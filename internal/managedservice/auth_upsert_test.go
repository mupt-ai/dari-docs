package managedservice

import (
	"context"
	"testing"
)

func TestUpsertUserForDariIdentityUsesStableAuthSubject(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	first, err := upsertUserForDariIdentity(ctx, tx, dariUserInfo{
		AuthSubject: "sub_stable",
		Email:       "first@example.test",
		DisplayName: "First",
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := upsertUserForDariIdentity(ctx, tx, dariUserInfo{
		AuthSubject: "sub_stable",
		Email:       "second@example.test",
		DisplayName: "Second",
	})
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("user id changed: first=%s second=%s", first, second)
	}
	var email, displayName string
	if err := tx.QueryRow(ctx, `SELECT email, display_name FROM users WHERE id=$1`, first).Scan(&email, &displayName); err != nil {
		t.Fatal(err)
	}
	if email != "second@example.test" || displayName != "Second" {
		t.Fatalf("user row = email %q display %q", email, displayName)
	}
}

func TestUpsertUserForDariIdentityKeepsStableUserOnEmailConflict(t *testing.T) {
	db := openManagedServiceTestDB(t)
	ctx := context.Background()
	tx, err := db.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	suffix := randomToken(8)
	userA := "usr_email_conflict_a_" + suffix
	userB := "usr_email_conflict_b_" + suffix
	subjectA := "sub_email_conflict_a_" + suffix
	subjectB := "sub_email_conflict_b_" + suffix
	emailA := "first-" + suffix + "@example.test"
	emailB := "second-" + suffix + "@example.test"
	if _, err := tx.Exec(ctx, `
INSERT INTO users (id, auth_subject, email, display_name)
VALUES
  ($1, $2, $3, 'First'),
  ($4, $5, $6, 'Second')
`, userA, subjectA, emailA, userB, subjectB, emailB); err != nil {
		t.Fatal(err)
	}

	got, err := upsertUserForDariIdentity(ctx, tx, dariUserInfo{
		AuthSubject: subjectA,
		Email:       emailB,
		DisplayName: "First Updated",
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != userA {
		t.Fatalf("user id = %q, want %s", got, userA)
	}
	var email, displayName string
	if err := tx.QueryRow(ctx, `SELECT email, display_name FROM users WHERE id=$1`, got).Scan(&email, &displayName); err != nil {
		t.Fatal(err)
	}
	if email != emailA || displayName != "First Updated" {
		t.Fatalf("user row = email %q display %q", email, displayName)
	}
}
