package api

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"cost-board/internal/auth"
)

type rateLimiter struct {
	mu    sync.Mutex
	fails map[string]*failState
}

type failState struct {
	count    int
	lastFail time.Time
}

const (
	maxFails     = 5
	failWindow   = time.Minute
	blockTimeout = time.Minute
)

func newRateLimiter() *rateLimiter {
	return &rateLimiter{fails: make(map[string]*failState)}
}

func (rl *rateLimiter) isBlocked(ip string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	st, ok := rl.fails[ip]
	if !ok {
		return false
	}

	if time.Since(st.lastFail) > blockTimeout {
		delete(rl.fails, ip)
		return false
	}

	return st.count >= maxFails
}

func (rl *rateLimiter) recordFail(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	st, ok := rl.fails[ip]
	if !ok || time.Since(st.lastFail) > failWindow {
		rl.fails[ip] = &failState{count: 1, lastFail: time.Now()}
		return
	}
	st.count++
	st.lastFail = time.Now()
}

func (rl *rateLimiter) reset(ip string) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	delete(rl.fails, ip)
}

func extractToken(r *http.Request) string {
	header := r.Header.Get("Authorization")
	if header == "" {
		return ""
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || strings.ToLower(parts[0]) != "bearer" {
		return ""
	}
	return strings.TrimSpace(parts[1])
}

func authMiddleware(auth *auth.Auth, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if token == "" || !auth.Validate(token) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next(w, r)
	}
}
