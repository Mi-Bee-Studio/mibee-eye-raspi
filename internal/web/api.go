package web

// SPEC v1 core endpoints: response envelope, cameras resource model,
// device status and the capability superset. Contract: ../mibee-webui/SPEC.md

import (
	"fmt"
	"net/http"
	"time"
)

// errorCode maps an HTTP status to the SPEC §0 machine code.
func errorCode(status int) string {
	switch status {
	case http.StatusBadRequest:
		return "bad_request"
	case http.StatusUnauthorized:
		return "unauthorized"
	case http.StatusForbidden:
		return "forbidden"
	case http.StatusNotFound:
		return "not_found"
	case http.StatusConflict:
		return "conflict"
	case http.StatusTooManyRequests:
		return "rate_limited"
	case http.StatusNotImplemented:
		return "not_implemented"
	case http.StatusServiceUnavailable:
		return "setup_required"
	default:
		return "internal_error"
	}
}

// writeOK emits the success envelope {"ok":true,"data":…}.
func writeOK(w http.ResponseWriter, status int, data interface{}) {
	writeJSON(w, status, map[string]interface{}{"ok": true, "data": data})
}

// writeError emits the failure envelope
// {"ok":false,"error":"<machine code>","message":"<human text>"}.
func writeError(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]interface{}{
		"ok":      false,
		"error":   errorCode(status),
		"message": msg,
	})
}

// processStart backs the uptime fields.
var processStart = time.Now()

// handleHealth (GET /api/health, public): liveness probe (SPEC §1).
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeOK(w, http.StatusOK, map[string]interface{}{
		"status": "ok",
		"uptime": int(time.Since(processStart).Seconds()),
	})
}

// handleStatus (GET /api/status): SPEC §3 core fields + Go extras.
func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	oc := s.cfg.OnvifConfig
	resp := map[string]interface{}{
		"device_name": "MiBee Eye",
		"firmware":    s.cfg.Version,
		"uptime":      int(time.Since(processStart).Seconds()),
		"gb28181":     s.cfg.GB28181Config != nil && s.cfg.GB28181Config.Enabled,
	}
	if oc != nil {
		resp["model"] = oc.DeviceModel()
		resp["vendor"] = oc.DeviceManufacturer()
		resp["resolution"] = fmt.Sprintf("%dx%d", oc.CameraWidth(), oc.CameraHeight())
		resp["fps"] = oc.CameraFPS()
	}
	if s.cfg.CameraStatus != nil {
		resp["camera_alive"] = s.cfg.CameraStatus()
	}
	if s.cfg.RTSPStatus != nil {
		resp["rtsp"] = s.cfg.RTSPStatus()
	}
	writeOK(w, http.StatusOK, resp)
}

// handleCapabilities (GET /api/capabilities): SPEC §3.1 superset.
func (s *Server) handleCapabilities(w http.ResponseWriter, r *http.Request) {
	oc := s.cfg.OnvifConfig
	caps := map[string]interface{}{
		"spec_version":      "1",
		"auth":              map[string]interface{}{"model": "session", "setup": true},
		"multi_camera":      false,
		"camera_management": false,
		"camera_control":    false,
		"imaging":           s.cfg.Params != nil,
		"ai":                false,
		"ptz":               false,
		"hls":               true,
		"recording":         false,
		"devices":           false,
		"mjpeg":             false, // Go dialect A4: H.264 pipeline has no raw frames
		"mse":               s.cfg.AUHub != nil,
		"webrtc":            false,
		"events":            []string{"param_changed"},
		"config_apply": map[string]interface{}{
			"default":  "restart",
			"sections": map[string]string{"imaging": "immediate"},
		},
		"restart": true,
	}
	device := map[string]interface{}{"name": "MiBee Eye"}
	if oc != nil {
		device["name"] = oc.DeviceName()
		device["model"] = oc.DeviceModel()
		device["vendor"] = oc.DeviceManufacturer()
	}
	caps["device"] = device
	writeOK(w, http.StatusOK, caps)
}

// cameraDoc builds the single-camera document (SPEC §4).
func (s *Server) cameraDoc() map[string]interface{} {
	doc := map[string]interface{}{
		"id":          "0",
		"name":        "MiBee Eye",
		"camera_type": "csi",
		"status":      "online",
	}
	if oc := s.cfg.OnvifConfig; oc != nil {
		doc["name"] = oc.DeviceName()
		doc["resolution"] = fmt.Sprintf("%dx%d", oc.CameraWidth(), oc.CameraHeight())
		doc["fps"] = oc.CameraFPS()
		doc["rtsp_url"] = fmt.Sprintf("rtsp://self:%d/stream", oc.RTSPPort())
	}
	if s.cfg.CameraStatus != nil && !s.cfg.CameraStatus() {
		doc["status"] = "offline"
	}
	return doc
}

// handleCameras (GET /api/cameras): the single-element camera list.
func (s *Server) handleCameras(w http.ResponseWriter, r *http.Request) {
	writeOK(w, http.StatusOK, []interface{}{s.cameraDoc()})
}
