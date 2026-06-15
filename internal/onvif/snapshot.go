package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"strings"
	"net/http"
	"os/exec"
	"sync"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)

// snapshotSem limits concurrent FFmpeg snapshot conversions to prevent OOM
// on memory-constrained devices like RPi 3B (905MB RAM).
var snapshotSem = make(chan struct{}, 2)


// GetSnapshotUriResponse is the ONVIF GetSnapshotUri SOAP response.
type GetSnapshotUriResponse struct {
	XMLName  xml.Name `xml:"trt:GetSnapshotUriResponse"`
	MediaUri MediaUri `xml:"tt:MediaUri"`
}

// SnapshotBuffer stores the latest H.264 key frame data for snapshot requests.
// It subscribes to the AUHub and keeps the most recent IDR frame in memory.
// SPS and PPS NALUs are tracked separately since mtxrpicam may send them
// in separate access units from the IDR slice.
type SnapshotBuffer struct {
	mu         sync.RWMutex
	latestAU   *h264.AccessUnit
	available  bool
	lastSPS    []byte
	lastPPS    []byte
}

// NewSnapshotBuffer creates a new frame buffer.
func NewSnapshotBuffer() *SnapshotBuffer {
	return &SnapshotBuffer{}
}

// Feed updates the buffer with a new access unit.
// It always tracks SPS/PPS from any frame and stores IDR frames for snapshot use.
func (sb *SnapshotBuffer) Feed(au h264.AccessUnit) {
	sb.mu.Lock()
	defer sb.mu.Unlock()

	// Track SPS and PPS from every access unit (they may come in non-IDR frames).
	for i := range au.NALUs {
		if au.NALUs[i].IsSPS {
			sb.lastSPS = append([]byte(nil), au.NALUs[i].Data...)
		}
		if au.NALUs[i].IsPPS {
			sb.lastPPS = append([]byte(nil), au.NALUs[i].Data...)
		}
	}

	// Only store IDR frames for snapshot conversion.
	if !au.KeyFrame {
		return
	}

	copied := h264.AccessUnit{
		NALUs:     make([]h264.NALU, len(au.NALUs)),
		Timestamp: au.Timestamp,
		KeyFrame:  au.KeyFrame,
	}
	for i, nalu := range au.NALUs {
		copied.NALUs[i] = h264.NALU{
			Type:  nalu.Type,
			Data:  append([]byte(nil), nalu.Data...),
			IsIDR: nalu.IsIDR,
			IsSPS: nalu.IsSPS,
			IsPPS: nalu.IsPPS,
		}
	}
	sb.latestAU = &copied
	sb.available = true
}

// Latest returns the most recent IDR access unit with SPS/PPS injected if missing.
// Returns nil if no IDR frame has been received.
func (sb *SnapshotBuffer) Latest() *h264.AccessUnit {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	if !sb.available || sb.latestAU == nil {
		return nil
	}

	au := *sb.latestAU // shallow copy

	// Check if SPS/PPS are already present in the AU.
	hasSPS, hasPPS := false, false
	for _, n := range au.NALUs {
		if n.IsSPS { hasSPS = true }
		if n.IsPPS { hasPPS = true }
	}

	// Inject cached SPS/PPS if missing.
	var extra []h264.NALU
	if !hasSPS && sb.lastSPS != nil {
		extra = append(extra, h264.NALU{Type: 7, Data: sb.lastSPS, IsSPS: true})
	}
	if !hasPPS && sb.lastPPS != nil {
		extra = append(extra, h264.NALU{Type: 8, Data: sb.lastPPS, IsPPS: true})
	}

	if len(extra) > 0 {
		au.NALUs = append(extra, au.NALUs...)
	}

	return &au
}

// Available reports whether a frame has been stored.
func (sb *SnapshotBuffer) Available() bool {
	sb.mu.RLock()
	defer sb.mu.RUnlock()
	return sb.available
}

// RegisterSnapshotHandlers registers the GetSnapshotUri SOAP action and the
// /snapshot HTTP endpoint. The buf is shared with the web UI for /api/snapshot.
// The frameSource channel feeds buf with incoming H.264 access units.
func RegisterSnapshotHandlers(s *Server, buf *SnapshotBuffer, frameSource <-chan h264.AccessUnit) {
	// Feed buffer from frame source in background goroutine.
	if frameSource != nil {
		go func() {
			for au := range frameSource {
				buf.Feed(au)
			}
		}()
	}

	s.RegisterAction("GetSnapshotUri", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetSnapshotUri(ctx, s.config), nil
	})

	s.snapshotHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleSnapshotHTTP(w, r, buf)
	})
}

// handleGetSnapshotUri returns the snapshot URL for the device.
// The IP portion reflects the NVR's source IP from the request context so the
// returned URL is reachable from whichever interface the NVR used.
func handleGetSnapshotUri(ctx context.Context, cfg ConfigProvider) *GetSnapshotUriResponse {
	ip := ServerIPFromContext(ctx, cfg.DeviceIP())
	uri := fmt.Sprintf("http://%s:%d/snapshot", ip, cfg.ONVIFPort())

	return &GetSnapshotUriResponse{
		MediaUri: MediaUri{
			Uri:                uri,
			InvalidAfterConnect: "false",
			InvalidAfterReboot:  "false",
			Timeout:             "PT10S",
		},
	}
}

// handleSnapshotHTTP serves a JPEG snapshot by converting the latest H.264 IDR
// frame to JPEG via an on-demand FFmpeg subprocess.
func handleSnapshotHTTP(w http.ResponseWriter, r *http.Request, buf *SnapshotBuffer) {
	handleSnapshotHTTPWithBin(w, r, buf, "ffmpeg")
}

// snapshotTimeout is the maximum time allowed for FFmpeg conversion.
const snapshotTimeout = 5 * time.Second

// handleSnapshotHTTPWithBin is like handleSnapshotHTTP but accepts a custom
// ffmpeg binary path, used in tests to inject a mock converter.
func handleSnapshotHTTPWithBin(w http.ResponseWriter, r *http.Request, buf *SnapshotBuffer, ffmpegBin string) {
	au := buf.Latest()
	if au == nil {
		http.Error(w, "snapshot not available: no frame received yet", http.StatusServiceUnavailable)
		return
	}

	// Limit concurrent FFmpeg conversions to prevent OOM on memory-constrained devices.
	select {
	case snapshotSem <- struct{}{}:
		defer func() { <-snapshotSem }()
	case <-time.After(3 * time.Second):
		http.Error(w, "snapshot server busy, try again later", http.StatusServiceUnavailable)
		return
	}

	jpegData, err := ConvertIDRToJPEG(r.Context(), au, ffmpegBin)
	if err != nil {
		http.Error(w, err.Error(), http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(jpegData)))
	w.Header().Set("X-Frame-Timestamp", au.Timestamp.UTC().Format(time.RFC3339Nano))
	w.Write(jpegData)
}

// ConvertIDRToJPEG converts an H.264 access unit to JPEG using FFmpeg.
// It builds an Annex-B bytestream and pipes it through FFmpeg for conversion.
// Returns an error if au is nil, has no data, or FFmpeg fails.
func ConvertIDRToJPEG(ctx context.Context, au *h264.AccessUnit, ffmpegBin string) ([]byte, error) {
	if au == nil {
		return nil, fmt.Errorf("snapshot not available: no frame received yet")
	}
	annexB := buildAnnexB(au.NALUs)
	if len(annexB) == 0 {
		return nil, fmt.Errorf("snapshot not available: empty frame data")
	}
	timeoutCtx, cancel := context.WithTimeout(ctx, snapshotTimeout)
	defer cancel()

	cmd := exec.CommandContext(timeoutCtx,
		ffmpegBin,
		"-f", "h264",
		"-i", "pipe:0",
		"-frames:v", "1",
		"-f", "image2pipe",
		"pipe:1",
	)
	cmd.Stdin = bytes.NewReader(annexB)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	jpegData, err := cmd.Output()
	if timeoutCtx.Err() != nil {
		return nil, fmt.Errorf("snapshot conversion timed out")
	}
	if err != nil {
		return nil, fmt.Errorf("snapshot conversion failed: %s", strings.TrimSpace(stderr.String()))
	}

	return jpegData, nil
}

// startCode is the 4-byte H.264 Annex-B start code.
var startCode = []byte{0x00, 0x00, 0x00, 0x01}

// buildAnnexB reconstructs an H.264 Annex-B bytestream from NALUs.
// SPS and PPS NALUs are placed before the IDR for decodability.
func buildAnnexB(nalus []h264.NALU) []byte {
	var sps, pps, idr, others []h264.NALU
	for _, n := range nalus {
		switch {
		case n.IsSPS:
			sps = append(sps, n)
		case n.IsPPS:
			pps = append(pps, n)
		case n.IsIDR:
			idr = append(idr, n)
		default:
			others = append(others, n)
		}
	}

	var buf []byte
	for _, n := range sps {
		buf = append(buf, startCode...)
		buf = append(buf, n.Data...)
	}
	for _, n := range pps {
		buf = append(buf, startCode...)
		buf = append(buf, n.Data...)
	}
	for _, n := range others {
		buf = append(buf, startCode...)
		buf = append(buf, n.Data...)
	}
	for _, n := range idr {
		buf = append(buf, startCode...)
		buf = append(buf, n.Data...)
	}
	return buf
}
