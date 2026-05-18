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
