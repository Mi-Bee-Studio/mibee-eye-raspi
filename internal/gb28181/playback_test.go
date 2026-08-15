package gb28181

import (
	"bytes"
	"context"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/recording"
)

// testRecordingIndex is a playback-capable RecordingIndex backed by a
// temp-dir recording (implements PlaybackIndex).
type testRecordingIndex struct {
	root     string
	segments []recording.SegmentInfo
}

func (t *testRecordingIndex) Lookup(startMs, endMs int64) []recording.SegmentInfo {
	var out []recording.SegmentInfo
	for _, si := range t.segments {
		if si.EndMS >= startMs && si.StartMS <= endMs {
			out = append(out, si)
		}
	}
	return out
}

func (t *testRecordingIndex) Root() string { return t.root }

// synthFrame describes one frame of a synthetic recording.
type synthFrame struct {
	offset time.Duration
	key    bool
}

// writeSyntheticRecording writes a synthetic recording via the real
// recording.Writer into dir and returns the index snapshot.
func writeSyntheticRecording(t *testing.T, dir string, frames []synthFrame) []recording.SegmentInfo {
	t.Helper()
	hub := h264.NewAUHubWithSize(64)
	cfg := config.RecordingConfig{Enabled: true, StoragePath: dir, SegmentSecs: 60}
	w := recording.NewWriter(hub, cfg)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- w.Run(ctx) }()

	deadline := time.Now().Add(2 * time.Second)
	for hub.SubscriberCount() != 1 && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if hub.SubscriberCount() != 1 {
		t.Fatal("recording writer did not subscribe")
	}

	base := time.Now().Add(-time.Hour).Truncate(time.Second)
	for _, f := range frames {
		var types []byte
		if f.key {
			types = []byte{7, 8, 5} // SPS, PPS, IDR
		} else {
			types = []byte{1}
		}
		nalus := make([]h264.NALU, 0, len(types))
		for _, ty := range types {
			nalus = append(nalus, h264.NALU{Type: ty, Data: []byte{ty, 0x01, 0x02, 0x03}, IsIDR: ty == 5, IsSPS: ty == 7, IsPPS: ty == 8})
		}
		hub.Write(h264.AccessUnit{NALUs: nalus, Timestamp: base.Add(f.offset), KeyFrame: f.key})
	}
	time.Sleep(300 * time.Millisecond) // let the writer drain and finalize
	cancel()
	<-done

	segs := w.Index().Snapshot()
	if len(segs) == 0 {
		t.Fatal("no segments written")
	}
	return segs
}

// startPlaybackServer starts a test-mode gb28181 server with the given
// recording index (nil = none) and returns the server and SIP port.
func startPlaybackServer(t *testing.T, idx RecordingIndex) (*Server, int, context.CancelFunc) {
	t.Helper()
	hub := h264.NewAUHub()
	cfg := config.GB28181Config{
		DeviceID:              "34020000001320000001",
		ChannelID:             "34020000001320000001",
		SIPDomain:             "3402000000",
		Password:              "12345678",
		LocalSIPPort:          0,
		PlatformSIPAddress:    "127.0.0.1",
		PlatformSIPPort:       15068,
		RegisterIntervalSecs:  60,
		HeartbeatIntervalSecs: 60,
		HeartbeatTimeoutCount: 3,
	}
	server := New(cfg, config.DeviceConfig{}, hub)
	server.SetTestMode()
	if idx != nil {
		server.SetRecordingIndex(idx)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	go func() { _ = server.Start(ctx) }()
	time.Sleep(100 * time.Millisecond)
	sipPort := server.sipConn.LocalAddr().(*net.UDPAddr).Port
	return server, sipPort, cancel
}

// buildPlaybackInvite builds an INVITE with the given session type and
// t= range (unix seconds), pointing media at mediaPort.
func buildPlaybackInvite(callID, sessionType string, startSec, endSec int64, mediaPort int) SipMessage {
	sdp := fmt.Sprintf("v=0\no=- 0 0 IN IP4 127.0.0.1\ns=%s\nc=IN IP4 127.0.0.1\nt=%d %d\nm=video %d RTP/AVP 96\na=rtpmap:96 PS/90000\na=recvonly\ny=0100000001", sessionType, startSec, endSec, mediaPort)
	inv := buildInvite(callID,
		"<sip:34020000012000000001@3402000000>;tag=pb001",
		"<sip:34020000001320000001@3402000000>",
		"<sip:34020000012000000001@127.0.0.1:5060>")
	inv.Body = sdp
	return inv
}

// TestParseSDP_SessionType verifies s= extraction: Play/Playback/Download,
// case-insensitive, missing or unknown values defaulting to "Play".
func TestParseSDP_SessionType(t *testing.T) {
	base := "v=0\no=- 0 0 IN IP4 192.168.1.100\nc=IN IP4 192.168.1.100\nt=0 0\nm=video 60000 RTP/AVP 96\na=rtpmap:96 PS/90000\na=recvonly\ny=0100000001"
	cases := []struct {
		name string
		s    string // s= line content, "" = omit
		want string
	}{
		{"play", "Play", "Play"},
		{"playback", "Playback", "Playback"},
		{"download", "Download", "Download"},
		{"lowercase", "playback", "Playback"},
		{"mixed case", "PLAYBACK", "Playback"},
		{"missing", "", "Play"},
		{"unknown", "SomethingElse", "Play"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := base
			if tc.s != "" {
				body = "s=" + tc.s + "\n" + body
			}
			_, _, got, err := parseSDP(body)
			if err != nil {
				t.Fatalf("parseSDP: %v", err)
			}
			if got != tc.want {
				t.Errorf("session type = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestParseSDPTimeRange verifies t= extraction: unix seconds to ms range,
// t=0 0 and missing/malformed lines meaning "all".
func TestParseSDPTimeRange(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		wantStartMs int64
		wantEndMs   int64
	}{
		{"explicit", "v=0\ns=Playback\nt=1750000000 1750003600\n", 1750000000000, 1750003600000},
		{"zero zero means all", "v=0\ns=Playback\nt=0 0\n", 0, int64(^uint64(0) >> 1)},
		{"missing means all", "v=0\ns=Playback\n", 0, int64(^uint64(0) >> 1)},
		{"malformed means all", "v=0\ns=Playback\nt=abc def\n", 0, int64(^uint64(0) >> 1)},
		{"single field means all", "v=0\ns=Playback\nt=1750000000\n", 0, int64(^uint64(0) >> 1)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			start, end := parseSDPTimeRange(tc.body)
			if start != tc.wantStartMs || end != tc.wantEndMs {
				t.Errorf("parseSDPTimeRange = (%d, %d), want (%d, %d)", start, end, tc.wantStartMs, tc.wantEndMs)
			}
		})
	}
}

// TestBuildDeviceSDP_EchoesSessionType verifies the SDP answer echoes the
// session type in the s= line and that the UDP s=Play output is
// byte-identical to the pre-playback golden format.
func TestBuildDeviceSDP_EchoesSessionType(t *testing.T) {
	const (
		deviceID = "34020000001320000001"
		localIP  = "192.168.1.50"
		port     = 40000
		ssrc     = uint32(100000001)
	)
	udpPlay := buildDeviceSDP(deviceID, localIP, port, ssrc, "udp", "Play")
	wantUDP := `v=0
o=34020000001320000001 0 0 IN IP4 192.168.1.50
s=Play
c=IN IP4 192.168.1.50
t=0 0
m=video 40000 RTP/AVP 96
a=sendonly
a=rtpmap:96 PS/90000
y=100000001`
	if udpPlay != wantUDP {
		t.Errorf("UDP s=Play SDP not byte-identical to golden:\ngot:\n%s\nwant:\n%s", udpPlay, wantUDP)
	}

	for _, st := range []string{"Playback", "Download"} {
		got := buildDeviceSDP(deviceID, localIP, port, ssrc, "udp", st)
		if !containsSubstring(got, "s="+st) {
			t.Errorf("UDP SDP missing s=%s:\n%s", st, got)
		}
		if strings.Replace(got, "s="+st, "s=Play", 1) != udpPlay {
			t.Errorf("UDP SDP for %s differs beyond the s= line:\n%s", st, got)
		}
	}

	tcpPlay := buildDeviceSDP(deviceID, localIP, port, ssrc, "tcp", "Play")
	if !containsSubstring(tcpPlay, "s=Play") || !containsSubstring(tcpPlay, "TCP/RTP/AVP") {
		t.Errorf("TCP s=Play SDP malformed:\n%s", tcpPlay)
	}
	for _, st := range []string{"Playback", "Download"} {
		got := buildDeviceSDP(deviceID, localIP, port, ssrc, "tcp", st)
		if !containsSubstring(got, "s="+st) {
			t.Errorf("TCP SDP missing s=%s:\n%s", st, got)
		}
		if strings.Replace(got, "s="+st, "s=Play", 1) != tcpPlay {
			t.Errorf("TCP SDP for %s differs beyond the s= line:\n%s", st, got)
		}
	}
}

// TestServer_PlaybackStreamsRecordingWithPacing verifies an end-to-end
// Playback INVITE: 200 OK echoes s=Playback, RTP flows to the SDP media
// address, the first frame is a keyframe (PSM present), and frames are
// paced at the recorded ~100ms interval.
func TestServer_PlaybackStreamsRecordingWithPacing(t *testing.T) {
	dir := t.TempDir()
	frames := []synthFrame{{0, true}}
	for i := 1; i < 10; i++ {
		frames = append(frames, synthFrame{time.Duration(i) * 100 * time.Millisecond, false})
	}
	segs := writeSyntheticRecording(t, dir, frames)
	idx := &testRecordingIndex{root: dir, segments: segs}

	_, sipPort, cancel := startPlaybackServer(t, idx)
	defer cancel()

	mediaSock, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("bind media socket: %v", err)
	}
	defer mediaSock.Close()
	mediaPort := mediaSock.LocalAddr().(*net.UDPAddr).Port

	invite := buildPlaybackInvite("test-call-pb@example.com", "Playback", segs[0].StartMS/1000, segs[0].EndMS/1000+1, mediaPort)
	clientConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: sipPort})
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write(invite.Serialize()); err != nil {
		t.Fatalf("send INVITE: %v", err)
	}

	respBuf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientConn.ReadFromUDP(respBuf)
	if err != nil {
		t.Fatalf("read 200 OK: %v", err)
	}
	resp, err := Parse(respBuf[:n])
	if err != nil {
		t.Fatalf("parse 200 OK: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if !containsSubstring(resp.Body, "s=Playback") {
		t.Fatalf("SDP answer does not echo s=Playback:\n%s", resp.Body)
	}

	// One RTP packet per frame; record arrival times.
	var arrivals []time.Time
	var firstPayload []byte
	mediaSock.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2048)
	for len(arrivals) < len(frames) {
		n, _, err := mediaSock.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("read RTP frame %d: %v", len(arrivals), err)
		}
		if n < 12 || buf[0]>>6 != 2 {
			t.Fatalf("packet %d is not RTP v2 (len=%d)", len(arrivals), n)
		}
		if len(arrivals) == 0 {
			firstPayload = append([]byte(nil), buf[:n]...)
		}
		arrivals = append(arrivals, time.Now())
	}

	// First frame must be a keyframe: PSM (0x000001BB) is only emitted on
	// keyframes by MuxH264ToPS.
	if !bytes.Contains(firstPayload, []byte{0x00, 0x00, 0x01, 0xBB}) {
		t.Fatal("first playback frame does not contain PSM (not a keyframe)")
	}

	// Pacing: inter-packet gaps approximate the 100ms frame interval.
	for i := 1; i < len(arrivals); i++ {
		gap := arrivals[i].Sub(arrivals[i-1])
		if gap < 40*time.Millisecond || gap > 250*time.Millisecond {
			t.Errorf("frame gap %d = %v, want ~100ms (paced)", i, gap)
		}
	}
}

// TestServer_DownloadStreamsWithoutPacing verifies a Download INVITE sends
// all frames as fast as possible (no pacing) and starts from a keyframe.
func TestServer_DownloadStreamsWithoutPacing(t *testing.T) {
	dir := t.TempDir()
	frames := []synthFrame{{0, true}}
	for i := 1; i < 10; i++ {
		frames = append(frames, synthFrame{time.Duration(i) * 100 * time.Millisecond, false})
	}
	segs := writeSyntheticRecording(t, dir, frames)
	idx := &testRecordingIndex{root: dir, segments: segs}

	_, sipPort, cancel := startPlaybackServer(t, idx)
	defer cancel()

	mediaSock, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("bind media socket: %v", err)
	}
	defer mediaSock.Close()
	mediaPort := mediaSock.LocalAddr().(*net.UDPAddr).Port

	invite := buildPlaybackInvite("test-call-dl@example.com", "Download", segs[0].StartMS/1000, segs[0].EndMS/1000+1, mediaPort)
	clientConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: sipPort})
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write(invite.Serialize()); err != nil {
		t.Fatalf("send INVITE: %v", err)
	}

	respBuf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientConn.ReadFromUDP(respBuf)
	if err != nil {
		t.Fatalf("read 200 OK: %v", err)
	}
	resp, err := Parse(respBuf[:n])
	if err != nil {
		t.Fatalf("parse 200 OK: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected 200 OK, got %d", resp.StatusCode)
	}
	if !containsSubstring(resp.Body, "s=Download") {
		t.Fatalf("SDP answer does not echo s=Download:\n%s", resp.Body)
	}

	// All frames must arrive well under the 1s recording duration.
	start := time.Now()
	var firstPayload []byte
	mediaSock.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2048)
	for i := 0; i < len(frames); i++ {
		n, _, err := mediaSock.ReadFromUDP(buf)
		if err != nil {
			t.Fatalf("read RTP frame %d: %v", i, err)
		}
		if i == 0 {
			firstPayload = append([]byte(nil), buf[:n]...)
		}
	}
	if elapsed := time.Since(start); elapsed > 500*time.Millisecond {
		t.Fatalf("download took %v, want < 500ms (no pacing)", elapsed)
	}
	if !bytes.Contains(firstPayload, []byte{0x00, 0x00, 0x01, 0xBB}) {
		t.Fatal("first download frame does not contain PSM (not a keyframe)")
	}
}

// TestServer_ByeStopsPlaybackStreaming verifies BYE cancels the playback
// goroutine: no RTP arrives after the BYE is processed.
func TestServer_ByeStopsPlaybackStreaming(t *testing.T) {
	dir := t.TempDir()
	frames := []synthFrame{{0, true}}
	for i := 1; i < 20; i++ {
		frames = append(frames, synthFrame{time.Duration(i) * 100 * time.Millisecond, false})
	}
	segs := writeSyntheticRecording(t, dir, frames)
	idx := &testRecordingIndex{root: dir, segments: segs}

	_, sipPort, cancel := startPlaybackServer(t, idx)
	defer cancel()

	mediaSock, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("bind media socket: %v", err)
	}
	defer mediaSock.Close()
	mediaPort := mediaSock.LocalAddr().(*net.UDPAddr).Port

	invite := buildPlaybackInvite("test-call-bye-pb@example.com", "Playback", segs[0].StartMS/1000, segs[0].EndMS/1000+1, mediaPort)
	clientConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: sipPort})
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write(invite.Serialize()); err != nil {
		t.Fatalf("send INVITE: %v", err)
	}

	respBuf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := clientConn.ReadFromUDP(respBuf); err != nil {
		t.Fatalf("read 200 OK: %v", err)
	}

	// Receive 3 frames (stream is flowing), then BYE.
	mediaSock.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2048)
	for i := 0; i < 3; i++ {
		if _, _, err := mediaSock.ReadFromUDP(buf); err != nil {
			t.Fatalf("read frame %d: %v", i, err)
		}
	}
	bye := buildBye("test-call-bye-pb@example.com",
		"<sip:34020000012000000001@3402000000>;tag=pb001",
		"<sip:34020000001320000001@3402000000>",
		"<sip:34020000012000000001@127.0.0.1:5060>")
	if _, err := clientConn.Write(bye.Serialize()); err != nil {
		t.Fatalf("send BYE: %v", err)
	}

	// No more RTP within 400ms.
	mediaSock.SetReadDeadline(time.Now().Add(400 * time.Millisecond))
	for {
		if _, _, err := mediaSock.ReadFromUDP(buf); err != nil {
			break // timeout — streaming stopped, good
		}
		t.Fatal("RTP still arriving after BYE")
	}
}

// TestServer_PlaybackFastForwardsToKeyframe verifies that a mid-GOP start
// fast-forwards to the next keyframe: the first sent frame contains PSM.
func TestServer_PlaybackFastForwardsToKeyframe(t *testing.T) {
	dir := t.TempDir()
	frames := []synthFrame{{0, true}}
	for i := 1; i < 20; i++ {
		frames = append(frames, synthFrame{time.Duration(i) * 100 * time.Millisecond, false})
	}
	// Second keyframe at 2s.
	frames = append(frames, synthFrame{2000 * time.Millisecond, true})
	for i := 21; i < 30; i++ {
		frames = append(frames, synthFrame{time.Duration(i) * 100 * time.Millisecond, false})
	}
	segs := writeSyntheticRecording(t, dir, frames)
	idx := &testRecordingIndex{root: dir, segments: segs}

	_, sipPort, cancel := startPlaybackServer(t, idx)
	defer cancel()

	mediaSock, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatalf("bind media socket: %v", err)
	}
	defer mediaSock.Close()
	mediaPort := mediaSock.LocalAddr().(*net.UDPAddr).Port

	// Request playback starting 1s in (mid-GOP, non-keyframe).
	invite := buildPlaybackInvite("test-call-ff@example.com", "Playback", segs[0].StartMS/1000+1, segs[0].EndMS/1000+1, mediaPort)
	clientConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: sipPort})
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer clientConn.Close()

	if _, err := clientConn.Write(invite.Serialize()); err != nil {
		t.Fatalf("send INVITE: %v", err)
	}

	respBuf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, _, err := clientConn.ReadFromUDP(respBuf); err != nil {
		t.Fatalf("read 200 OK: %v", err)
	}

	// First received frame must be the keyframe at 2s (contains PSM).
	mediaSock.SetReadDeadline(time.Now().Add(5 * time.Second))
	buf := make([]byte, 2048)
	n, _, err := mediaSock.ReadFromUDP(buf)
	if err != nil {
		t.Fatalf("read first frame: %v", err)
	}
	if !bytes.Contains(buf[:n], []byte{0x00, 0x00, 0x01, 0xBB}) {
		t.Fatal("first frame after mid-GOP start is not a keyframe (no PSM)")
	}
}

// TestServer_PlaybackInviteWithoutIndexGets488 verifies a Playback INVITE
// with no recording index is rejected with 488 Not Acceptable Here.
func TestServer_PlaybackInviteWithoutIndexGets488(t *testing.T) {
	_, sipPort, cancel := startPlaybackServer(t, nil)
	defer cancel()

	clientConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: sipPort})
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer clientConn.Close()

	invite := buildPlaybackInvite("test-call-488a@example.com", "Playback", 1, 2, 60000)
	if _, err := clientConn.Write(invite.Serialize()); err != nil {
		t.Fatalf("send INVITE: %v", err)
	}

	respBuf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientConn.ReadFromUDP(respBuf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp, err := Parse(respBuf[:n])
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.StatusCode != 488 {
		t.Fatalf("expected 488 Not Acceptable Here, got %d", resp.StatusCode)
	}
}

// TestServer_PlaybackInviteWithoutCoveringSegmentsGets488 verifies a
// Playback INVITE whose t= range overlaps no recordings is rejected with 488.
func TestServer_PlaybackInviteWithoutCoveringSegmentsGets488(t *testing.T) {
	dir := t.TempDir()
	segs := writeSyntheticRecording(t, dir, []synthFrame{{0, true}})
	idx := &testRecordingIndex{root: dir, segments: segs}

	_, sipPort, cancel := startPlaybackServer(t, idx)
	defer cancel()

	clientConn, err := net.DialUDP("udp", nil, &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: sipPort})
	if err != nil {
		t.Fatalf("dial server: %v", err)
	}
	defer clientConn.Close()

	// t= range far in the past (2001) — no overlap with the recording.
	invite := buildPlaybackInvite("test-call-488b@example.com", "Playback", 1000000000, 1000000100, 60000)
	if _, err := clientConn.Write(invite.Serialize()); err != nil {
		t.Fatalf("send INVITE: %v", err)
	}

	respBuf := make([]byte, 4096)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	n, _, err := clientConn.ReadFromUDP(respBuf)
	if err != nil {
		t.Fatalf("read response: %v", err)
	}
	resp, err := Parse(respBuf[:n])
	if err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp.StatusCode != 488 {
		t.Fatalf("expected 488 Not Acceptable Here, got %d", resp.StatusCode)
	}
}
