package web

// Config and imaging endpoints (SPEC v1 §5, §4.5).

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

// handleGetConfig returns the full configuration dump (enveloped, secrets
// masked to "****" per SPEC §5).
func (s *Server) handleGetConfig(w http.ResponseWriter, r *http.Request) {
	oc := s.cfg.OnvifConfig
	if oc == nil {
		writeError(w, http.StatusInternalServerError, "onvif config not available")
		return
	}

	cameraSection := map[string]interface{}{
		"device":  oc.CameraDevice(),
		"width":   oc.CameraWidth(),
		"height":  oc.CameraHeight(),
		"fps":     oc.CameraFPS(),
		"codec":   oc.CameraCodec(),
		"bitrate": oc.CameraBitrate(),
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

	webUser, webPass := s.currentCredentials()
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
			"name":          oc.DeviceName(),
			"manufacturer":  oc.DeviceManufacturer(),
			"model":         oc.DeviceModel(),
			"firmware":      oc.DeviceFirmware(),
			"hardware_id":   oc.DeviceHardwareID(),
			"serial_number": oc.DeviceSerialNumber(),
		},
		"logging": map[string]interface{}{
			"level": oc.LoggingLevel(),
		},
		"web": map[string]interface{}{
			"enabled":  true,
			"port":     s.cfg.Port,
			"username": webUser,
			"password": maskPassword(webPass),
		},
		"gb28181": func() map[string]interface{} {
			if s.cfg.GB28181Config == nil {
				return map[string]interface{}{"enabled": false}
			}
			return map[string]interface{}{
				"enabled":                 s.cfg.GB28181Config.Enabled,
				"platform_sip_address":    s.cfg.GB28181Config.PlatformSIPAddress,
				"platform_sip_port":       s.cfg.GB28181Config.PlatformSIPPort,
				"device_id":               s.cfg.GB28181Config.DeviceID,
				"channel_id":              s.cfg.GB28181Config.ChannelID,
				"sip_domain":              s.cfg.GB28181Config.SIPDomain,
				"password":                maskPassword(s.cfg.GB28181Config.Password),
				"local_sip_port":          s.cfg.GB28181Config.LocalSIPPort,
				"register_interval_secs":  s.cfg.GB28181Config.RegisterIntervalSecs,
				"heartbeat_interval_secs": s.cfg.GB28181Config.HeartbeatIntervalSecs,
				"heartbeat_timeout_count": s.cfg.GB28181Config.HeartbeatTimeoutCount,
			}
		}(),
	}

	writeOK(w, http.StatusOK, config)
}

// handlePutConfig accepts a partial config document (SPEC §5), deep-merges it
// over the YAML file, restores masked secrets, and restarts the process so
// the change applies (config_apply default = restart). If the web section
// changes the credentials, the in-memory ones update immediately too.
func (s *Server) handlePutConfig(w http.ResponseWriter, r *http.Request) {
	if s.cfg.ConfigPath == "" {
		writeError(w, http.StatusNotImplemented, "config path not configured")
		return
	}

	var update map[string]interface{}
	if err := json.NewDecoder(r.Body).Decode(&update); err != nil {
		writeError(w, http.StatusBadRequest, fmt.Sprintf("invalid request body: %v", err))
		return
	}

	data, err := os.ReadFile(s.cfg.ConfigPath)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read config: %v", err))
		return
	}
	var cfg map[string]interface{}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse config: %v", err))
		return
	}

	restoreMaskedSecrets(update, cfg)
	deepMerge(cfg, update)

	out, err := yaml.Marshal(cfg)
	if err != nil {
		writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to marshal config: %v", err))
		return
	}
	if err := atomicWrite(s.cfg.ConfigPath, out); err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	// Credentials take effect immediately so a fresh login works post-restart.
	if webSec, ok := update["web"].(map[string]interface{}); ok {
		if u, ok := webSec["username"].(string); ok && u != "" && u != "****" {
			s.mu.Lock()
			s.username = u
			s.mu.Unlock()
		}
	}

	s.logger.Printf("web: config updated, restarting in 500ms")
	go func() {
		<-time.After(500 * time.Millisecond)
		_ = syscall.Kill(os.Getpid(), syscall.SIGTERM)
	}()

	writeOK(w, http.StatusOK, map[string]interface{}{"applied": "restart"})
}

// restoreMaskedSecrets replaces "****" password values in the update with
// the stored ones so a masked GET → PUT round-trip keeps secrets.
func restoreMaskedSecrets(update, current map[string]interface{}) {
	sections := []string{"onvif", "web", "gb28181", "rtsp"}
	for _, sec := range sections {
		upd, ok := update[sec].(map[string]interface{})
		if !ok {
			continue
		}
		pw, ok := upd["password"].(string)
		if !ok || pw != "****" {
			continue
		}
		if cur, ok := current[sec].(map[string]interface{}); ok {
			if stored, ok := cur["password"].(string); ok {
				upd["password"] = stored
			}
		}
	}
}

// deepMerge recursively merges src into dst (maps only; other values replace).
func deepMerge(dst, src map[string]interface{}) {
	for k, v := range src {
		if sub, ok := v.(map[string]interface{}); ok {
			if dsub, ok := dst[k].(map[string]interface{}); ok {
				deepMerge(dsub, sub)
				continue
			}
		}
		dst[k] = v
	}
}

// atomicWrite writes data to path via a temp file + rename.
func atomicWrite(path string, data []byte) error {
	dir := filepath.Dir(path)
	tmpFile, err := os.CreateTemp(dir, filepath.Base(path)+".*.tmp")
	if err != nil {
		return fmt.Errorf("failed to create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	if _, err := tmpFile.Write(data); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to write temp file: %w", err)
	}
	if err := tmpFile.Sync(); err != nil {
		tmpFile.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("failed to sync temp file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename config: %w", err)
	}
	return nil
}

// handleGetCameraParams returns all current imaging parameters (SPEC §4.5).
func (s *Server) handleGetCameraParams(w http.ResponseWriter, r *http.Request) {
	if !s.requireCamera0(w, r) {
		return
	}
	if s.cfg.Params == nil {
		writeError(w, http.StatusInternalServerError, "param manager not available")
		return
	}

	result := map[string]interface{}{}
	for name := range camera.ParamRanges {
		if val, err := s.cfg.Params.Get(name); err == nil {
			result[name] = val
		}
	}
	for name := range camera.ParamEnums {
		if val, err := s.cfg.Params.Get(name); err == nil {
			result[name] = val
		}
	}
	writeOK(w, http.StatusOK, result)
}

// handlePostCameraParam sets a single imaging parameter (immediate effect,
// broadcasts param_changed via the SSE hub).
func (s *Server) handlePostCameraParam(w http.ResponseWriter, r *http.Request) {
	if !s.requireCamera0(w, r) {
		return
	}
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

	value := coerceFloat64(req.Value)
	if err := s.cfg.Params.Set(req.Name, value); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.logger.Printf("web: camera param %s set to %v", req.Name, value)
	writeOK(w, http.StatusOK, map[string]interface{}{"name": req.Name, "value": value})
}

// handleGetCameraOptions returns imaging parameter ranges and enums.
func (s *Server) handleGetCameraOptions(w http.ResponseWriter, r *http.Request) {
	if !s.requireCamera0(w, r) {
		return
	}
	result := map[string]interface{}{}
	for name, rg := range camera.ParamRanges {
		result[name] = map[string]interface{}{
			"min":     rg.Min,
			"max":     rg.Max,
			"default": rg.Default,
		}
	}
	for name, enums := range camera.ParamEnums {
		result[name] = map[string]interface{}{"enums": enums}
	}
	writeOK(w, http.StatusOK, result)
}

// requireCamera0 answers 404 for any camera id other than "0".
func (s *Server) requireCamera0(w http.ResponseWriter, r *http.Request) bool {
	if r.PathValue("id") != "0" {
		writeError(w, http.StatusNotFound, "no such camera")
		return false
	}
	return true
}
