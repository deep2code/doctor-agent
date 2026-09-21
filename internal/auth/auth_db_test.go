package auth

import (
	"errors"
	"strings"
	"testing"

	"github.com/doctor-agent/internal/config"
	"github.com/doctor-agent/internal/database"
)

// openTestAppDB connects to the scratch application database the auth tests
// share with internal/server (locally: MARIA_DB_PORT=3307 go test ./...).
// It skips the calling test when no instance is reachable, since credential
// storage is the whole subject here.
func openTestAppDB(t *testing.T) *database.DB {
	t.Helper()
	t.Setenv("MARIA_DB_APP_DB", "doctor_agent_test_auth")
	t.Setenv("MARIA_DB_KNOWLEDGE_DB", "doctor_knowledge")
	cfg := config.Load()
	if err := cfg.EnsureAppDB(); err != nil {
		t.Skipf("cannot create app db: %v", err)
	}
	db, err := database.New(database.Config{DSN: cfg.AppDBDSN()})
	if err != nil {
		t.Skipf("no local MariaDB: %v", err)
	}
	t.Cleanup(func() { db.Close() })
	return db
}

func dropUserByUsername(t *testing.T, db *database.DB, username string) {
	t.Helper()
	existing, err := db.GetUserByUsername(username)
	if err != nil {
		t.Fatalf("looking up %s: %v", username, err)
	}
	if existing != nil {
		if err := db.DeleteUser(existing.ID); err != nil {
			t.Fatalf("dropping stale %s: %v", username, err)
		}
	}
}

// TestLoginUpgradesLegacyHash covers the migration path for accounts created
// before PBKDF2: they must keep working, and the first successful login has to
// replace the fast, unsalted-sha256 verifier with the current one.
func TestLoginUpgradesLegacyHash(t *testing.T) {
	db := openTestAppDB(t)
	svc := NewService(db, "test-auth-secret")

	const (
		username = "legacy-hash-user"
		password = "一个旧格式的口令"
		legacy   = "0123456789abcdef"
	)
	dropUserByUsername(t, db, username)
	id, err := generateID()
	if err != nil {
		t.Fatalf("generateID: %v", err)
	}
	if err := db.CreateUser(&database.User{
		ID: id, Username: username, Nickname: "旧哈希用户",
		PasswordHash: legacyHash(legacy, password),
	}); err != nil {
		t.Fatalf("creating a legacy user: %v", err)
	}
	t.Cleanup(func() { _ = db.DeleteUser(id) })

	user, err := svc.Login(&LoginInput{Username: username, Password: password})
	if err != nil {
		t.Fatalf("logging in with a legacy verifier: %v", err)
	}
	if user.ID != id {
		t.Fatalf("Login returned %q, want %q", user.ID, id)
	}

	stored, err := db.GetUser(id)
	if err != nil {
		t.Fatalf("re-reading the user: %v", err)
	}
	if !strings.HasPrefix(stored.PasswordHash, pbkdf2Prefix) {
		t.Fatalf("verifier still in the legacy format: %q", stored.PasswordHash)
	}

	// The upgraded verifier authenticates on its own...
	if _, err := svc.Login(&LoginInput{Username: username, Password: password}); err != nil {
		t.Fatalf("login after upgrade: %v", err)
	}
	// ...and a wrong password is still refused.
	if _, err := svc.Login(&LoginInput{Username: username, Password: password + "x"}); err == nil {
		t.Error("wrong password accepted after upgrade")
	}
	// A second login must not re-hash: PBKDF2 hashes are salted, so the stored
	// value changing again would mean every login pays the KDF twice.
	again, err := db.GetUser(id)
	if err != nil {
		t.Fatalf("re-reading the user after the second login: %v", err)
	}
	if again.PasswordHash != stored.PasswordHash {
		t.Error("verifier rewritten on a login that was not legacy")
	}

	// Deleting the account revokes tokens already in the wild.
	token := svc.IssueToken(id)
	if u, err := svc.GetUserByToken(token); err != nil || u.ID != id {
		t.Fatalf("GetUserByToken = (%v, %v), want the user", u, err)
	}
	if err := db.DeleteUser(id); err != nil {
		t.Fatalf("deleting the user: %v", err)
	}
	if u, err := svc.GetUserByToken(token); !errors.Is(err, ErrInvalidToken) || u != nil {
		t.Errorf("token of a deleted user = (%v, %v), want (nil, ErrInvalidToken)", u, err)
	}
}

// TestAdminCreateUserStoresStrongVerifier checks the other side of the same
// storage: an admin-created account must never land in the legacy format.
func TestAdminCreateUserStoresStrongVerifier(t *testing.T) {
	db := openTestAppDB(t)
	svc := NewService(db, "test-auth-secret")

	const username = "created-by-admin-user"
	dropUserByUsername(t, db, username)
	// AdminCreateUser only reads createdBy.IsAdmin, so the creator is a value,
	// not a row.
	creator := &database.User{ID: "bootstrap-auth-test", Username: "bootstrap", IsAdmin: true}

	created, err := svc.AdminCreateUser(&AdminCreateUserInput{
		Username: username, Password: "a-good-password", Nickname: "新建用户",
	}, creator)
	if err != nil {
		t.Fatalf("AdminCreateUser: %v", err)
	}
	t.Cleanup(func() { _ = db.DeleteUser(created.ID) })

	stored, err := db.GetUser(created.ID)
	if err != nil {
		t.Fatalf("re-reading the new user: %v", err)
	}
	if !strings.HasPrefix(stored.PasswordHash, pbkdf2Prefix) {
		t.Errorf("new account stored a weak verifier: %q", stored.PasswordHash)
	}
	if _, err := svc.Login(&LoginInput{Username: username, Password: "a-good-password"}); err != nil {
		t.Errorf("login on a fresh account: %v", err)
	}
}
