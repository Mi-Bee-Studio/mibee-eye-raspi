package web

import (
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthEndpoint(t *testing.T) {
	sessions, err := NewSessionStore("admin", "admin123")
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	s := &Server{
		logger:    log.New(io.Discard, "", 0),
		sessions:  sessions,
		startTime: time.Now().Add(-2 * time.Hour), // 2 hours ago for a predictable uptime
	}
	s.mux = http.NewServeMux()
	s.registerRoutes()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if resp["status"] != "ok" {
		t.Errorf("expected status=ok, got %v", resp["status"])
	}

	uptime, ok := resp["uptime"].(string)
	if !ok || uptime == "" {
		t.Errorf("expected non-empty uptime string, got %v", resp["uptime"])
	}

	// Health endpoint should NOT require authentication
	req = httptest.NewRequest("GET", "/health", nil)
	rr = httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("expected 200 without auth, got %d", rr.Code)
	}
}

func TestHealthEndpoint_ContentType(t *testing.T) {
	sessions, err := NewSessionStore("admin", "admin123")
	if err != nil {
		t.Fatalf("NewSessionStore: %v", err)
	}
	s := &Server{
		logger:    log.New(io.Discard, "", 0),
		sessions:  sessions,
		startTime: time.Now(),
	}
	s.mux = http.NewServeMux()
	s.registerRoutes()

	req := httptest.NewRequest("GET", "/health", nil)
	rr := httptest.NewRecorder()
	s.mux.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d (body: %s)", rr.Code, rr.Body.String())
	}

	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type application/json, got %q", ct)
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}

	if _, ok := resp["status"]; !ok {
		t.Error("response missing 'status' field")
	}
	if _, ok := resp["uptime"]; !ok {
		t.Error("response missing 'uptime' field")
	}
}
