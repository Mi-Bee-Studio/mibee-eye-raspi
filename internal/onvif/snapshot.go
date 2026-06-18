package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)

// ---------------------------------------------------------------------------
// SnapshotBuffer — stores the latest H.264 IDR frame for snapshot capture
// ---------------------------------------------------------------------------

// SnapshotBuffer stores the latest H.264 IDR frame from the AUHub stream
// and provides JPEG capture via rpicam-still (with H.264 fallback).
type SnapshotBuffer struct {
	mu        sync.RWMutex
	latestIDR []byte // raw H.264 IDR NALU with start code
	latestSPS []byte // SPS NALU with start code
	latestPPS []byte // PPS NALU with start code
	hasFrame  bool
	enabled   bool
}

// NewSnapshotBuffer creates a new SnapshotBuffer.
func NewSnapshotBuffer(enabled bool) *SnapshotBuffer {
	return &SnapshotBuffer{enabled: enabled}
}

// Update stores the latest IDR frame data from an AUHub access unit.
// Called from the AUHub subscriber goroutine.
func (sb *SnapshotBuffer) Update(au h264.AccessUnit) {
	if !sb.enabled {
		return
	}
	if !au.KeyFrame || len(au.NALUs) == 0 {
		return
	}

	sb.mu.Lock()
	defer sb.mu.Unlock()

	// Reset
	sb.latestIDR = nil
	sb.latestSPS = nil
	sb.latestPPS = nil

	startCode := []byte{0x00, 0x00, 0x00, 0x01}

	for _, nalu := range au.NALUs {
		if nalu.IsSPS {
			sb.latestSPS = append(startCode, nalu.Data...)
		}
		if nalu.IsPPS {
			sb.latestPPS = append(startCode, nalu.Data...)
		}
		if nalu.IsIDR {
			sb.latestIDR = append(startCode, nalu.Data...)
		}
	}

	sb.hasFrame = sb.latestIDR != nil
}

// Snapshot returns a JPEG image or raw H.264 IDR frame.
//
// Strategy (two-tier):
//  1. Try rpicam-still subprocess for a real JPEG (works when camera is idle).
//  2. Fall back to the stored H.264 IDR frame with Content-Type video/H264
//     (works when the camera pipeline is running).
//
// Returns: image bytes, MIME content type, error.
func (sb *SnapshotBuffer) Snapshot() ([]byte, string, error) {
	// Tier 1: rpicam-still JPEG capture
	if data, err := captureRPiCamStill(); err == nil {
		slog.Debug("snapshot: captured via rpicam-still")
		return data, "image/jpeg", nil
	}

	// Tier 2: stored H.264 IDR frame
	sb.mu.RLock()
	defer sb.mu.RUnlock()

	if !sb.hasFrame {
		return nil, "", fmt.Errorf("no frame available")
	}

	// Build complete H.264 access unit: SPS + PPS + IDR
	var buf bytes.Buffer
	if sb.latestSPS != nil {
		buf.Write(sb.latestSPS)
	}
	if sb.latestPPS != nil {
		buf.Write(sb.latestPPS)
	}
	buf.Write(sb.latestIDR)

	return buf.Bytes(), "video/H264", nil
}

// HasFrame returns true if an IDR frame is available.
func (sb *SnapshotBuffer) HasFrame() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.hasFrame
}

// Enabled returns whether the snapshot buffer is active.
func (sb *SnapshotBuffer) Enabled() bool {
	return sb.enabled
}

// SubscribeToHub subscribes to the AUHub and updates the snapshot buffer
// whenever a key frame (IDR) is received. Blocks until ctx is cancelled.
func (sb *SnapshotBuffer) SubscribeToHub(ctx context.Context, hub *h264.AUHub) {
	if !sb.enabled {
		return
	}
	sub := hub.Subscribe(ctx)
	slog.Debug("snapshot: subscribed to AUHub")
	for {
		select {
		case <-ctx.Done():
			return
		case au, ok := <-sub.Channel:
			if !ok {
				return
			}
			sb.Update(au)
		}
	}
}

// captureRPiCamStill attempts to capture a JPEG frame using rpicam-still.
// Returns the JPEG bytes on success, or an error if the camera is busy/unavailable.
func captureRPiCamStill() ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, "rpicam-still",
		"-o", "-",
		"--nopreview",
		"-t", "100", // 100ms timeout — fail fast if camera is busy
	)
	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = io.Discard

	if err := cmd.Run(); err != nil {
		return nil, fmt.Errorf("rpicam-still: %w", err)
	}

	data := stdout.Bytes()
	if len(data) < 100 {
		return nil, fmt.Errorf("rpicam-still: output too small (%d bytes)", len(data))
	}

	// Verify JPEG header (SOI marker 0xFFD8)
	if len(data) < 2 || data[0] != 0xFF || data[1] != 0xD8 {
		return nil, fmt.Errorf("rpicam-still: output is not JPEG (got 0x%02x 0x%02x)", data[0], data[1])
	}

	return data, nil
}

// ---------------------------------------------------------------------------
// HTTP handler — GET /snapshot
// ---------------------------------------------------------------------------

// ServeHTTP handles GET /snapshot requests.
// Returns a JPEG image with Content-Type: image/jpeg, or a raw H.264 frame
// with Content-Type: video/H264 as fallback.
func (sb *SnapshotBuffer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	data, contentType, err := sb.Snapshot()
	if err != nil {
		slog.Warn("snapshot: failed to capture", "error", err)
		http.Error(w, "snapshot unavailable: "+err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "no-store, must-revalidate")
	w.Write(data)
}

// ---------------------------------------------------------------------------
// SOAP handler — GetSnapshotUri
// ---------------------------------------------------------------------------

// GetSnapshotUriResponse is the ONVIF GetSnapshotUri SOAP response.
type GetSnapshotUriResponse struct {
	XMLName  xml.Name `xml:"trt:GetSnapshotUriResponse"`
	MediaUri MediaUri `xml:"tt:MediaUri"`
}

// RegisterSnapshotHandlers registers the GetSnapshotUri SOAP action on the
// ONVIF server, and optionally the /snapshot HTTP endpoint on the server.
func RegisterSnapshotHandlers(s *Server, sb *SnapshotBuffer) {
	if !sb.enabled {
		return
	}

	s.RegisterAction("GetSnapshotUri", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetSnapshotUri(ctx, s.config), nil
	})

	s.SetSnapshotHandler(sb)
}

// handleGetSnapshotUri returns the HTTP URL for snapshot capture.
func handleGetSnapshotUri(ctx context.Context, cfg config.ConfigProvider) *GetSnapshotUriResponse {
	ip := ServerIPFromContext(ctx, cfg.DeviceIP())
	port := cfg.ONVIFPort()
	uri := fmt.Sprintf("http://%s:%d/snapshot", ip, port)

	return &GetSnapshotUriResponse{
		MediaUri: MediaUri{
			Uri:                 uri,
			InvalidAfterConnect: "false",
			InvalidAfterReboot:  "false",
			Timeout:             "PT0S",
		},
	}
}
