package web

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// handleStreamWS upgrades to WebSocket and streams H.264 access units.
// Each AU is sent as a binary WebSocket message in Annex-B bytestream format.
// Implements two strategies for reliable browser MSE playback:
// 1. SPS/PPS caching: always inject cached SPS+PPS before IDR frames
// 2. Fast-forward: when subscriber falls behind, skip stale P-frames
func (s *Server) handleStreamWS(w http.ResponseWriter, r *http.Request) {
	if s.cfg.AUHub == nil {
		http.Error(w, "streaming not available", http.StatusServiceUnavailable)
		return
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.logger.Printf("web: stream WebSocket upgrade failed: %v", err)
		return
	}
	defer conn.Close()
	conn.SetReadLimit(0)

	sub := s.cfg.AUHub.Subscribe(r.Context())
	defer s.cfg.AUHub.Unsubscribe(sub.ID)

	startCode := []byte{0x00, 0x00, 0x00, 0x01}
	var cachedSPS, cachedPPS []byte // latest SPS/PPS seen across all AUs

	for au := range sub.Channel {
		// Fast-forward: if multiple AUs are queued, drain non-keyframes.
		backlog := len(sub.Channel)
		if backlog > 2 {
			for i := 0; i < backlog; i++ {
				candidate := <-sub.Channel
				if candidate.KeyFrame {
					au = candidate
					remaining := len(sub.Channel)
					for j := 0; j < remaining; j++ {
						skip := <-sub.Channel
						if skip.KeyFrame { au = skip }
					}
					break
				}
				if i < backlog-1 { continue }
				au = candidate
			}
		}

		// Scan NALUs: update SPS/PPS cache, detect IDR presence.
		hasSPS, hasPPS, hasIDR := false, false, false
		for _, nalu := range au.NALUs {
			if nalu.IsSPS { cachedSPS = nalu.Data; hasSPS = true }
			if nalu.IsPPS { cachedPPS = nalu.Data; hasPPS = true }
			if nalu.IsIDR { hasIDR = true }
		}

		// Build the list of NALU data to send.
		var nalusToSend [][]byte
		for _, nalu := range au.NALUs {
			nalusToSend = append(nalusToSend, nalu.Data)
		}

		// Inject cached SPS+PPS before IDR if missing (e.g. after frame drops
		// or if camera sends SPS/PPS in a separate AU from IDR).
		if hasIDR && (!hasSPS || !hasPPS) && cachedSPS != nil && cachedPPS != nil {
			prepend := make([][]byte, 0, 2)
			if !hasSPS { prepend = append(prepend, cachedSPS) }
			if !hasPPS { prepend = append(prepend, cachedPPS) }
			nalusToSend = append(prepend, nalusToSend...)
		}

		// Build Annex-B bytestream.
		totalSize := 0
		for _, data := range nalusToSend {
			totalSize += 4 + len(data)
		}
		if totalSize == 0 { continue }

		buf := make([]byte, 0, totalSize)
		for _, data := range nalusToSend {
			buf = append(buf, startCode...)
			buf = append(buf, data...)
		}

		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
			return
		}
	}
}
