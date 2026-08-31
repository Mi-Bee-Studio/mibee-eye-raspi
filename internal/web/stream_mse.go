package web

// Chunked-HTTP fMP4 streaming for MSE (SPEC v1 §4.1:
// GET /api/cameras/{id}/stream.mse). Replaces the WebSocket video path:
// init segment first, then one moof+mdat fragment per access unit, with the
// same SPS/PPS caching + fast-forward strategies the WS handler had.

import (
	"net/http"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)

// handleStreamMSE streams H.264 as fMP4 over chunked HTTP.
func (s *Server) handleStreamMSE(w http.ResponseWriter, r *http.Request, cameraID string) {
	if cameraID != "0" {
		writeError(w, http.StatusNotFound, "no such camera")
		return
	}
	if s.cfg.AUHub == nil {
		http.Error(w, "streaming not available", http.StatusServiceUnavailable)
		return
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming unsupported", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "video/mp4")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(http.StatusOK)

	sub := s.cfg.AUHub.Subscribe(r.Context())
	defer s.cfg.AUHub.Unsubscribe(sub.ID)

	var cachedSPS, cachedPPS []byte
	initialized := false
	var sequence uint32
	var prevFrame time.Time
	var mediaClock uint64

	for au := range sub.Channel {
		select {
		case <-r.Context().Done():
			return
		default:
		}

		// Fast-forward: drain stale non-keyframes when we fall behind.
		for len(sub.Channel) > 2 {
			candidate := <-sub.Channel
			if candidate.KeyFrame {
				au = candidate
				break
			}
			au = candidate
		}

		// Update the SPS/PPS cache from whatever this AU carries.
		for _, nalu := range au.NALUs {
			if nalu.IsSPS {
				cachedSPS = nalu.Data
			}
			if nalu.IsPPS {
				cachedPPS = nalu.Data
			}
		}

		if !initialized {
			if !au.KeyFrame || cachedSPS == nil || cachedPPS == nil {
				continue
			}
			width, height := s.cameraDimensions()
			if _, err := w.Write(buildInitSegment(cachedSPS, cachedPPS, width, height)); err != nil {
				return
			}
			flusher.Flush()
			initialized = true
		}

		nalus := make([][]byte, 0, len(au.NALUs))
		for _, nalu := range au.NALUs {
			nalus = append(nalus, nalu.Data)
		}

		// Wall-clock interval → 90 kHz ticks keeps the MSE timeline gapless
		// and true-speed regardless of sensor fps.
		now := time.Now()
		duration := uint32(6000)
		if !prevFrame.IsZero() {
			us := now.Sub(prevFrame).Microseconds()
			ticks := us * 90 / 1000
			if ticks < 90 {
				ticks = 90
			} else if ticks > 18000 {
				ticks = 18000
			}
			duration = uint32(ticks)
		}
		prevFrame = now
		timestamp := mediaClock
		mediaClock += uint64(duration)

		seg := buildMediaSegment(nalus, sequence, timestamp, duration, au.KeyFrame)
		sequence++
		if _, err := w.Write(seg); err != nil {
			return
		}
		flusher.Flush()
	}
}

// cameraDimensions reports the configured capture size for the init segment.
func (s *Server) cameraDimensions() (uint32, uint32) {
	if oc := s.cfg.OnvifConfig; oc != nil {
		return uint32(oc.CameraWidth()), uint32(oc.CameraHeight())
	}
	return 1280, 720
}

// compile-time assertion that the hub types stay wired as expected.
var _ = h264.AccessUnit{}
