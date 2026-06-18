package web

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	sessionTTL  = 24 * time.Hour
	cleanupTick = 1 * time.Hour
)

// loginRateLimiter tracks failed login attempts per IP.
type loginRateLimiter struct {
	mu       sync.Mutex
	attempts map[string]*rateLimitEntry
}

type rateLimitEntry struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

const (
	maxLoginAttempts   = 10
	loginWindow        = 1 * time.Minute
	loginBlockDuration = 5 * time.Minute
)

// session represents an authenticated user session.
type session struct {
	username  string
	createdAt time.Time
	expiresAt time.Time
}

// SessionStore manages in-memory bearer-token sessions.
type SessionStore struct {
	mu       sync.RWMutex
	sessions map[string]session

	username string
	password string
}

// NewSessionStore creates a new session store with the given credentials.
// Tokens are issued via Login and validated via Validate.
// Returns an error if the password is empty.
func NewSessionStore(username, password string) (*SessionStore, error) {
	if password == "" {
		return nil, errors.New("web: password must not be empty. Set web.password or MIBEE_EYE_WEB_PASSWORD")
	}
	s := &SessionStore{
		sessions: make(map[string]session),
		username: username,
		password: password,
	}
	go s.cleanup()
	return s, nil
}

// Login validates credentials and returns a new bearer token on success.
func (s *SessionStore) Login(user, pass string) (string, time.Time, error) {
	userMatch := subtle.ConstantTimeCompare([]byte(user), []byte(s.username)) == 1
	passMatch := subtle.ConstantTimeCompare([]byte(pass), []byte(s.password)) == 1
	if !userMatch || !passMatch {
		return "", time.Time{}, errors.New("invalid credentials")
	}

	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		return "", time.Time{}, err
	}
	token := base64.RawURLEncoding.EncodeToString(tokenBytes)
	now := time.Now()
	expires := now.Add(sessionTTL)

	s.mu.Lock()
	s.sessions[token] = session{
		username:  user,
		createdAt: now,
		expiresAt: expires,
	}
	s.mu.Unlock()

	return token, expires, nil
}

// Validate checks a bearer token and returns the associated username.
// Returns an error if the token is missing, unknown, or expired.
func (s *SessionStore) Validate(token string) (string, error) {
	if token == "" {
		return "", errors.New("missing token")
	}
	s.mu.RLock()
	sess, ok := s.sessions[token]
	s.mu.RUnlock()
	if !ok {
		return "", errors.New("invalid token")
	}
	if time.Now().After(sess.expiresAt) {
		s.mu.Lock()
		delete(s.sessions, token)
		s.mu.Unlock()
		return "", errors.New("token expired")
	}
	return sess.username, nil
}

// Logout invalidates a bearer token. No-op if token is empty/invalid.
func (s *SessionStore) Logout(token string) {
	if token == "" {
		return
	}
	s.mu.Lock()
	delete(s.sessions, token)
	s.mu.Unlock()
}

// Count returns the number of active sessions (for diagnostics).
func (s *SessionStore) Count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.sessions)
}

// cleanup periodically prunes expired sessions.
func (s *SessionStore) cleanup() {
	t := time.NewTicker(cleanupTick)
	defer t.Stop()
	for range t.C {
		now := time.Now()
		s.mu.Lock()
		for token, sess := range s.sessions {
			if now.After(sess.expiresAt) {
				delete(s.sessions, token)
			}
		}
		s.mu.Unlock()
	}
}

// allow checks if the given IP is allowed to attempt login.
// Returns false if the IP is currently blocked.
func (l *loginRateLimiter) allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	entry, ok := l.attempts[ip]
	if !ok {
		return true
	}
	now := time.Now()
	// If blocked and block not expired yet, deny.
	if now.Before(entry.blockedUntil) {
		return false
	}
	// If block expired, clean up and allow.
	if !entry.blockedUntil.IsZero() {
		delete(l.attempts, ip)
		return true
	}
	// If window expired, clean up and allow.
	if now.Sub(entry.windowStart) > loginWindow {
		delete(l.attempts, ip)
		return true
	}
	return entry.count < maxLoginAttempts
}

// recordFailure increments the failed attempt counter for the given IP.
// If the counter reaches maxLoginAttempts, the IP is blocked for loginBlockDuration.
func (l *loginRateLimiter) recordFailure(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	entry, ok := l.attempts[ip]
	if !ok {
		l.attempts[ip] = &rateLimitEntry{
			count:       1,
			windowStart: now,
		}
		return
	}
	// If already blocked, don't increment further.
	if now.Before(entry.blockedUntil) {
		return
	}
	// If window expired, reset count.
	if now.Sub(entry.windowStart) > loginWindow {
		entry.count = 1
		entry.windowStart = now
		entry.blockedUntil = time.Time{}
		return
	}
	entry.count++
	if entry.count >= maxLoginAttempts {
		entry.blockedUntil = now.Add(loginBlockDuration)
	}
}

// recordSuccess resets the rate limiter for the given IP after a successful login.
func (l *loginRateLimiter) recordSuccess(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, ip)
}

// extractIP returns the client IP address from the request, stripping the port.
func extractIP(r *http.Request) string {
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// extractToken returns the bearer token from the Authorization header.
// For WebSocket upgrade requests (which cannot set custom headers in browsers),
// the token may also be passed via the ?token= query parameter.
func extractToken(r *http.Request) string {
	auth := r.Header.Get("Authorization")
	if strings.HasPrefix(auth, "Bearer ") {
		return strings.TrimPrefix(auth, "Bearer ")
	}

	// Browser WebSocket API cannot set Authorization header.
	// Allow token via query parameter ONLY for WebSocket upgrade requests.
	if isWebSocketUpgrade(r) {
		if token := r.URL.Query().Get("token"); token != "" {
			return token
		}
	}
	return ""
}

// isWebSocketUpgrade returns true if the request is a WebSocket upgrade request.
func isWebSocketUpgrade(r *http.Request) bool {
	return strings.EqualFold(r.Header.Get("Upgrade"), "websocket") &&
		strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade")
}

// authRequired wraps a handler with bearer-token validation.
// Returns 401 JSON on missing/invalid token.
func (s *Server) authRequired(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := extractToken(r)
		if _, err := s.sessions.Validate(token); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusUnauthorized)
			w.Write([]byte(`{"error":"unauthorized"}`))
			return
		}
		next(w, r)
	}
}

// handleLogin authenticates the user and returns a bearer token.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid request body")
		return
	}

	// If no password is configured, reject all login attempts.
	if s.password == "" {
		writeError(w, http.StatusUnauthorized, "Password cannot be empty. Set onvif.password in config")
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	// Check rate limiter before attempting authentication.
	ip := extractIP(r)
	if !s.loginLimiter.allow(ip) {
		writeError(w, http.StatusTooManyRequests, "too many login attempts, try again later")
		return
	}

	token, expires, err := s.sessions.Login(req.Username, req.Password)
	if err != nil {
		s.loginLimiter.recordFailure(ip)
		slog.Warn("web: login failed", "user", req.Username)
		writeError(w, http.StatusUnauthorized, "invalid username or password")
		return
	}

	s.loginLimiter.recordSuccess(ip)
	slog.Info("web: login OK", "user", req.Username, "active_sessions", s.sessions.Count())
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"token":      token,
		"username":   req.Username,
		"expires_at": expires.UTC().Format(time.RFC3339),
		"expires_in": int(sessionTTL.Seconds()),
	})
}
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	token := extractToken(r)
	if username, err := s.sessions.Validate(token); err == nil {
		s.sessions.Logout(token)
		slog.Info("web: logout OK", "user", username, "active_sessions", s.sessions.Count())
	}
	w.WriteHeader(http.StatusNoContent)
}
