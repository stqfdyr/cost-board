package auth

import (
	"testing"
	"time"
)

func newTestAuth(t *testing.T) *Auth {
	t.Helper()
	a, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return a
}

func TestNewWithoutCredentials(t *testing.T) {
	a := newTestAuth(t)
	if a.HasCredentials() {
		t.Fatal("new auth should have no credentials")
	}
}

func TestSetAndLogin(t *testing.T) {
	a := newTestAuth(t)
	if err := a.SetCredentials("testuser", "testpass"); err != nil {
		t.Fatalf("SetCredentials: %v", err)
	}
	if !a.HasCredentials() {
		t.Fatal("should have credentials after SetCredentials")
	}

	token, ok := a.Login("testuser", "testpass")
	if !ok {
		t.Fatal("login should succeed with correct credentials")
	}
	if token == "" {
		t.Fatal("token should not be empty")
	}
}

func TestLoginWrongPassword(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("testuser", "testpass")

	_, ok := a.Login("testuser", "wrongpass")
	if ok {
		t.Fatal("login should fail with wrong password")
	}
}

func TestLoginWrongUsername(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("testuser", "testpass")

	_, ok := a.Login("wronguser", "testpass")
	if ok {
		t.Fatal("login should fail with wrong username")
	}
}

func TestLoginWithoutCredentials(t *testing.T) {
	a := newTestAuth(t)

	_, ok := a.Login("anyuser", "anypass")
	if ok {
		t.Fatal("login should fail when no credentials set")
	}
}

func TestValidateValidToken(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("u", "p")
	token, _ := a.Login("u", "p")

	if !a.Validate(token) {
		t.Fatal("validate should accept valid token")
	}
}

func TestValidateInvalidToken(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("u", "p")

	if a.Validate("nonexistent") {
		t.Fatal("validate should reject unknown token")
	}
}

func TestValidateEmptyToken(t *testing.T) {
	a := newTestAuth(t)
	if a.Validate("") {
		t.Fatal("validate should reject empty token")
	}
}

func TestLogoutInvalidatesToken(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("u", "p")
	token, _ := a.Login("u", "p")

	a.Logout(token)

	if a.Validate(token) {
		t.Fatal("validate should reject token after logout")
	}
}

func TestEachLoginProducesDifferentToken(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("u", "p")

	t1, _ := a.Login("u", "p")
	t2, _ := a.Login("u", "p")

	if t1 == t2 {
		t.Fatal("each login should produce a different token")
	}
	if !a.Validate(t1) || !a.Validate(t2) {
		t.Fatal("both tokens should be valid")
	}
}

func TestValidateExpiredToken(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("u", "p")
	token, _ := a.Login("u", "p")

	// Manually expire the session
	a.mu.Lock()
	a.sessions[token] = time.Now().Add(-time.Hour)
	a.mu.Unlock()

	if a.Validate(token) {
		t.Fatal("validate should reject expired token")
	}

	// Token should be cleaned up after validation
	a.mu.RLock()
	_, exists := a.sessions[token]
	a.mu.RUnlock()
	if exists {
		t.Fatal("expired token should be removed after validation")
	}
}

func TestCleanupExpiredSessions(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("u", "p")
	t1, _ := a.Login("u", "p")
	t2, _ := a.Login("u", "p")

	// Expire t1, keep t2
	a.mu.Lock()
	a.sessions[t1] = time.Now().Add(-time.Hour)
	a.mu.Unlock()

	a.CleanupExpiredSessions()

	if a.Validate(t1) {
		t.Fatal("expired token t1 should not validate")
	}
	if !a.Validate(t2) {
		t.Fatal("valid token t2 should still validate")
	}
}

func TestCredentialsPersistence(t *testing.T) {
	dir := t.TempDir()

	a1, _ := New(dir)
	a1.SetCredentials("persistuser", "persistpass")

	a2, err := New(dir)
	if err != nil {
		t.Fatalf("New (reload): %v", err)
	}
	if !a2.HasCredentials() {
		t.Fatal("credentials should persist across restarts")
	}

	token, ok := a2.Login("persistuser", "persistpass")
	if !ok || token == "" {
		t.Fatal("login should work with persisted credentials")
	}
}

func TestSetCredentialsOverwrites(t *testing.T) {
	a := newTestAuth(t)
	a.SetCredentials("user1", "pass1")
	a.SetCredentials("user2", "pass2")

	_, ok1 := a.Login("user1", "pass1")
	if ok1 {
		t.Fatal("old credentials should not work after overwrite")
	}

	_, ok2 := a.Login("user2", "pass2")
	if !ok2 {
		t.Fatal("new credentials should work after overwrite")
	}
}
