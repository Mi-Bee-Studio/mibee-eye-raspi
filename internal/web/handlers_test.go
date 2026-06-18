package web

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"

	"gopkg.in/yaml.v3"
)

// mockOnvifConfig implements config.ConfigProvider for testing.
type mockOnvifConfig struct {
	port     int
	username string
	password string
}

func (m *mockOnvifConfig) ONVIFPort() int             { return m.port }
func (m *mockOnvifConfig) ONVIFUsername() string      { return m.username }
func (m *mockOnvifConfig) ONVIFPassword() string      { return m.password }
func (m *mockOnvifConfig) RTSPPort() int              { return 8554 }
func (m *mockOnvifConfig) DeviceIP() string           { return "192.168.1.1" }
func (m *mockOnvifConfig) CameraDevice() string       { return "/dev/video0" }
func (m *mockOnvifConfig) CameraCodec() string        { return "h264" }
func (m *mockOnvifConfig) CameraBitrate() int         { return 2000000 }
func (m *mockOnvifConfig) CameraWidth() int           { return 1280 }
func (m *mockOnvifConfig) CameraHeight() int          { return 720 }
func (m *mockOnvifConfig) CameraFPS() int             { return 15 }
func (m *mockOnvifConfig) DeviceName() string         { return "Test Camera" }
func (m *mockOnvifConfig) DeviceManufacturer() string { return "Test Manufacturer" }
func (m *mockOnvifConfig) DeviceModel() string        { return "TestModel" }
func (m *mockOnvifConfig) DeviceFirmware() string     { return "1.0.0" }
func (m *mockOnvifConfig) DeviceHardwareID() string   { return "TEST001" }
func (m *mockOnvifConfig) DeviceSerialNumber() string { return "" }
func (m *mockOnvifConfig) LoggingLevel() string       { return "info" }
func (m *mockOnvifConfig) SnapshotEnabled() bool { return true }
func (m *mockOnvifConfig) SnapshotQuality() int  { return 85 }

func TestSaveConfigPreservesAllSections(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	// Write a multi-section config file.
	initialYAML := `camera:
  device: /dev/video0
  width: 1280
  height: 720
  fps: 15
  codec: h264
  bitrate: 2000000
rtsp:
  port: 8554
  username: ""
  password: ""
onvif:
  port: 8080
  username: old-admin
  password: old-secret
rtmp:
  enabled: true
  url: rtmp://example.com/live
device:
  name: Test Camera
  manufacturer: Test Manufacturer
  model: Test Model
  firmware: 1.0.0
  hardware_id: TEST-001
  serial_number: SN12345
logging:
  level: debug
web:
  enabled: true
  port: 8088
  username: web-user
  password: web-pass
`
	if err := os.WriteFile(configPath, []byte(initialYAML), 0600); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		cfg: Config{
			ConfigPath: configPath,
			OnvifConfig: &mockOnvifConfig{
				port:     8080,
				username: "old-admin",
				password: "old-secret",
			},
		},
		logger: log.New(io.Discard, "", 0),
	}

	// POST new ONVIF credentials.
	body := `{"username":"new-admin","password":"new-secret"}`
	req := httptest.NewRequest("POST", "/api/config/onvif", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handlePostConfigOnvif(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// Read back the config file.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("failed to parse saved config: %v", err)
	}

	// Verify all sections are present.
	sections := []string{"camera", "rtsp", "onvif", "device", "logging", "web"}
	for _, sec := range sections {
		if _, ok := result[sec]; !ok {
			t.Errorf("section %q is missing after save", sec)
		}
	}

	// Verify onvif section was updated.
	onvif, ok := result["onvif"].(map[string]interface{})
	if !ok {
		t.Fatal("onvif section is not a map")
	}
	if onvif["username"] != "new-admin" {
		t.Errorf("expected username 'new-admin', got %v", onvif["username"])
	}
	if onvif["password"] != "new-secret" {
		t.Errorf("expected password 'new-secret', got %v", onvif["password"])
	}
	if onvif["port"] != 8080 {
		t.Errorf("expected port 8080, got %v", onvif["port"])
	}

	web, ok := result["web"].(map[string]interface{})
	if !ok {
		t.Fatal("web section is not a map")
	}
	if web["port"] != 8088 {
		t.Errorf("expected web.port 8088, got %v", web["port"])
	}
}

func TestSaveConfigAtomicWrite(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	initialYAML := `camera:
  device: /dev/video0
onvif:
  port: 8080
  username: admin
  password: secret
`
	if err := os.WriteFile(configPath, []byte(initialYAML), 0600); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		cfg: Config{
			ConfigPath: configPath,
			OnvifConfig: &mockOnvifConfig{
				port:     8080,
				username: "admin",
				password: "secret",
			},
		},
		logger: log.New(io.Discard, "", 0),
	}

	body := `{"username":"new","password":"newpass"}`
	req := httptest.NewRequest("POST", "/api/config/onvif", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handlePostConfigOnvif(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}

	// List directory — should only contain config.yaml (no leftover temp files).
	entries, err := os.ReadDir(tmpDir)
	if err != nil {
		t.Fatal(err)
	}

	for _, e := range entries {
		if e.Name() != "config.yaml" {
			t.Errorf("unexpected file left in config directory: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file (config.yaml), got %d", len(entries))
	}

	// Verify the config file is valid YAML and contains updated credentials.
	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]interface{}
	if err := yaml.Unmarshal(data, &result); err != nil {
		t.Fatalf("saved config is not valid YAML: %v", err)
	}
	onvif, ok := result["onvif"].(map[string]interface{})
	if !ok {
		t.Fatal("onvif section missing")
	}
	if onvif["username"] != "new" {
		t.Errorf("expected username 'new', got %v", onvif["username"])
	}
}

func TestSaveConfigHandlesMissingFile(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "nonexistent.yaml")

	s := &Server{
		cfg: Config{
			ConfigPath: configPath,
			OnvifConfig: &mockOnvifConfig{
				port:     8080,
				username: "admin",
				password: "secret",
			},
		},
		logger: log.New(io.Discard, "", 0),
	}

	body := `{"username":"new","password":"newpass"}`
	req := httptest.NewRequest("POST", "/api/config/onvif", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handlePostConfigOnvif(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for missing file, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestSaveConfigHandlesInvalidYAML(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")

	if err := os.WriteFile(configPath, []byte("not: valid: yaml: [[["), 0600); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		cfg: Config{
			ConfigPath: configPath,
			OnvifConfig: &mockOnvifConfig{
				port:     8080,
				username: "admin",
				password: "secret",
			},
		},
		logger: log.New(io.Discard, "", 0),
	}

	body := `{"username":"new","password":"newpass"}`
	req := httptest.NewRequest("POST", "/api/config/onvif", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	s.handlePostConfigOnvif(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for invalid YAML, got %d: %s", rr.Code, rr.Body.String())
	}
}

// ============================================================================
// Mock camera for ParamManager tests
// ============================================================================

type mockCamera struct {
	values map[string]interface{}
}

func newMockCamera() *mockCamera {
	return &mockCamera{values: make(map[string]interface{})}
}

func (m *mockCamera) Start(ctx context.Context) error { return nil }
func (m *mockCamera) Stop() error                     { return nil }
func (m *mockCamera) Frames() <-chan camera.Frame     { return nil }
func (m *mockCamera) SetParam(name string, value interface{}) error {
	m.values[name] = value
	return nil
}
func (m *mockCamera) GetParam(name string) (interface{}, error) {
	v, ok := m.values[name]
	if !ok {
		return nil, fmt.Errorf("param %q not set", name)
	}
	return v, nil
}
func (m *mockCamera) Info() camera.CameraInfo { return camera.CameraInfo{} }

// ============================================================================
// handleGetConfig tests
// ============================================================================

func TestHandleGetConfig_MissingOnvifConfig(t *testing.T) {
	s := &Server{
		cfg:    Config{},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	s.handleGetConfig(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetConfig_NoParams(t *testing.T) {
	s := &Server{
		cfg: Config{
			OnvifConfig: &mockOnvifConfig{port: 8080, username: "admin", password: "secret"},
		},
		logger:   log.New(io.Discard, "", 0),
		username: "webadmin",
		password: "webpass",
	}
	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	s.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	sections := []string{"camera", "rtsp", "onvif", "device", "logging", "web"}
	for _, sec := range sections {
		if _, ok := resp[sec]; !ok {
			t.Errorf("missing section %q", sec)
		}
	}

	onvif := resp["onvif"].(map[string]interface{})
	if onvif["password"] != "***" {
		t.Errorf("expected masked password, got %v", onvif["password"])
	}
}

func TestHandleGetConfig_WithParams(t *testing.T) {
	cam := newMockCamera()
	pm := camera.NewParamManager(cam)
	if err := pm.Set("Brightness", 0.5); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		cfg: Config{
			OnvifConfig: &mockOnvifConfig{port: 8080, username: "admin", password: "secret"},
			Params:      pm,
		},
		logger:   log.New(io.Discard, "", 0),
		username: "webadmin",
		password: "webpass",
	}
	req := httptest.NewRequest("GET", "/api/config", nil)
	w := httptest.NewRecorder()
	s.handleGetConfig(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	camSec, ok := resp["camera"].(map[string]interface{})
	if !ok {
		t.Fatal("camera section missing or not a map")
	}
	brightness, ok := camSec["Brightness"]
	if !ok {
		t.Error("expected Brightness in camera section")
	} else if brightness != 0.5 {
		t.Errorf("expected Brightness=0.5, got %v", brightness)
	}
}

// ============================================================================
// handleGetCameraParams tests
// ============================================================================

func TestHandleGetCameraParams_NilParamManager(t *testing.T) {
	s := &Server{
		cfg:    Config{},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("GET", "/api/camera/params", nil)
	w := httptest.NewRecorder()
	s.handleGetCameraParams(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetCameraParams_Success(t *testing.T) {
	cam := newMockCamera()
	pm := camera.NewParamManager(cam)
	if err := pm.Set("Brightness", 0.5); err != nil {
		t.Fatal(err)
	}
	if err := pm.Set("Contrast", 1.5); err != nil {
		t.Fatal(err)
	}

	s := &Server{
		cfg:    Config{Params: pm},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("GET", "/api/camera/params", nil)
	w := httptest.NewRecorder()
	s.handleGetCameraParams(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	if resp["Brightness"] != 0.5 {
		t.Errorf("expected Brightness=0.5, got %v", resp["Brightness"])
	}
	if resp["Contrast"] != 1.5 {
		t.Errorf("expected Contrast=1.5, got %v", resp["Contrast"])
	}
}

// ============================================================================
// handlePostCameraParam tests
// ============================================================================

func TestHandlePostCameraParam_NilParamManager(t *testing.T) {
	s := &Server{
		cfg:    Config{},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"name":"Brightness","value":0.5}`
	req := httptest.NewRequest("POST", "/api/camera/param", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostCameraParam(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostCameraParam_Success(t *testing.T) {
	cam := newMockCamera()
	pm := camera.NewParamManager(cam)

	s := &Server{
		cfg:    Config{Params: pm},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"name":"Brightness","value":0.5}`
	req := httptest.NewRequest("POST", "/api/camera/param", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostCameraParam(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}
	if resp["name"] != "Brightness" {
		t.Errorf("expected name=Brightness, got %v", resp["name"])
	}

	// Verify through ParamManager
	val, err := pm.Get("Brightness")
	if err != nil {
		t.Fatal(err)
	}
	if val != 0.5 {
		t.Errorf("expected Brightness=0.5, got %v", val)
	}
}

func TestHandlePostCameraParam_InvalidBody(t *testing.T) {
	s := &Server{
		cfg:    Config{Params: camera.NewParamManager(newMockCamera())},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("POST", "/api/camera/param", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostCameraParam(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePostCameraParam_EmptyName(t *testing.T) {
	s := &Server{
		cfg:    Config{Params: camera.NewParamManager(newMockCamera())},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"name":"","value":0.5}`
	req := httptest.NewRequest("POST", "/api/camera/param", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostCameraParam(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ============================================================================
// handleGetCameraOptions tests
// ============================================================================

func TestHandleGetCameraOptions(t *testing.T) {
	s := &Server{logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("GET", "/api/camera/options", nil)
	w := httptest.NewRecorder()
	s.handleGetCameraOptions(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	brightness, ok := resp["Brightness"].(map[string]interface{})
	if !ok {
		t.Fatal("missing Brightness in options")
	}
	if brightness["min"] != -1.0 {
		t.Errorf("expected min=-1, got %v", brightness["min"])
	}
	if brightness["max"] != 1.0 {
		t.Errorf("expected max=1, got %v", brightness["max"])
	}

	awb, ok := resp["AWBMode"].(map[string]interface{})
	if !ok {
		t.Fatal("missing AWBMode in options")
	}
	enums, ok := awb["enums"].([]interface{})
	if !ok {
		t.Fatal("AWBMode.enums is not a list")
	}
	if len(enums) == 0 {
		t.Error("AWBMode.enums is empty")
	}
}

