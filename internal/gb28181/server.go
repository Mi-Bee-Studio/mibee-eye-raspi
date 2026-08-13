// Package gb28181 implements the GB/T 28181 server entrypoint —
// SIP UDP listener lifecycle and orchestration of signaling and
// media streaming components.
//
// SIZE_OK: This file exceeds 250 LOC but is a cohesive, indivisible module.
// All functionality is tightly coupled around the Server struct lifecycle:
// - SIP UDP listener and message dispatch
// - REGISTER authentication flow with digest auth
// - Keepalive heartbeat with re-registration
// - INVITE handling with SDP parsing, media binding, 200 OK response
// - AUHub subscription and media goroutine (PS mux + RTP push)
// - BYE handling with cleanup
// - MESSAGE handling with MANSCDP dispatch
// Splitting would create artificial boundaries that don't reflect the actual
// logical structure of the GB28181 protocol lifecycle.
package gb28181

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)

// Server implements the GB/T 28181 SIP server lifecycle.
// Manages SIP signaling, digest authentication, keepalive, and media streaming.
type Server struct {
	cfg         config.GB28181Config
	hub         *h264.AUHub
	sipConn     *net.UDPConn
	mediaConn   *net.UDPConn
	mu          sync.Mutex
	cancel      context.CancelFunc
	mediaCancel context.CancelFunc
	sub         *h264.Subscriber
	// Remote address for RTP streaming
	remoteRTPAddr *net.UDPAddr
	// testMode skips REGISTER lifecycle for testing
	testMode bool
}

// New creates a new GB28181 server.
func New(cfg config.GB28181Config, hub *h264.AUHub) *Server {
	return &Server{
		cfg: cfg,
		hub: hub,
	}
}

// SetTestMode enables test mode which skips REGISTER lifecycle.
func (s *Server) SetTestMode() {
	s.testMode = true
}

// Start starts the GB28181 server SIP listener and lifecycle.
func (s *Server) Start(ctx context.Context) error {
	// Bind SIP UDP
	sipAddr := &net.UDPAddr{Port: s.cfg.LocalSIPPort}
	sipConn, err := net.ListenUDP("udp", sipAddr)
	if err != nil {
		return fmt.Errorf("binding SIP UDP on port %d: %w", s.cfg.LocalSIPPort, err)
	}
	s.sipConn = sipConn
	slog.Info("gb28181: SIP UDP listener started", "port", s.cfg.LocalSIPPort)

	// Create child context with cancel for lifecycle management
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// Run REGISTER lifecycle (skip in test mode)
	if !s.testMode {
		if err := s.runRegisterLifecycle(ctx); err != nil {
			return fmt.Errorf("REGISTER lifecycle failed: %w", err)
		}
	}

	// Keepalive failure counter (declared here for all modes)
	keepaliveFailures := 0

	// Spawn keepalive goroutine (skip in test mode)
	if !s.testMode {
		heartbeatInterval := time.Duration(s.cfg.HeartbeatIntervalSecs) * time.Second
		go func() {
			ticker := time.NewTicker(heartbeatInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					if err := s.sendKeepalive(ctx); err != nil {
						keepaliveFailures++
						slog.Warn("gb28181: keepalive failed", "error", err, "failures", keepaliveFailures)
						if keepaliveFailures >= s.cfg.HeartbeatTimeoutCount {
							slog.Warn("gb28181: too many keepalive failures, re-registering")
							if err := s.runRegisterLifecycle(ctx); err != nil {
								slog.Error("gb28181: re-register failed", "error", err)
							}
							keepaliveFailures = 0
						}
					} else {
						keepaliveFailures = 0
					}
				}
			}
		}()
	}

	// Enter SIP recv loop
	buf := make([]byte, 4096)
	for {
		select {
		case <-ctx.Done():
			slog.Info("gb28181: SIP recv loop stopped")
			return nil
		default:
			// Set read deadline for shutdown responsiveness
			sipConn.SetReadDeadline(time.Now().Add(1 * time.Second))
			n, addr, err := sipConn.ReadFromUDP(buf)
			if err != nil {
				if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
					continue // Timeout is expected for shutdown check
				}
				slog.Warn("gb28181: SIP recv error", "error", err)
				continue
			}
			msg, err := Parse(buf[:n])
			if err != nil {
				slog.Warn("gb28181: failed to parse SIP message", "error", err)
				continue
			}

			// Handle based on method
			switch msg.Method {
			case "INVITE":
				s.handleInvite(ctx, msg, addr)
			case "BYE":
				s.handleBye(ctx, msg, addr)
			case "MESSAGE":
				s.handleMessage(ctx, msg, addr)
			case "ACK":
				// No action needed - media is now flowing
			case "200":
				// Response to our keepalive - reset failure counter
				keepaliveFailures = 0
			default:
				slog.Debug("gb28181: unhandled SIP method", "method", msg.Method)
			}
		}
	}
}

// Stop stops the server.
func (s *Server) Stop() {
	if s.cancel != nil {
		s.cancel()
	}
	if s.mediaCancel != nil {
		s.mediaCancel()
	}
	if s.mediaConn != nil {
		s.mediaConn.Close()
	}
	if s.sipConn != nil {
		s.sipConn.Close()
	}
}

// parseSDP extracts media address and SSRC from SDP body.
// Returns (mediaAddr, ssrc, err).
func parseSDP(body string) (string, uint32, error) {
	var mediaAddr string
	var ssrc uint32
	var ssrcFound bool

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "c=") {
			// Connection: c=IN IP4 <address>
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				mediaAddr = parts[2]
			}
		} else if strings.HasPrefix(line, "y=") {
			// SSRC: y=<10-digit decimal>
			ssrcStr := strings.TrimPrefix(line, "y=")
			ssrcVal, err := strconv.ParseUint(ssrcStr, 10, 32)
			if err != nil {
				return "", 0, fmt.Errorf("invalid SSRC value: %s: %w", ssrcStr, err)
			}
			ssrc = uint32(ssrcVal)
			ssrcFound = true
		}
	}

	if !ssrcFound {
		return "", 0, fmt.Errorf("SDP missing y= SSRC line")
	}

	return mediaAddr, ssrc, nil
}

// buildDeviceSDP builds the device SDP answer for INVITE 200 OK.
func buildDeviceSDP(deviceID, localIP string, mediaPort int, ssrc uint32) string {
	return fmt.Sprintf(`v=0
o=%s 0 0 IN IP4 %s
s=Play
c=IN IP4 %s
t=0 0
m=video %d RTP/AVP 96
a=sendonly
a=rtpmap:96 PS/90000
y=%d`,
		deviceID, localIP, localIP, mediaPort, ssrc)
}

// localIP detects a local IP address or returns 0.0.0.0 placeholder.
func localIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "0.0.0.0"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() {
			if ipnet.IP.To4() != nil {
				return ipnet.IP.String()
			}
		}
	}
	return "0.0.0.0"
}

// getLocalIP determines the local source IP that would be used to reach
// remoteAddr, by dialing a temporary UDP connection (no packets are sent).
func getLocalIP(remoteAddr string) (string, error) {
	conn, err := net.Dial("udp", remoteAddr)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	localAddr := conn.LocalAddr().(*net.UDPAddr)
	return localAddr.IP.String(), nil
}

// runRegisterLifecycle performs the REGISTER authentication flow.
func (s *Server) runRegisterLifecycle(ctx context.Context) error {
	requestURI := fmt.Sprintf("sip:%s@%s", s.cfg.SIPDomain, s.cfg.SIPDomain)
	from := fmt.Sprintf("<sip:%s@%s>", s.cfg.DeviceID, s.cfg.SIPDomain)
	to := from
	platformAddr := &net.UDPAddr{
		IP:   net.ParseIP(s.cfg.PlatformSIPAddress),
		Port: s.cfg.PlatformSIPPort,
	}

	// Determine the real local IP toward the platform for Via/Contact headers
	localIPAddr, err := getLocalIP(platformAddr.String())
	if err != nil {
		slog.Warn("gb28181: failed to determine local IP, falling back to interface scan", "error", err)
		localIPAddr = localIP()
	}
	callID := fmt.Sprintf("%d@%s", time.Now().Unix(), localIPAddr)
	cseq := "1 REGISTER"
	contact := fmt.Sprintf("<sip:%s@%s:%d>", s.cfg.DeviceID, localIPAddr, s.cfg.LocalSIPPort)
	via := fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bK%016x", localIPAddr, s.cfg.LocalSIPPort, time.Now().UnixNano())

	// Initial REGISTER
	slog.Info("gb28181: sending initial REGISTER")
	regMsg := BuildRegister(requestURI, from, to, callID, cseq, contact, "")
	regMsg.Via = via
	if _, err := s.sipConn.WriteToUDP(regMsg.Serialize(), platformAddr); err != nil {
		return fmt.Errorf("sending REGISTER: %w", err)
	}

	// Wait for response
	buf := make([]byte, 4096)
	n, _, err := s.sipConn.ReadFromUDP(buf)
	if err != nil {
		return fmt.Errorf("reading REGISTER response: %w", err)
	}
	resp, err := Parse(buf[:n])
	if err != nil {
		return fmt.Errorf("parsing REGISTER response: %w", err)
	}

	// Handle 401 Unauthorized
	if resp.StatusCode == 401 {
		slog.Info("gb28181: received 401, authenticating")
		auth, err := ParseChallenge(resp.WWWAuthenticate)
		if err != nil {
			return fmt.Errorf("parsing digest challenge: %w", err)
		}

		authHeader := BuildAuthorizationHeader(auth, s.cfg.DeviceID, s.cfg.Password, requestURI, "REGISTER")
		cseq = "2 REGISTER"
		authMsg := BuildRegister(requestURI, from, to, callID, cseq, contact, authHeader)
		authMsg.Via = via

		if _, err := s.sipConn.WriteToUDP(authMsg.Serialize(), platformAddr); err != nil {
			return fmt.Errorf("sending authenticated REGISTER: %w", err)
		}

		// Wait for 200 OK
		n, _, err = s.sipConn.ReadFromUDP(buf)
		if err != nil {
			return fmt.Errorf("reading 200 OK response: %w", err)
		}
		resp, err = Parse(buf[:n])
		if err != nil {
			return fmt.Errorf("parsing 200 OK response: %w", err)
		}

		if resp.StatusCode == 200 {
			slog.Info("gb28181: REGISTER successful")
			return nil
		}
		return fmt.Errorf("unexpected response after auth: %d", resp.StatusCode)
	}

	if resp.StatusCode == 200 {
		slog.Info("gb28181: REGISTER successful (no auth required)")
		return nil
	}

	return fmt.Errorf("unexpected REGISTER response: %d", resp.StatusCode)
}

// sendKeepalive sends a keepalive MESSAGE to the platform.
func (s *Server) sendKeepalive(ctx context.Context) error {
	platformAddr := &net.UDPAddr{
		IP:   net.ParseIP(s.cfg.PlatformSIPAddress),
		Port: s.cfg.PlatformSIPPort,
	}

	requestURI := fmt.Sprintf("sip:%s@%s", s.cfg.SIPDomain, s.cfg.SIPDomain)
	from := fmt.Sprintf("<sip:%s@%s>", s.cfg.DeviceID, s.cfg.SIPDomain)
	to := from

	// Determine the real local IP toward the platform for Via/Contact headers
	localIPAddr, err := getLocalIP(platformAddr.String())
	if err != nil {
		slog.Warn("gb28181: failed to determine local IP, falling back to interface scan", "error", err)
		localIPAddr = localIP()
	}
	callID := fmt.Sprintf("keepalive-%d@%s", time.Now().Unix(), localIPAddr)
	contact := fmt.Sprintf("<sip:%s@%s:%d>", s.cfg.DeviceID, localIPAddr, s.cfg.LocalSIPPort)

	msg := BuildKeepaliveMessage(strconv.FormatInt(time.Now().Unix(), 10), s.cfg.DeviceID, "OK")
	msg.RequestURI = requestURI
	msg.From = from
	msg.To = to
	msg.CallID = callID
	msg.Contact = contact
	msg.CSeq = "1 MESSAGE"
	msg.Via = fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bK%016x", localIPAddr, s.cfg.LocalSIPPort, time.Now().UnixNano())

	if _, err := s.sipConn.WriteToUDP(msg.Serialize(), platformAddr); err != nil {
		return fmt.Errorf("sending keepalive MESSAGE: %w", err)
	}

	return nil
}

// handleInvite handles INVITE requests - parses SDP, binds media, sends 200 OK with device SDP, subscribes to AUHub.
func (s *Server) handleInvite(ctx context.Context, msg SipMessage, fromAddr *net.UDPAddr) {
	slog.Info("gb28181: received INVITE", "from", fromAddr.String())

	// Parse SDP for SSRC
	_, ssrc, err := parseSDP(msg.Body)
	if err != nil {
		slog.Warn("gb28181: failed to parse INVITE SDP", "error", err)
		return
	}
	slog.Info("gb28181: parsed SSRC from INVITE", "ssrc", ssrc)

	// Bind local media UDP on ephemeral port
	mediaConn, err := net.ListenUDP("udp", nil)
	if err != nil {
		slog.Warn("gb28181: failed to bind media UDP", "error", err)
		return
	}
	localMediaPort := mediaConn.LocalAddr().(*net.UDPAddr).Port
	slog.Info("gb28181: bound media UDP", "port", localMediaPort)

	s.mu.Lock()
	s.mediaConn = mediaConn
	s.mu.Unlock()

	// Build device SDP answer
	localIPAddr := localIP()
	deviceSDP := buildDeviceSDP(s.cfg.DeviceID, localIPAddr, localMediaPort, ssrc)

	// Send 200 OK with SDP answer
	ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "application/sdp", deviceSDP)
	if _, err := s.sipConn.WriteToUDP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK", "error", err)
		return
	}

	// Subscribe to AUHub
	sub := s.hub.Subscribe(ctx)
	s.mu.Lock()
	s.sub = sub
	s.remoteRTPAddr = fromAddr
	s.mu.Unlock()
	slog.Info("gb28181: subscribed to AUHub", "sub_id", sub.ID)

	// Create media context for goroutine
	mediaCtx, mediaCancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.mediaCancel = mediaCancel
	s.mu.Unlock()

	// Spawn goroutine draining AU + PS mux + RTP push
	go func() {
		defer mediaCancel()
		pusher := NewRtpPusher(mediaConn, fromAddr)
		slog.Info("gb28181: media goroutine started", "remote", fromAddr.String())

		for {
			select {
			case <-mediaCtx.Done():
				slog.Info("gb28181: media goroutine stopped")
				return
			case au, ok := <-sub.Channel:
				if !ok {
					slog.Info("gb28181: AU channel closed")
					return
				}
				// Convert []NALU to [][]byte for PS muxing
				naluBytes := make([][]byte, len(au.NALUs))
				for i, nalu := range au.NALUs {
					naluBytes[i] = nalu.Data
				}
				// Mux H.264 to PS
				psData := MuxH264ToPS(naluBytes, au.KeyFrame, au.Timestamp, au.Timestamp)
				// Send PS data over RTP
				if err := pusher.SendFrame(psData, au.KeyFrame, au.Timestamp, ssrc); err != nil {
					slog.Warn("gb28181: failed to send RTP frame", "error", err)
				}
			}
		}
	}()
}

// handleBye handles BYE requests - unsubscribe from AUHub and close media socket.
func (s *Server) handleBye(ctx context.Context, msg SipMessage, fromAddr *net.UDPAddr) {
	slog.Info("gb28181: received BYE", "from", fromAddr.String())

	s.mu.Lock()
	if s.sub != nil {
		s.hub.Unsubscribe(s.sub.ID)
		s.sub = nil
	}
	if s.mediaConn != nil {
		s.mediaConn.Close()
		s.mediaConn = nil
	}
	if s.mediaCancel != nil {
		s.mediaCancel()
		s.mediaCancel = nil
	}
	s.remoteRTPAddr = nil
	s.mu.Unlock()

	// Send 200 OK to BYE
	ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
	if _, err := s.sipConn.WriteToUDP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK to BYE", "error", err)
	}
	slog.Info("gb28181: sent 200 OK to BYE")
}

// handleMessage handles MESSAGE requests - dispatch MANSCDP XML, send 200 OK, and any queued response.
func (s *Server) handleMessage(ctx context.Context, msg SipMessage, fromAddr *net.UDPAddr) {
	ok200, queuedResp, err := DispatchInboundMessage(msg)
	if err != nil {
		slog.Warn("gb28181: failed to dispatch MESSAGE", "error", err)
		return
	}

	// Send 200 OK
	if _, err := s.sipConn.WriteToUDP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK to MESSAGE", "error", err)
	}
	if _, err := s.sipConn.WriteToUDP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK to MESSAGE", "error", err)
	}

	// Send queued response if any
	if queuedResp != nil {
		queuedResp.RequestURI = fmt.Sprintf("sip:%s@%s", s.cfg.SIPDomain, s.cfg.SIPDomain)
		queuedResp.From = msg.To
		queuedResp.To = msg.From
		queuedResp.CallID = msg.CallID
		queuedResp.Contact = msg.Contact
		queuedResp.CSeq = "2 MESSAGE"
		if _, err := s.sipConn.WriteToUDP(queuedResp.Serialize(), fromAddr); err != nil {
			slog.Warn("gb28181: failed to send queued MESSAGE response", "error", err)
		}
	}
}
