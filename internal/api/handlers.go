package api

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"cost-board/internal/auth"
	"cost-board/internal/store"
)

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type loginResponse struct {
	Token string `json:"token"`
}

const (
	maxBodySize  = 1 << 20
	maxItems     = 1000
)

func handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func handleLogin(authInst *auth.Auth, rl *rateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		ip := getClientIP(r)
		if rl.isBlocked(ip) {
			writeJSON(w, http.StatusTooManyRequests, map[string]string{"error": "too many attempts, try again later"})
			return
		}

		var req loginRequest
		if err := decodeBody(r, &req); err != nil {
			rl.recordFail(ip)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request"})
			return
		}

		token, ok := authInst.Login(req.Username, req.Password)
		if !ok {
			rl.recordFail(ip)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid credentials"})
			return
		}

		rl.reset(ip)
		writeJSON(w, http.StatusOK, loginResponse{Token: token})
	}
}

func handleLogout(authInst *auth.Auth) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		authInst.Logout(token)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	}
}

func handleGetItems(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		items, err := s.GetAll()
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load items"})
			return
		}
		if items == nil {
			items = []store.Item{}
		}
		empty, _ := s.IsEmpty()
		w.Header().Set("X-Initialized", boolStr(!empty))
		writeJSON(w, http.StatusOK, items)
	}
}

func handlePutItems(s *store.Store) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var items []store.Item
		if err := decodeBody(r, &items); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json"})
			return
		}
		if len(items) > maxItems {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "too many items"})
			return
		}
		normalized := normalizeItems(items)
		if err := s.ReplaceAll(normalized); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save items"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "count": len(normalized)})
	}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func decodeBody(r *http.Request, v any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(io.LimitReader(r.Body, maxBodySize+1))
	return dec.Decode(v)
}

func getClientIP(r *http.Request) string {
	if ip := r.Header.Get("X-Forwarded-For"); ip != "" {
		ip = strings.TrimSpace(ip)
		for i := 0; i < len(ip); i++ {
			if ip[i] == ',' {
				return ip[:i]
			}
		}
		return ip
	}
	if ip := r.Header.Get("X-Real-IP"); ip != "" {
		return ip
	}
	host := r.RemoteAddr
	for i := len(host) - 1; i >= 0; i-- {
		if host[i] == ':' {
			return host[:i]
		}
	}
	return host
}

func boolStr(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

func normalizeItems(items []store.Item) []store.Item {
	result := make([]store.Item, 0, len(items))
	for i, it := range items {
		if it.ID == "" {
			it.ID = "cost-" + strconv.FormatInt(int64(time.Now().UnixNano()), 10) + "-" + strconv.Itoa(i)
		}
		if it.Name == "" {
			it.Name = "未命名项目"
		}
		if it.Category == "" {
			it.Category = "未分类"
		}
		if it.Currency == "" {
			it.Currency = "CNY"
		} else {
			it.Currency = toUpper(it.Currency)
		}
		switch it.BillingMonths {
		case 1, 3, 6, 12:
		default:
			it.BillingMonths = 12
		}
		it.Order = i
		result = append(result, it)
	}
	return result
}

func toUpper(s string) string {
	b := []byte(s)
	for i := range b {
		if b[i] >= 'a' && b[i] <= 'z' {
			b[i] -= 32
		}
	}
	return string(b)
}
