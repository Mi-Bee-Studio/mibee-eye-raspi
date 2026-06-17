package web

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ptz"
)
// OnvifConfigProvider provides read-only access to ONVIF, RTSP, device, and camera configuration.
type OnvifConfigProvider interface {
	ONVIFPort() int
	ONVIFUsername() string
	ONVIFPassword() string
	RTSPPort() int
	DeviceIP() string
	CameraDevice() string
	CameraCodec() string
	CameraBitrate() int
	CameraWidth() int
	CameraHeight() int
	CameraFPS() int
	DeviceName() string
	DeviceManufacturer() string
	DeviceModel() string
	DeviceFirmware() string
	DeviceHardwareID() string
	DeviceSerialNumber() string
	LoggingLevel() string
}

// Config holds the web server configuration.
type Config struct {
	Port        int                 // listen port (default 8088)
	Username    string              // basic-auth user (default = onvif user)
	Password    string              // basic-auth pass (default = onvif pass)
	ConfigPath  string              // path to config.yaml (used by /api/config/onvif)
	OnvifConfig OnvifConfigProvider // read-only onvif/rtsp config
	Params      *camera.ParamManager
	PTZ         *ptz.State
	AUHub       *h264.AUHub
	Version     string              // build version from ldflags
	Logger      *log.Logger // nil -> log.Default()
}

// Server is the web UI HTTP server.
type Server struct {
	cfg    Config
	logger *log.Logger
	mux    *http.ServeMux
	hub    *wsHub
	loginLimiter *loginRateLimiter
	server *http.Server

	username string
	password string
	sessions *SessionStore
	startTime time.Time
}

// New creates a new web server.
func New(cfg Config) *Server {
	logger := cfg.Logger
	if logger == nil {
		logger = log.Default()
	}

	username := cfg.Username
	password := cfg.Password
	if username == "" && cfg.OnvifConfig != nil {
		username = cfg.OnvifConfig.ONVIFUsername()
	}
	if password == "" && cfg.OnvifConfig != nil {
		password = cfg.OnvifConfig.ONVIFPassword()
	}

	return &Server{
		cfg:      cfg,
		logger:   logger,
		username: username,
		password: password,
		sessions: NewSessionStore(username, password),
		loginLimiter: &loginRateLimiter{attempts: make(map[string]*rateLimitEntry)},
	}
}

// Start starts the web UI HTTP server on the configured port.
func (s *Server) Start(ctx context.Context) error {
	port := s.cfg.Port
	if port == 0 {
		port = 8088
	}

	s.mux = http.NewServeMux()
	s.hub = newWSHub(s.logger)

	// Wire up hooks from ParamManager and PTZ to WebSocket hub.
	if s.cfg.Params != nil {
		s.cfg.Params.SetOnChange(func(name string, value interface{}) {
			s.hub.sendEvent(wsEvent{
				Type:  "param-changed",
				Name:  name,
				Value: value,
			})
		})
	}
	if s.cfg.PTZ != nil {
		s.cfg.PTZ.SetOnPositionChange(func(pos ptz.Position) {
			s.hub.sendEvent(wsEvent{
				Type: "ptz-position",
			})
		})
		s.cfg.PTZ.SetOnPresetListChange(func() {
			s.hub.sendEvent(wsEvent{
				Type: "preset-list-changed",
			})
		})
	}

	s.startTime = time.Now()
	s.registerRoutes()

	addr := net.JoinHostPort("0.0.0.0", strconv.Itoa(port))
	s.server = &http.Server{
		Addr:              addr,
		Handler:           securityHeaders(s.mux),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		s.logger.Printf("web: server starting on %s", addr)
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	go s.hub.run(ctx)

	select {
	case <-ctx.Done():
		return s.Stop()
	case err := <-errCh:
		return err
	}
}

// Stop stops the web server.
func (s *Server) Stop() error {
	if s.server == nil {
		return nil
	}
	s.hub.close()
	return s.server.Close()
}

// registerRoutes registers all HTTP routes on the server's ServeMux.
func (s *Server) registerRoutes() {
	m := s.mux

	// Static assets — no auth required
	m.HandleFunc("GET /{$}", s.handleIndex)
	m.HandleFunc("GET /static/style.css", s.handleStaticFile("static/style.css", "text/css"))
	m.HandleFunc("GET /static/app.js", s.handleStaticFile("static/app.js", "application/javascript"))
	m.HandleFunc("GET /health", s.handleHealth)
	m.HandleFunc("GET /api/version", s.handleVersion)

	// Auth endpoints — login is public, logout requires auth
	m.HandleFunc("POST /api/login", s.handleLogin)
	m.HandleFunc("POST /api/logout", s.authRequired(s.handleLogout))

	// API routes — auth required (bearer token in Authorization header)
	m.HandleFunc("GET /api/config", s.authRequired(s.handleGetConfig))
	m.HandleFunc("POST /api/config/onvif", s.authRequired(s.handlePostConfigOnvif))
	m.HandleFunc("GET /api/camera/params", s.authRequired(s.handleGetCameraParams))
	m.HandleFunc("POST /api/camera/param", s.authRequired(s.handlePostCameraParam))
	m.HandleFunc("GET /api/camera/options", s.authRequired(s.handleGetCameraOptions))
	m.HandleFunc("GET /api/ptz/status", s.authRequired(s.handleGetPTZStatus))
	m.HandleFunc("POST /api/ptz/move", s.authRequired(s.handlePostPTZMove))
	m.HandleFunc("POST /api/ptz/absolute", s.authRequired(s.handlePostPTZAbsolute))
	m.HandleFunc("POST /api/ptz/relative", s.authRequired(s.handlePostPTZRelative))
	m.HandleFunc("POST /api/ptz/stop", s.authRequired(s.handlePostPTZStop))
	m.HandleFunc("GET /api/ptz/presets", s.authRequired(s.handleGetPTZPresets))
	m.HandleFunc("POST /api/ptz/preset", s.authRequired(s.handlePostPTZPreset))
	m.HandleFunc("POST /api/ptz/preset/goto", s.authRequired(s.handlePostPTZPresetGoto))
	m.HandleFunc("DELETE /api/ptz/preset/{token}", s.authRequired(s.handleDeletePTZPreset))
	m.HandleFunc("PUT /api/ptz/preset/{token}", s.authRequired(s.handlePutPTZPreset))

	// WebSocket — auth via ?token= query string
	m.HandleFunc("GET /ws", s.authRequired(s.handleWS))

	// H.264 video stream via WebSocket — auth via ?token= query string
	m.HandleFunc("GET /api/stream/ws", s.authRequired(s.handleStreamWS))

}

// wsEvent represents a WebSocket event to broadcast.
type wsEvent struct {
	Type  string      `json:"type"`
	Name  string      `json:"name,omitempty"`
	Value interface{} `json:"value,omitempty"`
}

// writeJSON writes a JSON response with the given status code.
func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	data, err := json.Marshal(v)
	if err != nil {
		http.Error(w, `{"error":"internal server error"}`, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	w.Write(data)
	w.Write([]byte("\n"))
}

// writeError writes a JSON error response.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// securityHeaders sets security-related HTTP headers on all responses.
// Content-Security-Policy is only applied to HTML page routes, not API/media endpoints.
func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Strip sensitive credentials from URL query parameters.
		// Prevents credential leakage via browser history, server logs, and Referer headers.
		if q := r.URL.Query(); q.Has("password") || q.Has("username") {
			q.Del("username")
			q.Del("password")
			clean := r.URL.Path
			if enc := q.Encode(); enc != "" {
				clean += "?" + enc
			}
			http.Redirect(w, r, clean, http.StatusFound)
			return
		}

		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// CSP only on HTML pages (root and static assets), not on API/media routes.
		path := r.URL.Path
		if path == "/" || strings.HasPrefix(path, "/static/") {
			w.Header().Set("Content-Security-Policy",
				"default-src 'none'; "+
					"script-src 'self'; "+
					"style-src 'self' 'unsafe-inline'; "+
					"img-src 'self' data: blob:; "+
					"font-src 'self'; "+
					"connect-src 'self' ws: wss:; "+
					"media-src 'self' blob:; "+
					"base-uri 'self'; "+
					"form-action 'self'; "+

					"frame-ancestors 'none'")
		}

		next.ServeHTTP(w, r)
	})
}

// handleIndex serves the embedded index.html.
func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, "index.html not found", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

// handleStaticFile serves an embedded static file.
func (s *Server) handleStaticFile(path, contentType string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		data, err := staticFS.ReadFile(path)
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", contentType)
		w.Write(data)
	}
}

// handleHealth returns a minimal health check with server uptime.
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"uptime": time.Since(s.startTime).String(),
	})
}

// handleVersion returns the build version.
func (s *Server) handleVersion(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version": s.cfg.Version,
	})
}

// extractPresetToken extracts the token from the URL path /api/ptz/preset/{token}.
func extractPresetToken(r *http.Request) string {
	// Go 1.22+ ServeMux with {token} pattern puts it in r.PathValue
	return r.PathValue("token")
}

// maskPassword returns "***" if the password is non-empty, "" otherwise.
func maskPassword(pw string) string {
	if pw == "" {
		return ""
	}
	return "***"
}

// coerceFloat64 converts JSON-decoded float64 to appropriate Go type for ParamManager.
// JSON numbers always decode as float64; this converts to int for integer params.
func coerceFloat64(v interface{}) interface{} {
	f, ok := v.(float64)
	if !ok {
		return v
	}
	if f == float64(int(f)) && !strings.Contains(fmt.Sprintf("%g", f), ".") {
		return int(f)
	}
	return f
}

// _ = fmt.Sprint to avoid import in some Go versions
var _ = strings.TrimSpace
