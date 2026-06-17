package web

import (
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// handleStreamWS upgrades to WebSocket and streams H.264 access units.
// Each AU is sent as a binary WebSocket message in Annex-B bytestream format.
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

	// Don't expect client messages on the stream WS
	conn.SetReadLimit(0)

	sub := s.cfg.AUHub.Subscribe(r.Context())
	defer s.cfg.AUHub.Unsubscribe(sub.ID)

	// Build Annex-B bytestream from AU's NALUs and send as binary message.
	// Each NALU gets a 4-byte start code (00 00 00 01) prefix.
	for au := range sub.Channel {
		// Calculate total size
		totalSize := 0
		for _, nalu := range au.NALUs {
			totalSize += 4 + len(nalu.Data) // start code + NALU data
		}
		if totalSize == 0 {
			continue
		}

		buf := make([]byte, 0, totalSize)
		for _, nalu := range au.NALUs {
			buf = append(buf, 0x00, 0x00, 0x00, 0x01) // Annex-B start code
			buf = append(buf, nalu.Data...)
		}

		conn.SetWriteDeadline(time.Now().Add(5 * time.Second))
		if err := conn.WriteMessage(websocket.BinaryMessage, buf); err != nil {
			return // client disconnected or error
		}
	}
}
