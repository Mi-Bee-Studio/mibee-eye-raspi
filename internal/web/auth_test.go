package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestServer(user, pass string) *Server {
	return &Server{
		username: user,
		password: pass,
		sessions: NewSessionStore(user, pass),
		logger:   log.New(io.Discard, "", 0),
		loginLimiter: &loginRateLimiter{attempts: make(map[string]*rateLimitEntry)},
	}
}

func TestSessionStore_LoginSuccess(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, err := s.sessions.Login("admin", "admin123")
	if err != nil {
		t.Fatalf("Login failed: %v", err)
	}
	if token == "" {
		t.Fatal("empty token")
	}
	username, err := s.sessions.Validate(token)
	if err != nil {
		t.Fatalf("Validate failed: %v", err)
	}
	if username != "admin" {
		t.Errorf("expected username 'admin', got %q", username)
	}
}

func TestSessionStore_LoginBadPassword(t *testing.T) {
	s := newTestServer("admin", "admin123")
	_, _, err := s.sessions.Login("admin", "wrong")
	if err == nil {
		t.Fatal("expected error for wrong password")
	}
}

func TestSessionStore_LoginBadUsername(t *testing.T) {
	s := newTestServer("admin", "admin123")
	_, _, err := s.sessions.Login("root", "admin123")
	if err == nil {
		t.Fatal("expected error for wrong username")
	}
}

func TestSessionStore_EmptyPassword(t *testing.T) {
	s := newTestServer("admin", "")
	_, _, err := s.sessions.Login("admin", "")
	if err != nil {
		t.Fatalf("Login with empty password should work: %v", err)
	}
}

func TestSessionStore_InvalidateOnLogout(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, _ := s.sessions.Login("admin", "admin123")

	if _, err := s.sessions.Validate(token); err != nil {
		t.Fatalf("expected valid token before logout: %v", err)
	}

	s.sessions.Logout(token)

	if _, err := s.sessions.Validate(token); err == nil {
		t.Fatal("expected token to be invalid after logout")
	}
}

func TestAuthMiddleware_BearerHeader(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, _ := s.sessions.Login("admin", "admin123")

	called := false
	handler := s.authRequired(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
	if called {
		t.Error("handler should not be called without token")
	}

	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr = httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 with valid token, got %d", rr.Code)
	}
	if !called {
		t.Error("handler should be called with valid token")
	}

	called = false
	req = httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer not-a-real-token")
	rr = httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 with invalid token, got %d", rr.Code)
	}
	if called {
		t.Error("handler should not be called with invalid token")
	}
}

func TestAuthMiddleware_QueryToken(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, _ := s.sessions.Login("admin", "admin123")

	called := false
	handler := s.authRequired(func(w http.ResponseWriter, r *http.Request) {
		called = true
	})

	req := httptest.NewRequest("GET", "/ws?token="+token, nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if !called {
		t.Error("handler should be called with ?token= query param")
	}

	req = httptest.NewRequest("GET", "/ws?token=", nil)
	rr = httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 for empty ?token=, got %d", rr.Code)
	}
}

func TestHandleLogin_HTTP(t *testing.T) {
	s := newTestServer("admin", "admin123")

	body := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	token, ok := resp["token"].(string)
	if !ok || token == "" {
		t.Error("expected non-empty token in response")
	}
	if resp["username"] != "admin" {
		t.Errorf("expected username=admin, got %v", resp["username"])
	}
}

func TestHandleLogin_HTTPWrongPassword(t *testing.T) {
	s := newTestServer("admin", "admin123")
	body := `{"username":"admin","password":"WRONG"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", rr.Code)
	}
}

func TestHandleLogin_HTTPEmptyFields(t *testing.T) {
	s := newTestServer("admin", "admin123")
	tests := []struct {
		name string
		body string
	}{
		{"empty username", `{"username":"","password":"admin123"}`},
		{"empty password", `{"username":"admin","password":""}`},
		{"both empty", `{"username":"","password":""}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleLogin(rr, req)
			if rr.Code != http.StatusBadRequest {
				t.Errorf("expected 400, got %d", rr.Code)
			}
		})
	}
}

func TestHandleLogin_HTTPEmptyStoredPassword(t *testing.T) {
	// When no password is configured, login should be rejected with a specific message.
	s := newTestServer("admin", "")
	tests := []struct {
		name string
		body string
	}{
		{"empty credentials", `{"username":"admin","password":""}`},
		{"non-empty password", `{"username":"admin","password":"anything"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("POST", "/api/login", strings.NewReader(tt.body))
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			s.handleLogin(rr, req)
			if rr.Code != http.StatusUnauthorized {
				t.Errorf("expected 401, got %d", rr.Code)
			}
			if !strings.Contains(rr.Body.String(), "Password cannot be empty") {
				t.Errorf("expected message about empty password, got: %s", rr.Body.String())
			}
		})
	}
}

func TestHandleLogin_HTTPInvalidBody(t *testing.T) {
	s := newTestServer("admin", "admin123")
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", rr.Code)
	}
}

func TestHandleLogout_HTTP(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, _ := s.sessions.Login("admin", "admin123")

	req := httptest.NewRequest("POST", "/api/logout", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	s.authRequired(s.handleLogout)(rr, req)

	if rr.Code != http.StatusNoContent {
		t.Errorf("expected 204, got %d", rr.Code)
	}

	_, err := s.sessions.Validate(token)
	if err == nil {
		t.Error("expected token to be invalid after logout")
	}
}

func TestExtractToken_Priorities(t *testing.T) {
	headerToken := "header-token"
	queryToken := "query-token"

	// Both header and query param: header wins, not from query
	req := httptest.NewRequest("GET", "/ws?token="+queryToken, nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	tok, fromQuery := extractToken(req)
	if tok != headerToken {
		t.Errorf("expected header token to win, got %q", tok)
	}
	if fromQuery {
		t.Error("expected fromQuery=false when header wins over query")
	}

	// Only query param
	req = httptest.NewRequest("GET", "/ws?token="+queryToken, nil)
	tok, fromQuery = extractToken(req)
	if tok != queryToken {
		t.Errorf("expected query token, got %q", tok)
	}
	if !fromQuery {
		t.Error("expected fromQuery=true when token is from query param")
	}

	// Only header
	req = httptest.NewRequest("GET", "/ws", nil)
	req.Header.Set("Authorization", "Bearer "+headerToken)
	tok, fromQuery = extractToken(req)
	if tok != headerToken {
		t.Errorf("expected header token, got %q", tok)
	}
	if fromQuery {
		t.Error("expected fromQuery=false when token is from header")
	}

	// No token at all
	req = httptest.NewRequest("GET", "/ws", nil)
	tok, fromQuery = extractToken(req)
	if tok != "" {
		t.Errorf("expected empty token, got %q", tok)
	}
	if fromQuery {
		t.Error("expected fromQuery=false when no token present")
	}
}

func TestSessionStore_LogoutUnknownToken(t *testing.T) {
	s := newTestServer("admin", "admin123")
	s.sessions.Logout("never-issued-token")
}

func TestHandleLogin_ResponseShape(t *testing.T) {
	s := newTestServer("admin", "admin123")
	body := `{"username":"admin","password":"admin123"}`
	req := httptest.NewRequest("POST", "/api/login", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	ct := rr.Header().Get("Content-Type")
	if !strings.Contains(ct, "json") {
		t.Errorf("expected JSON content-type, got %q", ct)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	required := []string{"token", "username", "expires_at", "expires_in"}
	for _, k := range required {
		if _, ok := resp[k]; !ok {
			t.Errorf("response missing %q field: %v", k, resp)
		}
	}
}

func TestRateLimiter_UnderLimit(t *testing.T) {
	s := newTestServer("admin", "admin123")
	ip := "192.168.1.1"

	// 9 failed logins — all should return 401 (not blocked).
	for i := 0; i < 9; i++ {
		req := httptest.NewRequest("POST", "/api/login",
			strings.NewReader(`{"username":"admin","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":12345"
		rr := httptest.NewRecorder()
		s.handleLogin(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d (body: %s)", i+1, rr.Code, rr.Body.String())
		}
	}

	// Verify counter is at 9, not blocked.
	s.loginLimiter.mu.Lock()
	entry := s.loginLimiter.attempts[ip]
	s.loginLimiter.mu.Unlock()
	if entry == nil {
		t.Fatal("expected rate limit entry after 9 failures")
	}
	if entry.count >= maxLoginAttempts {
		t.Errorf("expected count < %d after 9 failures, got %d", maxLoginAttempts, entry.count)
	}
	if !entry.blockedUntil.IsZero() {
		t.Error("expected not blocked after 9 failures")
	}
}

func TestRateLimiter_Blocked(t *testing.T) {
	s := newTestServer("admin", "admin123")
	ip := "192.168.1.1"

	// 10 failed logins.
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("POST", "/api/login",
			strings.NewReader(`{"username":"admin","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":12345"
		rr := httptest.NewRecorder()
		s.handleLogin(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}

	// 11th attempt — should be blocked with 429.
	req := httptest.NewRequest("POST", "/api/login",
		strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":12345"
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusTooManyRequests {
		t.Errorf("expected 429, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestRateLimiter_ResetOnSuccess(t *testing.T) {
	s := newTestServer("admin", "admin123")
	ip := "192.168.1.1"

	// 8 failures.
	for i := 0; i < 8; i++ {
		req := httptest.NewRequest("POST", "/api/login",
			strings.NewReader(`{"username":"admin","password":"wrong"}`))
		req.Header.Set("Content-Type", "application/json")
		req.RemoteAddr = ip + ":12345"
		rr := httptest.NewRecorder()
		s.handleLogin(rr, req)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("attempt %d: expected 401, got %d", i+1, rr.Code)
		}
	}

	// Successful login — resets counter.
	req := httptest.NewRequest("POST", "/api/login",
		strings.NewReader(`{"username":"admin","password":"admin123"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":12345"
	rr := httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 after correct login, got %d", rr.Code)
	}

	// After reset, 9th failure (1st after reset) should return 401, not 429.
	req = httptest.NewRequest("POST", "/api/login",
		strings.NewReader(`{"username":"admin","password":"wrong"}`))
	req.Header.Set("Content-Type", "application/json")
	req.RemoteAddr = ip + ":12345"
	rr = httptest.NewRecorder()
	s.handleLogin(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("expected 401 after reset, got %d (body: %s)", rr.Code, rr.Body.String())
	}
}

func TestSecurityHeaders(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct {
		name    string
		path    string
		wantCSP bool
	}{
		{"root", "/", true},
		{"static css", "/static/style.css", true},
		{"static js", "/static/app.js", true},
		{"api login", "/api/login", false},
		{"api config", "/api/config", false},
		{"ws", "/ws", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Header().Get("X-Frame-Options") != "DENY" {
				t.Error("missing X-Frame-Options: DENY")
			}
			if rr.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Error("missing X-Content-Type-Options: nosniff")
			}
			if rr.Header().Get("Referrer-Policy") != "strict-origin-when-cross-origin" {
				t.Error("missing Referrer-Policy: strict-origin-when-cross-origin")
			}

			csp := rr.Header().Get("Content-Security-Policy")
			if tt.wantCSP && csp == "" {
				t.Errorf("expected CSP header for path %q", tt.path)
			}
			if !tt.wantCSP && csp != "" {
				t.Errorf("unexpected CSP header for path %q: %s", tt.path, csp)
			}
		})
	}
}

func TestAuthMiddleware_BearerHeaderNoDeprecationWarning(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, _ := s.sessions.Login("admin", "admin123")

	handler := s.authRequired(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
	if h := rr.Header().Get("Deprecation-Warning"); h != "" {
		t.Errorf("expected no Deprecation-Warning for Bearer header token, got %q", h)
	}
}

func TestAuthMiddleware_QueryTokenDeprecationWarning(t *testing.T) {
	s := newTestServer("admin", "admin123")
	token, _, _ := s.sessions.Login("admin", "admin123")

	handler := s.authRequired(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	req := httptest.NewRequest("GET", "/api/test?token="+token, nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200 (backward compatible), got %d", rr.Code)
	}
	if h := rr.Header().Get("Deprecation-Warning"); h == "" {
		t.Error("expected Deprecation-Warning header for query token")
	}

}
func TestCredentialStrippedFromURL(t *testing.T) {
	handler := securityHeaders(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	tests := []struct{ name, path string }{
		{"password in query", "/?password=secret"},
		{"username+password", "/?username=admin&password=admin123"},
		{"with other params", "/?password=secret&page=1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tt.path, nil)
			rr := httptest.NewRecorder()
			handler.ServeHTTP(rr, req)

			if rr.Code != http.StatusFound {
				t.Errorf("expected 302 redirect, got %d", rr.Code)
			}
			loc := rr.Header().Get("Location")
			if strings.Contains(loc, "password=") || strings.Contains(loc, "username=") {
				t.Errorf("redirect location still contains credentials: %s", loc)
			}
		})
	}
}
