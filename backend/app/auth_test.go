package app

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCreateTemporaryUserCreatesRandomCredentials(t *testing.T) {
	service, store := newAuthTestService(t)

	expiresAt := time.Now().UTC().Add(2 * time.Hour)
	user, password, err := service.CreateTemporaryUser(expiresAt, []Permission{PermissionUpload, PermissionDownload})
	if err != nil {
		t.Fatalf("CreateTemporaryUser() error = %v", err)
	}
	if user.Username == "" || len(user.Username) <= len("tmp-") {
		t.Fatalf("expected random temporary username, got %q", user.Username)
	}
	if password == "" {
		t.Fatal("expected random temporary password")
	}
	if !user.Temporary {
		t.Fatal("expected temporary user flag to be set")
	}
	if user.ExpiresAt == nil || !user.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("expected expiresAt %v, got %v", expiresAt, user.ExpiresAt)
	}
	if user.Role != RoleUser {
		t.Fatalf("expected role %q, got %q", RoleUser, user.Role)
	}
	if len(user.Permissions) != 2 || user.Permissions[0] != PermissionUpload || user.Permissions[1] != PermissionDownload {
		t.Fatalf("unexpected permissions: %#v", user.Permissions)
	}

	state := store.snapshot()
	if len(state.Users) != 1 {
		t.Fatalf("expected 1 stored user, got %d", len(state.Users))
	}
	if state.Users[0].PasswordHash == "" {
		t.Fatal("expected password hash to be stored")
	}
	if state.Users[0].Username != user.Username {
		t.Fatalf("expected stored username %q, got %q", user.Username, state.Users[0].Username)
	}
}

func TestTemporaryUserExpiryInvalidatesSignInAndSession(t *testing.T) {
	service, store := newAuthTestService(t)

	expiresAt := time.Now().UTC().Add(time.Hour)
	user, password, err := service.CreateTemporaryUser(expiresAt, []Permission{PermissionDownload})
	if err != nil {
		t.Fatalf("CreateTemporaryUser() error = %v", err)
	}
	token, _, err := service.SignIn(user.Username, password)
	if err != nil {
		t.Fatalf("SignIn() error = %v", err)
	}

	expiredAt := time.Now().UTC().Add(-time.Minute)
	if err := store.update(func(state *State) error {
		for i := range state.Users {
			if state.Users[i].Username == user.Username {
				state.Users[i].ExpiresAt = &expiredAt
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("expire temporary user: %v", err)
	}

	if _, err := service.Authenticate(token); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("Authenticate() error = %v, want %v", err, ErrUnauthorized)
	}
	if _, _, err := service.SignIn(user.Username, password); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("SignIn() after expiry error = %v, want %v", err, ErrInvalidCredentials)
	}
	if remaining := store.snapshot().Users; len(remaining) != 0 {
		t.Fatalf("expected expired temporary user to be pruned, got %d users", len(remaining))
	}
	if remaining := store.snapshot().Sessions; len(remaining) != 0 {
		t.Fatalf("expected expired temporary sessions to be pruned, got %d sessions", len(remaining))
	}
}

func TestCreateTemporaryUserRejectsPastExpiry(t *testing.T) {
	service, _ := newAuthTestService(t)

	_, _, err := service.CreateTemporaryUser(time.Now().UTC().Add(-time.Minute), []Permission{PermissionUpload})
	if !errors.Is(err, ErrInvalidUserExpiry) {
		t.Fatalf("CreateTemporaryUser() error = %v, want %v", err, ErrInvalidUserExpiry)
	}
}

func newAuthTestService(t *testing.T) (*Service, *Store) {
	t.Helper()

	dir := t.TempDir()
	store, err := NewStore(filepath.Join(dir, "state.json"))
	if err != nil {
		t.Fatalf("NewStore() error = %v", err)
	}
	t.Cleanup(func() {
		_ = os.RemoveAll(dir)
	})
	return NewService(nil, store), store
}
