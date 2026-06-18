package web

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"

	"gopkg.in/yaml.v3"
)

// handleGetConfig returns the full configuration dump.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	oc := s.cfg.OnvifConfig
	if oc == nil {
		writeError(w, http.StatusInternalServerError, "onvif config not available")
		return
	}

	cameraSection := map[string]interface{}{
		"device":     oc.CameraDevice(),
		"width":      oc.CameraWidth(),
		"height":     oc.CameraHeight(),
		"fps":        oc.CameraFPS(),
		"codec":      oc.CameraCodec(),
		"bitrate":    oc.CameraBitrate(),
	}

	// Overlay live params from ParamManager when available.
	if s.cfg.Params != nil {
		for name := range camera.ParamRanges {
			if val, err := s.cfg.Params.Get(name); err == nil {
				cameraSection[name] = val
			}
		}
		for name := range camera.ParamEnums {
			if val, err := s.cfg.Params.Get(name); err == nil {
				cameraSection[name] = val
			}
		}
	}

	config := map[string]interface{}{
		"camera": cameraSection,
		"rtsp": map[string]interface{}{
			"port":     oc.RTSPPort(),
			"username": "",
			"password": "",
		},
		"onvif": map[string]interface{}{
			"port":     oc.ONVIFPort(),
			"username": oc.ONVIFUsername(),
			"password": maskPassword(oc.ONVIFPassword()),
		},
		"device": map[string]interface{}{
			"name":           oc.DeviceName(),
			"manufacturer":   oc.DeviceManufacturer(),
			"model":          oc.DeviceModel(),
			"firmware":       oc.DeviceFirmware(),
			"hardware_id":    oc.DeviceHardwareID(),
			"serial_number":  oc.DeviceSerialNumber(),
		},
		"logging": map[string]interface{}{
			"level": oc.LoggingLevel(),
		},
		"web": map[string]interface{}{
			"enabled":  true,
			"port":     s.cfg.Port,
			"username": s.username,
			"password": maskPassword(s.password),
		},
	}

	writeJSON(w, http.StatusOK, config)
}

// handlePostConfigOnvif updates ONVIF credentials and triggers restart.
func (s *Server) handlePostConfigOnvif(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ConfigPath == "" {
		writeError(w, http.StatusNotImplemented, "config path not configured")
		return
	}

	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.Username == "" || req.Password == "" {
		writeError(w, http.StatusBadRequest, "username and password are required")
		return
	}

	// Read existing config file to preserve all sections.
	data, err := os.ReadFile(s.cfg.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read config: %v", err))
		return
	}

	// Unmarshal into generic map to preserve all sections.
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse config: %v", err))
		return
	}

	// Update only the onvif section.
	cfg["onvif"] = map[string]interface{}{
		"port":     s.cfg.OnvifConfig.ONVIFPort(),
		"username": req.Username,
		"password": req.Password,
	}

	// Marshal the complete config back to YAML.
	out, err := yaml.Marshal(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal config: %v", err))
		return
	}

	// Atomic write: temp file in same directory, then rename.
	dir := filepath.Dir(s.cfg.ConfigPath)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(s.cfg.ConfigPath)+".*.tmp")
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to create temp file: %v", err))
		return
	}
	tmpPath := tmpFile.Name()

	if _, err := tmpFile.Write(out); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to write temp file: %v", err))
		return
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to sync temp file: %v", err))
		return
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to close temp file: %v", err))
		return
	}
	if err := os.Rename(tmpPath, s.cfg.ConfigPath); err != nil {
		os.Remove(tmpPath)
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to rename config: %v", err))
		return
	}

	s.logger.Printf("web: ONVIF config updated, restarting in 500ms")

	// Schedule restart after response is sent.
	go func() {
		<-time.After(500 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":               true,
		"restart_required": true,
	})
}

// handleGetCameraParams returns all current camera parameters.
func (s *Server) handleGetCameraParams(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Params == nil {
		writeError(w, http.StatusInternalServerError, "param manager not available")
		return
	}

	result := map[string]interface{}{}

	// Get all ranged params.
	for name := range camera.ParamRanges {
		if val, err := s.cfg.Params.Get(name); err == nil {
			result[name] = val
		}
	}
	// Get all enum params.
	for name := range camera.ParamEnums {
		if val, err := s.cfg.Params.Get(name); err == nil {
			result[name] = val
		}
	}

	writeJSON(w, http.StatusOK, result)
}

// handlePostCameraParam sets a single camera parameter.
func (s *Server) handlePostCameraParam(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Params == nil {
		writeError(w, http.StatusInternalServerError, "param manager not available")
		return
	}

	var req struct {
		Name  string      `json:"name"`
		Value interface{} `json:"value"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "parameter name is required")
		return
	}

	// Coerce JSON number (always float64) to int for integer params.
	value := coerceFloat64(req.Value)

	if err := s.cfg.Params.Set(req.Name, value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	s.logger.Printf("web: camera param %s set to %v", req.Name, value)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":   true,
		"name": req.Name,
		"value": value,
	})
}

// handleGetCameraOptions returns parameter ranges and enum values.
func (s *Server) handleGetCameraOptions(w http.ResponseWriter, r *http.Request) {
	result := map[string]interface{}{}

	for name, r := range camera.ParamRanges {
		result[name] = map[string]interface{}{
			"min":     r.Min,
			"max":     r.Max,
			"default": r.Default,
		}
	}
	for name, enums := range camera.ParamEnums {
		result[name] = map[string]interface{}{
			"enums": enums,
		}
	}

	writeJSON(w, http.StatusOK, result)
}

