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
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ptz"

	"gopkg.in/yaml.v3"
)

// handleGetConfig returns the full configuration dump.
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	oc := s.cfg.OnvifConfig
	if oc == nil {
		writeError(w, http.StatusInternalServerError, "onvif config not available")
		return
	}

	config := map[string]interface{}{
		"camera": map[string]interface{}{
			"device":     "/dev/video0",
			"width":      1280,
			"height":     720,
			"fps":        15,
			"codec":      "h264",
			"bitrate":    2000000,
			"brightness": 0.0,
			"contrast":   1.0,
			"saturation": 1.0,
			"sharpness":  1.0,
		},
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
		"rtmp": map[string]interface{}{
			"enabled": false,
			"url":     "",
		},
		"device": map[string]interface{}{
			"name":           "Pi Camera V1",
			"manufacturer":   "Raspberry Pi",
			"model":          "OV5647",
			"firmware":       "1.0.0",
			"hardware_id":    "OV5647",
			"serial_number": "",
		},
		"logging": map[string]interface{}{
			"level": "info",
		},
		"web": map[string]interface{}{
			"enabled":  true,
			"port":     s.cfg.Port,
			"username": s.username,
			"password": maskPassword(s.password),
		},
	}

	// Override camera params from ParamManager if available.
	if s.cfg.Params != nil {
		cam := map[string]interface{}{}
		for name := range camera.ParamRanges {
			if val, err := s.cfg.Params.Get(name); err == nil {
				cam[name] = val
			}
		}
		for name := range camera.ParamEnums {
			if val, err := s.cfg.Params.Get(name); err == nil {
				cam[name] = val
			}
		}
		if len(cam) > 0 {
			config["camera"] = cam
		}
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

// handleGetPTZStatus returns current PTZ position and status.
func (s *Server) handleGetPTZStatus(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	pos := s.cfg.PTZ.GetPosition()
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"position": pos,
		"status":   s.cfg.PTZ.GetStatus(),
	})
}

// handlePostPTZMove starts continuous movement.
func (s *Server) handlePostPTZMove(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	var vel ptz.Velocity
	if err := json.NewDecoder(r.Body).Decode(&vel); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	s.cfg.PTZ.ContinuousMove(vel)
	s.logger.Printf("web: PTZ continuous move pan=%.2f tilt=%.2f zoom=%.2f", vel.Pan, vel.Tilt, vel.Zoom)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handlePostPTZAbsolute moves to an absolute position.
func (s *Server) handlePostPTZAbsolute(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	var pos ptz.Position
	if err := json.NewDecoder(r.Body).Decode(&pos); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	s.cfg.PTZ.AbsoluteMove(pos)
	s.logger.Printf("web: PTZ absolute move pan=%.2f tilt=%.2f zoom=%.2f", pos.Pan, pos.Tilt, pos.Zoom)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handlePostPTZRelative applies relative movement.
func (s *Server) handlePostPTZRelative(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	var delta ptz.Velocity
	if err := json.NewDecoder(r.Body).Decode(&delta); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	s.cfg.PTZ.RelativeMove(delta)
	s.logger.Printf("web: PTZ relative move pan=%.2f tilt=%.2f zoom=%.2f", delta.Pan, delta.Tilt, delta.Zoom)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handlePostPTZStop stops all PTZ movement.
func (s *Server) handlePostPTZStop(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	s.cfg.PTZ.Stop()
	s.logger.Printf("web: PTZ stop")
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleGetPTZPresets returns the full preset list.
func (s *Server) handleGetPTZPresets(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	presets := s.cfg.PTZ.ListPresets()
	result := make([]map[string]interface{}, 0, len(presets))
	for _, p := range presets {
		result = append(result, map[string]interface{}{
			"token":    p.Token,
			"name":     p.Name,
			"position": p.Position,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// handlePostPTZPreset creates a new preset.
func (s *Server) handlePostPTZPreset(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	var req struct {
		Token string `json:"token"`
		Name  string `json:"name"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if req.Name == "" {
		writeError(w, http.StatusBadRequest, "preset name is required")
		return
	}

	token := req.Token
	if token == "" {
		token = fmt.Sprintf("preset-%d", time.Now().UnixNano()%100000)
	}

	// Check if token already exists.
	if _, err := s.cfg.PTZ.GetPresetPosition(token); err == nil {
		writeError(w, http.StatusConflict, fmt.Sprintf("preset %s already exists", token))
		return
	}

	if err := s.cfg.PTZ.SetPreset(token, req.Name); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	pos := s.cfg.PTZ.GetPosition()
	s.logger.Printf("web: PTZ preset created token=%s name=%s", token, req.Name)

	writeJSON(w, http.StatusCreated, map[string]interface{}{
		"ok":       true,
		"token":    token,
		"name":     req.Name,
		"position": pos,
	})
}

// handlePostPTZPresetGoto moves to a preset position.
func (s *Server) handlePostPTZPresetGoto(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	var req struct {
		Token string `json:"token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	if err := s.cfg.PTZ.GotoPreset(req.Token); err != nil {
		if err == ptz.ErrPresetNotFound {
			writeError(w, http.StatusNotFound, fmt.Sprintf("preset %s not found", req.Token))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Printf("web: PTZ goto preset %s", req.Token)
	writeJSON(w, http.StatusOK, map[string]interface{}{"ok": true})
}

// handleDeletePTZPreset removes a preset.
func (s *Server) handleDeletePTZPreset(w http.ResponseWriter, r *http.Request) {
	if s.cfg.PTZ == nil {
		writeError(w, http.StatusInternalServerError, "PTZ not available")
		return
	}

	token := extractPresetToken(r)
	if token == "" {
		writeError(w, http.StatusBadRequest, "preset token is required")
		return
	}

	if err := s.cfg.PTZ.RemovePreset(token); err != nil {
		if err == ptz.ErrPresetNotFound {
			writeError(w, http.StatusNotFound, fmt.Sprintf("preset %s not found", token))
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	s.logger.Printf("web: PTZ preset removed %s", token)
	w.WriteHeader(http.StatusNoContent)
}
