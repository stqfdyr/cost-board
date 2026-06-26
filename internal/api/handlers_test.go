package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cost-board/internal/auth"
	"cost-board/internal/store"
)

func newTestServer(t *testing.T) (*store.Store, *auth.Auth, *rateLimiter) {
	t.Helper()
	dir := t.TempDir()
	s, err := store.New(dir+"/test.db", dir)
	if err != nil {
		t.Fatalf("store.New: %v", err)
	}
	t.Cleanup(func() { s.Close() })
	a, err := auth.New(dir)
	if err != nil {
		t.Fatalf("auth.New: %v", err)
	}
	a.SetCredentials("testuser", "testpass")
	return s, a, newRateLimiter()
}

func doRequest(t *testing.T, handler http.HandlerFunc, method, path string, body string, token string) *httptest.ResponseRecorder {
	t.Helper()
	var bodyReader *strings.Reader
	if body != "" {
		bodyReader = strings.NewReader(body)
	} else {
		bodyReader = strings.NewReader("")
	}
	var req *http.Request
	if bodyReader != nil {
		req = httptest.NewRequest(method, path, bodyReader)
	} else {
		req = httptest.NewRequest(method, path, nil)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	handler(w, req)
	return w
}

func TestHandleHealth(t *testing.T) {
	w := doRequest(t, handleHealth, "GET", "/api/health", "", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var resp map[string]bool
	json.Unmarshal(w.Body.Bytes(), &resp)
	if !resp["ok"] {
		t.Fatal("expected ok: true")
	}
}

func TestHandleLoginSuccess(t *testing.T) {
	_, a, rl := newTestServer(t)
	h := handleLogin(a, rl)
	w := doRequest(t, h, "POST", "/api/login", `{"username":"testuser","password":"testpass"}`, "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	var resp loginResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Token == "" {
		t.Fatal("token should not be empty")
	}
}

func TestHandleLoginWrongPassword(t *testing.T) {
	_, a, rl := newTestServer(t)
	h := handleLogin(a, rl)
	w := doRequest(t, h, "POST", "/api/login", `{"username":"testuser","password":"wrong"}`, "")
	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHandleLoginInvalidJSON(t *testing.T) {
	_, a, rl := newTestServer(t)
	h := handleLogin(a, rl)
	w := doRequest(t, h, "POST", "/api/login", "not json", "")
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleLoginRateLimit(t *testing.T) {
	_, a, rl := newTestServer(t)
	h := handleLogin(a, rl)
	for i := 0; i < 5; i++ {
		doRequest(t, h, "POST", "/api/login", `{"username":"testuser","password":"wrong"}`, "")
	}
	w := doRequest(t, h, "POST", "/api/login", `{"username":"testuser","password":"testpass"}`, "")
	if w.Code != 429 {
		t.Fatalf("after 5 fails, status = %d, want 429", w.Code)
	}
}

func TestHandleGetItemsPublic(t *testing.T) {
	s, _, _ := newTestServer(t)
	h := handleGetItems(s)
	w := doRequest(t, h, "GET", "/api/items", "", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var items []store.Item
	json.Unmarshal(w.Body.Bytes(), &items)
	if items != nil && len(items) != 0 {
		t.Fatalf("expected empty or nil, got %d items", len(items))
	}
	if w.Header().Get("X-Initialized") != "0" {
		t.Fatalf("X-Initialized = %q, want 0", w.Header().Get("X-Initialized"))
	}
}

func TestHandleGetItemsAfterInsert(t *testing.T) {
	s, _, _ := newTestServer(t)
	s.ReplaceAll([]store.Item{
		{ID: "x", Name: "X", Category: "c", Amount: 1, Currency: "CNY", BillingMonths: 12, Enabled: true, Order: 0},
	})
	h := handleGetItems(s)
	w := doRequest(t, h, "GET", "/api/items", "", "")
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if w.Header().Get("X-Initialized") != "1" {
		t.Fatalf("X-Initialized = %q, want 1", w.Header().Get("X-Initialized"))
	}
	var items []store.Item
	json.Unmarshal(w.Body.Bytes(), &items)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
}

func TestHandlePutItemsWithAuth(t *testing.T) {
	s, a, _ := newTestServer(t)
	token, _ := a.Login("testuser", "testpass")
	h := authMiddleware(a, handlePutItems(s))
	w := doRequest(t, h, "PUT", "/api/items", `[{"id":"a","name":"A","category":"c","amount":1,"currency":"usd","billingMonths":12,"enabled":true,"order":0}]`, token)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200; body: %s", w.Code, w.Body.String())
	}
	got, _ := s.GetAll()
	if len(got) != 1 {
		t.Fatalf("expected 1 item in store, got %d", len(got))
	}
	if got[0].Currency != "USD" {
		t.Fatalf("currency should be uppercased, got %q", got[0].Currency)
	}
}

func TestHandlePutItemsWithoutAuth(t *testing.T) {
	s, a, _ := newTestServer(t)
	h := authMiddleware(a, handlePutItems(s))
	w := doRequest(t, h, "PUT", "/api/items", `[]`, "")
	if w.Code != 401 {
		t.Fatalf("status = %d, want 401", w.Code)
	}
}

func TestHandlePutItemsInvalidJSON(t *testing.T) {
	s, a, _ := newTestServer(t)
	token, _ := a.Login("testuser", "testpass")
	h := authMiddleware(a, handlePutItems(s))
	w := doRequest(t, h, "PUT", "/api/items", "not json", token)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandlePutItemsNonArray(t *testing.T) {
	s, a, _ := newTestServer(t)
	token, _ := a.Login("testuser", "testpass")
	h := authMiddleware(a, handlePutItems(s))
	w := doRequest(t, h, "PUT", "/api/items", `{"not":"array"}`, token)
	if w.Code != 400 {
		t.Fatalf("status = %d, want 400", w.Code)
	}
}

func TestHandleLogout(t *testing.T) {
	_, a, _ := newTestServer(t)
	token, _ := a.Login("testuser", "testpass")
	h := authMiddleware(a, handleLogout(a))
	w := doRequest(t, h, "POST", "/api/logout", "", token)
	if w.Code != 200 {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	if a.Validate(token) {
		t.Fatal("token should be invalid after logout")
	}
}

func TestNormalizeItems(t *testing.T) {
	items := []store.Item{
		{Name: "A", Category: "c", Amount: 1, Currency: "usd", BillingMonths: 12, Enabled: true, Order: 99},
		{ID: "b", Name: "", Category: "", Amount: 2, Currency: "", BillingMonths: 99, Enabled: true, Order: 99},
	}
	result := normalizeItems(items)

	if result[0].ID == "" {
		t.Fatal("empty ID should be generated")
	}
	if result[0].Currency != "USD" {
		t.Fatalf("currency should be uppercased, got %q", result[0].Currency)
	}
	if result[0].Order != 0 {
		t.Fatalf("order should be 0 (index), got %d", result[0].Order)
	}

	if result[1].ID != "b" {
		t.Fatalf("existing ID should be preserved, got %q", result[1].ID)
	}
	if result[1].Name != "未命名项目" {
		t.Fatalf("empty name should default, got %q", result[1].Name)
	}
	if result[1].Category != "未分类" {
		t.Fatalf("empty category should default, got %q", result[1].Category)
	}
	if result[1].Currency != "CNY" {
		t.Fatalf("empty currency should default to CNY, got %q", result[1].Currency)
	}
	if result[1].BillingMonths != 12 {
		t.Fatalf("invalid billingMonths should default to 12, got %d", result[1].BillingMonths)
	}
	if result[1].Order != 1 {
		t.Fatalf("order should be 1 (index), got %d", result[1].Order)
	}
}

func TestNormalizeItemsEmpty(t *testing.T) {
	result := normalizeItems([]store.Item{})
	if len(result) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result))
	}
}

func TestNormalizeItemsNil(t *testing.T) {
	result := normalizeItems(nil)
	if len(result) != 0 {
		t.Fatalf("expected 0 items, got %d", len(result))
	}
}

func TestToUpper(t *testing.T) {
	tests := []struct{ in, want string }{
		{"usd", "USD"},
		{"USD", "USD"},
		{"cny", "CNY"},
		{"eur", "EUR"},
		{"", ""},
	}
	for _, tt := range tests {
		if got := toUpper(tt.in); got != tt.want {
			t.Errorf("toUpper(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "1" {
		t.Fatal("boolStr(true) should be \"1\"")
	}
	if boolStr(false) != "0" {
		t.Fatal("boolStr(false) should be \"0\"")
	}
}

func TestGetClientIPXForwardedFor(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4, 5.6.7.8")
	if got := getClientIP(req); got != "1.2.3.4" {
		t.Fatalf("got %q, want 1.2.3.4", got)
	}
}

func TestGetClientIPXForwardedForSingle(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "1.2.3.4")
	if got := getClientIP(req); got != "1.2.3.4" {
		t.Fatalf("got %q, want 1.2.3.4", got)
	}
}

func TestGetClientIPXRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "9.8.7.6")
	if got := getClientIP(req); got != "9.8.7.6" {
		t.Fatalf("got %q, want 9.8.7.6", got)
	}
}

func TestGetClientIPRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	if got := getClientIP(req); got != "192.168.1.1" {
		t.Fatalf("got %q, want 192.168.1.1", got)
	}
}

func TestGetClientIPXForwardedForTrimmed(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", " 1.2.3.4 ")
	if got := getClientIP(req); got != "1.2.3.4" {
		t.Fatalf("got %q, want 1.2.3.4 (trimmed)", got)
	}
}

func TestExtractTokenValid(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer abc123")
	if got := extractToken(req); got != "abc123" {
		t.Fatalf("got %q, want abc123", got)
	}
}

func TestExtractTokenCaseInsensitive(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "bearer abc123")
	if got := extractToken(req); got != "abc123" {
		t.Fatalf("got %q, want abc123", got)
	}
}

func TestExtractTokenMissing(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	if got := extractToken(req); got != "" {
		t.Fatalf("got %q, want empty", got)
	}
}

func TestExtractTokenWrongScheme(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Basic abc123")
	if got := extractToken(req); got != "" {
		t.Fatalf("got %q, want empty for non-bearer", got)
	}
}

func TestExtractTokenEmptyValue(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer ")
	if got := extractToken(req); got != "" {
		t.Fatalf("got %q, want empty for bearer with empty token", got)
	}
}
