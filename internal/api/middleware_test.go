package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"cost-board/internal/auth"
)

func TestRateLimiterNotBlockedInitially(t *testing.T) {
	rl := newRateLimiter()
	if rl.isBlocked("1.2.3.4") {
		t.Fatal("new limiter should not block")
	}
}

func TestRateLimiterBlocksAfterMaxFails(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < 5; i++ {
		rl.recordFail("1.2.3.4")
	}
	if !rl.isBlocked("1.2.3.4") {
		t.Fatal("should be blocked after 5 fails")
	}
}

func TestRateLimiterDifferentIPsIndependent(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < 5; i++ {
		rl.recordFail("1.2.3.4")
	}
	if !rl.isBlocked("1.2.3.4") {
		t.Fatal("1.2.3.4 should be blocked")
	}
	if rl.isBlocked("5.6.7.8") {
		t.Fatal("5.6.7.8 should not be blocked")
	}
}

func TestRateLimiterReset(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < 5; i++ {
		rl.recordFail("1.2.3.4")
	}
	rl.reset("1.2.3.4")
	if rl.isBlocked("1.2.3.4") {
		t.Fatal("should not be blocked after reset")
	}
}

func TestAuthMiddlewareAllowsValidToken(t *testing.T) {
	dir := t.TempDir()
	a, _ := auth.New(dir)
	a.SetCredentials("u", "p")
	token, _ := a.Login("u", "p")

	called := false
	h := authMiddleware(a, func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(200)
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	h(w, req)

	if !called {
		t.Fatal("handler should be called with valid token")
	}
}

func TestAuthMiddlewareRejectsNoToken(t *testing.T) {
	dir := t.TempDir()
	a, _ := auth.New(dir)
	a.SetCredentials("u", "p")

	called := false
	h := authMiddleware(a, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h(w, req)

	if called {
		t.Fatal("handler should not be called without token")
	}
	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestAuthMiddlewareRejectsInvalidToken(t *testing.T) {
	dir := t.TempDir()
	a, _ := auth.New(dir)
	a.SetCredentials("u", "p")

	called := false
	h := authMiddleware(a, func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalidtoken")
	w := httptest.NewRecorder()
	h(w, req)

	if called {
		t.Fatal("handler should not be called with invalid token")
	}
	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}
