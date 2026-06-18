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
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ptz"

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

// ============================================================================
// PTZ handler tests
// ============================================================================

func TestHandleGetPTZStatus_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("GET", "/api/ptz/status", nil)
	w := httptest.NewRecorder()
	s.handleGetPTZStatus(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetPTZStatus_Success(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("GET", "/api/ptz/status", nil)
	w := httptest.NewRecorder()
	s.handleGetPTZStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}

	status, ok := resp["status"].(string)
	if !ok || status != "IDLE" {
		t.Errorf("expected status=IDLE, got %v", resp["status"])
	}
	pos, ok := resp["position"].(map[string]interface{})
	if !ok {
		t.Error("expected position field")
	} else {
		if pos["Pan"] != 0.0 {
			t.Errorf("expected Pan=0, got %v", pos["Pan"])
		}
		if pos["Tilt"] != 0.0 {
			t.Errorf("expected Tilt=0, got %v", pos["Tilt"])
		}
		if pos["Zoom"] != 0.0 {
			t.Errorf("expected Zoom=0, got %v", pos["Zoom"])
		}
	}
}

func TestHandlePostPTZMove_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("POST", "/api/ptz/move", nil)
	w := httptest.NewRecorder()
	s.handlePostPTZMove(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostPTZMove_Success(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"pan":0.5,"tilt":0.0,"zoom":0.0}`
	req := httptest.NewRequest("POST", "/api/ptz/move", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZMove(w, req)

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
}

func TestHandlePostPTZMove_InvalidBody(t *testing.T) {
	s := &Server{
		cfg:    Config{PTZ: ptz.NewState()},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("POST", "/api/ptz/move", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZMove(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePostPTZAbsolute_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("POST", "/api/ptz/absolute", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZAbsolute(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostPTZAbsolute_Success(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"pan":0.3,"tilt":-0.2,"zoom":0.5}`
	req := httptest.NewRequest("POST", "/api/ptz/absolute", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZAbsolute(w, req)

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
}

func TestHandlePostPTZAbsolute_InvalidBody(t *testing.T) {
	s := &Server{
		cfg:    Config{PTZ: ptz.NewState()},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("POST", "/api/ptz/absolute", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZAbsolute(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePostPTZRelative_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("POST", "/api/ptz/relative", strings.NewReader(`{}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZRelative(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostPTZRelative_Success(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"pan":0.1,"tilt":0.2,"zoom":0.0}`
	req := httptest.NewRequest("POST", "/api/ptz/relative", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZRelative(w, req)

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

	// Relative move is immediate
	pos := ptzState.GetPosition()
	if pos.Pan != 0.1 {
		t.Errorf("expected Pan=0.1, got %f", pos.Pan)
	}
	if pos.Tilt != 0.2 {
		t.Errorf("expected Tilt=0.2, got %f", pos.Tilt)
	}
}

func TestHandlePostPTZRelative_InvalidBody(t *testing.T) {
	s := &Server{
		cfg:    Config{PTZ: ptz.NewState()},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("POST", "/api/ptz/relative", strings.NewReader("not json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZRelative(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandlePostPTZStop_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("POST", "/api/ptz/stop", nil)
	w := httptest.NewRecorder()
	s.handlePostPTZStop(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostPTZStop_Success(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("POST", "/api/ptz/stop", nil)
	w := httptest.NewRecorder()
	s.handlePostPTZStop(w, req)

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
}

func TestHandleGetPTZPresets_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("GET", "/api/ptz/presets", nil)
	w := httptest.NewRecorder()
	s.handleGetPTZPresets(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleGetPTZPresets_Empty(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("GET", "/api/ptz/presets", nil)
	w := httptest.NewRecorder()
	s.handleGetPTZPresets(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() != "[]\n" {
		t.Errorf("expected empty array, got %s", w.Body.String())
	}
}

func TestHandleGetPTZPresets_WithPresets(t *testing.T) {
	ptzState := ptz.NewState()
	ptzState.SetPreset("preset-1", "Home")
	ptzState.SetPreset("preset-2", "Door")

	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("GET", "/api/ptz/presets", nil)
	w := httptest.NewRecorder()
	s.handleGetPTZPresets(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var presets []interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &presets); err != nil {
		t.Fatal(err)
	}
	if len(presets) != 2 {
		t.Errorf("expected 2 presets, got %d", len(presets))
	}
}

func TestHandlePostPTZPreset_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	body := `{"name":"Test"}`
	req := httptest.NewRequest("POST", "/api/ptz/preset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPreset(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostPTZPreset_Success(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"token":"home","name":"Home Position"}`
	req := httptest.NewRequest("POST", "/api/ptz/preset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPreset(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["token"] != "home" {
		t.Errorf("expected token 'home', got %v", resp["token"])
	}
	if resp["name"] != "Home Position" {
		t.Errorf("expected name 'Home Position', got %v", resp["name"])
	}
	if resp["ok"] != true {
		t.Error("expected ok=true")
	}

	// Verify it was stored
	if _, err := ptzState.GetPresetPosition("home"); err != nil {
		t.Errorf("preset not stored: %v", err)
	}
}

func TestHandlePostPTZPreset_Duplicate(t *testing.T) {
	ptzState := ptz.NewState()
	ptzState.SetPreset("home", "Home Position")

	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"token":"home","name":"Home Again"}`
	req := httptest.NewRequest("POST", "/api/ptz/preset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPreset(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostPTZPreset_EmptyName(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"token":"home","name":""}`
	req := httptest.NewRequest("POST", "/api/ptz/preset", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPreset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandlePostPTZPresetGoto_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("POST", "/api/ptz/preset/goto", nil)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPresetGoto(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandlePostPTZPresetGoto_Success(t *testing.T) {
	ptzState := ptz.NewState()
	ptzState.SetPreset("home", "Home")

	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"token":"home"}`
	req := httptest.NewRequest("POST", "/api/ptz/preset/goto", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPresetGoto(w, req)

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
}

func TestHandlePostPTZPresetGoto_NotFound(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	body := `{"token":"nonexistent"}`
	req := httptest.NewRequest("POST", "/api/ptz/preset/goto", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handlePostPTZPresetGoto(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeletePTZPreset_NilPTZ(t *testing.T) {
	s := &Server{cfg: Config{}, logger: log.New(io.Discard, "", 0)}
	req := httptest.NewRequest("DELETE", "/api/ptz/preset/test", nil)
	req.SetPathValue("token", "test")
	w := httptest.NewRecorder()
	s.handleDeletePTZPreset(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleDeletePTZPreset_Success(t *testing.T) {
	ptzState := ptz.NewState()
	ptzState.SetPreset("home", "Home")

	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("DELETE", "/api/ptz/preset/home", nil)
	req.SetPathValue("token", "home")
	w := httptest.NewRecorder()
	s.handleDeletePTZPreset(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}

	if presets := ptzState.ListPresets(); len(presets) != 0 {
		t.Error("preset was not removed")
	}
}

func TestHandleDeletePTZPreset_NotFound(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("DELETE", "/api/ptz/preset/nonexistent", nil)
	req.SetPathValue("token", "nonexistent")
	w := httptest.NewRecorder()
	s.handleDeletePTZPreset(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleDeletePTZPreset_EmptyToken(t *testing.T) {
	ptzState := ptz.NewState()
	s := &Server{
		cfg:    Config{PTZ: ptzState},
		logger: log.New(io.Discard, "", 0),
	}
	req := httptest.NewRequest("DELETE", "/api/ptz/preset/", nil)
	req.SetPathValue("token", "")
	w := httptest.NewRecorder()
	s.handleDeletePTZPreset(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
