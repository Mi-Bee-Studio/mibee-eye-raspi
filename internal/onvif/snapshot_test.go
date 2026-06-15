package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)
// ---------------------------------------------------------------------------
// Unit tests — response builders
// ---------------------------------------------------------------------------

func TestGetSnapshotUri(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}

	resp := handleGetSnapshotUri(context.Background(), cfg)

	if !strings.Contains(resp.MediaUri.Uri, "/snapshot") {
		t.Errorf("expected URI to contain '/snapshot', got %q", resp.MediaUri.Uri)
	}
	expected := "http://192.168.1.100:8080/snapshot"
	if resp.MediaUri.Uri != expected {
		t.Errorf("expected URI %q, got %q", expected, resp.MediaUri.Uri)
	}
	if resp.MediaUri.Timeout != "PT10S" {
		t.Errorf("expected timeout PT10S, got %q", resp.MediaUri.Timeout)
	}
}

// ---------------------------------------------------------------------------
// SnapshotBuffer tests
// ---------------------------------------------------------------------------

func TestSnapshotBufferRejectsNonKeyFrames(t *testing.T) {
	buf := NewSnapshotBuffer()

	buf.Feed(h264.AccessUnit{
		NALUs:    []h264.NALU{{Type: 1, Data: []byte("non-idr")}},
		Timestamp: time.Now(),
		KeyFrame:  false,
	})

	if buf.Available() {
		t.Error("buffer should not be available after non-key frame")
	}
	if buf.Latest() != nil {
		t.Error("latest should be nil after non-key frame")
	}
}

func TestSnapshotBufferStoresKeyFrame(t *testing.T) {
	buf := NewSnapshotBuffer()

	ts := time.Now().UTC()
	buf.Feed(h264.AccessUnit{
		NALUs:    []h264.NALU{{Type: 5, Data: []byte("idr-frame-data"), IsIDR: true}},
		Timestamp: ts,
		KeyFrame:  true,
	})

	if !buf.Available() {
		t.Fatal("buffer should be available after key frame")
	}

	au := buf.Latest()
	if au == nil {
		t.Fatal("latest should not be nil")
	}
	if len(au.NALUs) != 1 {
		t.Fatalf("expected 1 NALU, got %d", len(au.NALUs))
	}
	if string(au.NALUs[0].Data) != "idr-frame-data" {
		t.Errorf("expected NALU data 'idr-frame-data', got %q", string(au.NALUs[0].Data))
	}
}

func TestSnapshotBufferOverwrites(t *testing.T) {
	buf := NewSnapshotBuffer()

	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("first-idr"), IsIDR: true}},
		Timestamp: time.Now(),
		KeyFrame:  true,
	})
	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("second-idr"), IsIDR: true}},
		Timestamp: time.Now(),
		KeyFrame:  true,
	})

	au := buf.Latest()
	if string(au.NALUs[0].Data) != "second-idr" {
		t.Errorf("expected latest to be 'second-idr', got %q", string(au.NALUs[0].Data))
	}
}

// ---------------------------------------------------------------------------
// HTTP endpoint tests
// ---------------------------------------------------------------------------

// ---------------------------------------------------------------------------
// HTTP endpoint tests — use mock FFmpeg scripts
// ---------------------------------------------------------------------------

// writeMockFFmpeg creates a shell script that reads H.264 from stdin and
// writes a minimal JPEG (JFIF header with valid SOI/EOI) to stdout.
// Returns the script path. Caller should clean up.
func writeMockFFmpeg(t *testing.T, name string, hang bool) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, name)

	if hang {
		script := "#!/bin/sh\nexec sleep 30\n"
		if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
			t.Fatalf("failed to write mock %s: %v", name, err)
		}
		return path
	}

	// Write a minimal JPEG file and a shell script that cats it to stdout.
	jpegPath := filepath.Join(dir, name+".jpg")
	jpeg := []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 0x4A, 0x46, 0x49, 0x46, 0x00, 0x01, 0x01, 0x00, 0x00, 0x01, 0x00, 0x01, 0x00, 0x00, 0xFF, 0xD9}
	if err := os.WriteFile(jpegPath, jpeg, 0o644); err != nil {
		t.Fatalf("failed to write mock JPEG: %v", err)
	}
	script := "#!/bin/sh\ncat > /dev/null\ncat " + jpegPath + "\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("failed to write mock %s: %v", name, err)
	}
	return path
}

func TestSnapshotNotAvailable(t *testing.T) {
	buf := NewSnapshotBuffer()

	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	w := httptest.NewRecorder()

	handleSnapshotHTTP(w, req, buf)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "no frame received") {
		t.Errorf("expected error message about no frame, got %q", w.Body.String())
	}
}

func TestSnapshotHTTPWithMockFFmpeg(t *testing.T) {
	buf := NewSnapshotBuffer()

	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("fake-h264-nalu-data"), IsIDR: true}},
		Timestamp: time.Date(2024, 1, 15, 10, 30, 0, 0, time.UTC),
		KeyFrame:  true,
	})

	mockFFmpeg := writeMockFFmpeg(t, "mock-ffmpeg", false)
	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	w := httptest.NewRecorder()

	handleSnapshotHTTPWithBin(w, req, buf, mockFFmpeg)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %q", ct)
	}
	body := w.Body.Bytes()
	if len(body) < 2 {
		t.Fatal("expected body with at least 2 bytes (JPEG SOI marker)")
	}
	if body[0] != 0xFF || body[1] != 0xD8 {
		t.Errorf("expected JPEG SOI marker (FF D8), got %02X %02X", body[0], body[1])
	}
	tsHeader := w.Header().Get("X-Frame-Timestamp")
	if !strings.Contains(tsHeader, "2024-01-15") {
		t.Errorf("expected timestamp header to contain date, got %q", tsHeader)
	}
}

func TestSnapshotTimeoutWithCancelledContext(t *testing.T) {
	buf := NewSnapshotBuffer()

	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("fake-h264-nalu-data"), IsIDR: true}},
		Timestamp: time.Now(),
		KeyFrame:  true,
	})

	// Use a request with an already-cancelled context.
	cancelledCtx, cancel := context.WithCancel(context.Background())
	cancel()
	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil).WithContext(cancelledCtx)
	w := httptest.NewRecorder()

	handleSnapshotHTTPWithBin(w, req, buf, "nonexistent-binary")

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 for cancelled context, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// SOAP integration test — GetSnapshotUri via full HTTP round-trip
// ---------------------------------------------------------------------------

func TestGetSnapshotUriViaHTTPIntegration(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterSnapshotHandlers(srv, NewSnapshotBuffer(), make(chan h264.AccessUnit))

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
 xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
  <s:Body>
    <trt:GetSnapshotUri>
      <trt:ProfileToken>main</trt:ProfileToken>
    </trt:GetSnapshotUri>
  </s:Body>
</s:Envelope>`

	req := httptest.NewRequest(http.MethodPost, "/onvif/media_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		body := w.Body.String()
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, body)
	}

	// Parse with NVR-compatible struct
	type testEnvelope struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			GetSnapshotUriResponse struct {
				MediaUri struct {
					URI string `xml:"Uri"`
				} `xml:"MediaUri"`
			} `xml:"GetSnapshotUriResponse"`
		} `xml:"Body"`
	}

	var env testEnvelope
	if err := xml.Unmarshal(w.Body.Bytes(), &env); err != nil {
		// If strict parse fails, check body for the URI
		body := w.Body.String()
		if !strings.Contains(body, "/snapshot") {
			t.Fatalf("failed to parse and URI not in body. Parse error: %v\nBody:\n%s", err, body)
		}
		t.Fatalf("failed to parse with NVR struct (parse error: %v). URI IS in body — XML path mismatch.\nBody:\n%s", err, body)
	}

	if !strings.Contains(env.Body.GetSnapshotUriResponse.MediaUri.URI, "/snapshot") {
		t.Errorf("expected URI to contain '/snapshot', got %q", env.Body.GetSnapshotUriResponse.MediaUri.URI)
	}
}

// ---------------------------------------------------------------------------
// Integration test — /snapshot routed through Server.ServeHTTP
// ---------------------------------------------------------------------------

func TestSnapshotViaServerServeHTTP(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)

	// Manually set up snapshot handler with a pre-fed buffer and mock FFmpeg.
	buf := NewSnapshotBuffer()
	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("test-frame-bytes"), IsIDR: true}},
		Timestamp: time.Now(),
		KeyFrame:  true,
	})
	mockFFmpeg := writeMockFFmpeg(t, "mock-ffmpeg-srv", false)
	srv.snapshotHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleSnapshotHTTPWithBin(w, r, buf, mockFFmpeg)
	})

	// GET /snapshot should be routed to the snapshot handler, not SOAP.
	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
	ct := w.Header().Get("Content-Type")
	if ct != "image/jpeg" {
		t.Errorf("expected Content-Type image/jpeg, got %q", ct)
	}
}

// ---------------------------------------------------------------------------
// Integration test — /snapshot unavailable through Server.ServeHTTP
// ---------------------------------------------------------------------------

func TestSnapshotUnavailableViaServerServeHTTP(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)

	buf := NewSnapshotBuffer()
	srv.snapshotHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handleSnapshotHTTP(w, r, buf)
	})

	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Test GetSnapshotUri marshalling
// ---------------------------------------------------------------------------

func TestGetSnapshotUriMarshalling(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	resp := handleGetSnapshotUri(context.Background(), cfg)

	data, err := MarshalSOAP(resp)
	if err != nil {
		t.Fatalf("MarshalSOAP failed: %v", err)
	}

	body := string(data)
	required := []string{
		"GetSnapshotUriResponse",
		"MediaUri",
		"/snapshot",
		"PT10S",
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("response XML missing %q\nBody:\n%s", want, body)
		}
	}
}

func TestGetSnapshotUri_ContextFallback(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}

	// Without client IP in context, falls back to mockConfig.DeviceIP()
	resp := handleGetSnapshotUri(context.Background(), cfg)
	if !strings.Contains(resp.MediaUri.Uri, "192.168.1.100") {
		t.Errorf("expected fallback IP in URI, got %q", resp.MediaUri.Uri)
	}

	// With client IP in context, the URI uses the client IP
	ctx := WithServerIP(context.Background(), "10.20.30.40")
	resp = handleGetSnapshotUri(ctx, cfg)
	if !strings.Contains(resp.MediaUri.Uri, "10.20.30.40") {
		t.Errorf("expected client IP in URI, got %q", resp.MediaUri.Uri)
	}
}

// ---------------------------------------------------------------------------
// Test SnapshotBuffer with RegisterSnapshotHandlers and live channel
// ---------------------------------------------------------------------------

func TestSnapshotHandlerFedFromChannel(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	frameCh := make(chan h264.AccessUnit, 16)

	srv := New(cfg)
	RegisterSnapshotHandlers(srv, NewSnapshotBuffer(), frameCh)

	// Send a key frame through the channel.
	frameCh <- h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("live-frame"), IsIDR: true}},
		Timestamp: time.Now(),
		KeyFrame:  true,
	}

	// Wait a bit for the goroutine to consume.
	<-time.After(50 * time.Millisecond)

	// The handler will try real ffmpeg and fail — we expect 503.
	// This tests the channel-feed + FFmpeg error path.
	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 (no ffmpeg available), got %d", w.Code)
	}
}

// ---------------------------------------------------------------------------
// Verify GetSnapshotUri response also parses with NVR-style raw SOAP
// ---------------------------------------------------------------------------

func TestGetSnapshotUriRawSOAP(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterSnapshotHandlers(srv, NewSnapshotBuffer(), make(chan h264.AccessUnit))

	soapReq := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
 xmlns:trt="http://www.onvif.org/ver10/media/wsdl">
  <s:Body>
    <trt:GetSnapshotUri>
      <trt:ProfileToken>main</trt:ProfileToken>
    </trt:GetSnapshotUri>
  </s:Body>
</s:Envelope>`)

	req := httptest.NewRequest(http.MethodPost, "/onvif/media_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	req.RemoteAddr = "192.168.1.100:55123"
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "http://192.168.1.100:8080/snapshot") {
		t.Errorf("response body missing snapshot URI.\nBody:\n%s", body)
	}


	// Verify with the same struct the NVR uses
	type trtGetSnapshotUriResponse struct {
		XMLName  xml.Name `xml:"GetSnapshotUriResponse"`
		MediaUri struct {
			URI string `xml:"Uri"`
		} `xml:"MediaUri"`
	}

	var env struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			GetSnapshotUriResponse trtGetSnapshotUriResponse `xml:"GetSnapshotUriResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse response with NVR-style struct: %v\nBody:\n%s", err, body)
	}

	expectedURI := "http://192.168.1.100:8080/snapshot"
	if env.Body.GetSnapshotUriResponse.MediaUri.URI != expectedURI {
		t.Errorf("expected URI %q, got %q", expectedURI, env.Body.GetSnapshotUriResponse.MediaUri.URI)
	}
}


// ---------------------------------------------------------------------------
// TOCTOU + concurrency limit tests
// ---------------------------------------------------------------------------

func TestSnapshotUsesConsistentFrame(t *testing.T) {
	buf := NewSnapshotBuffer()
	ts := time.Date(2024, 6, 15, 14, 30, 0, 0, time.UTC)
	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("test-frame-data"), IsIDR: true}},
		Timestamp: ts,
		KeyFrame:  true,
	})
	mockFFmpeg := writeMockFFmpeg(t, "mock-ffmpeg-consistent", false)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil).WithContext(ctx)
	w := httptest.NewRecorder()
	handleSnapshotHTTPWithBin(w, req, buf, mockFFmpeg)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	wantTS := "2024-06-15T14:30:00Z"
	gotTS := w.Header().Get("X-Frame-Timestamp")
	if gotTS != wantTS {
		t.Errorf("expected timestamp %q, got %q", wantTS, gotTS)
	}
}

func TestSnapshotConcurrencyLimit(t *testing.T) {
	buf := NewSnapshotBuffer()
	buf.Feed(h264.AccessUnit{
		NALUs:     []h264.NALU{{Type: 5, Data: []byte("test-frame"), IsIDR: true}},
		Timestamp: time.Now(),
		KeyFrame:  true,
	})
	mockFFmpeg := writeMockFFmpeg(t, "mock-ffmpeg-concurrency", false)

	// Fill semaphore to capacity so new requests must wait.
	snapshotSem <- struct{}{}
	snapshotSem <- struct{}{}
	defer func() { <-snapshotSem; <-snapshotSem }()

	req := httptest.NewRequest(http.MethodGet, "/snapshot", nil)
	w := httptest.NewRecorder()
	handleSnapshotHTTPWithBin(w, req, buf, mockFFmpeg)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503 when semaphore full, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "busy") {
		t.Errorf("expected 'busy' in response body, got %q", w.Body.String())
	}
}
