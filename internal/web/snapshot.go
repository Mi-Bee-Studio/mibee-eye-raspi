package web

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/onvif"
)

const snapshotTimeout = 5 * time.Second

// webSnapshotSem limits concurrent JPEG conversions in the web handler.
// Matches the ONVIF handler's limit to protect memory on constrained devices.
var webSnapshotSem = make(chan struct{}, 2)

// handleGetSnapshot serves a JPEG snapshot from the latest H.264 IDR frame.
func (s *Server) handleGetSnapshot(w http.ResponseWriter, r *http.Request) {
	if s.cfg.Snapshot == nil {
		writeError(w, http.StatusServiceUnavailable, "snapshot not available: snapshot buffer not configured")
		return
	}

	au := s.cfg.Snapshot.Latest()
	if au == nil {
		writeError(w, http.StatusServiceUnavailable, "snapshot not available: no frame received yet")
		return
	}

	// Acquire concurrency slot (3s timeout to avoid stacking requests).
	select {
	case webSnapshotSem <- struct{}{}:
		defer func() { <-webSnapshotSem }()
	case <-time.After(3 * time.Second):
		writeError(w, http.StatusServiceUnavailable, "snapshot server busy, try again later")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), snapshotTimeout)
	defer cancel()

	jpegData, err := onvif.ConvertIDRToJPEG(ctx, au, "ffmpeg")
	if err != nil {
		writeError(w, http.StatusServiceUnavailable, err.Error())
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(jpegData)))
	w.Write(jpegData)
}
