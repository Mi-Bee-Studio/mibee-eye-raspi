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
	"sync/atomic"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)

type Server struct {
	cfg          config.GB28181Config
	deviceCfg    config.DeviceConfig
	hub          *h264.AUHub
	sipConn      *net.UDPConn
	mediaConn    *net.UDPConn
	mediaTCPConn *net.TCPConn
	// tcpListener is the TCP SIP listener (transport="tcp" only)
	tcpListener net.Listener
	// tcpConns tracks active TCP SIP connections keyed by remote address
	tcpConns    sync.Map
	mu          sync.Mutex
	cancel      context.CancelFunc
	mediaCancel context.CancelFunc
	sub         *h264.Subscriber
	// Remote address for RTP streaming
	remoteRTPAddr *net.UDPAddr
	// regRespCh routes REGISTER responses from the recv loop to an active
	// registration attempt (prevents both readers racing on sipConn).
	regRespCh chan SipMessage
	// reRegisterCh signals the lifecycle goroutine to re-register now
	// (keepalive 401/403 rejection, send failures, periodic tick).
	reRegisterCh chan struct{}
	// regMu serializes registration attempts.
	regMu sync.Mutex
	// keepaliveFailures counts consecutive keepalive failures/errors.
	keepaliveFailures atomic.Int32
	// testMode skips REGISTER lifecycle for testing
	testMode bool
	// devContext holds device identity for MANSCDP responses
	devCtx DeviceContext
}

// New creates a new GB28181 server.
func New(cfg config.GB28181Config, deviceCfg config.DeviceConfig, hub *h264.AUHub) *Server {
	return &Server{
		cfg:          cfg,
		deviceCfg:    deviceCfg,
		hub:          hub,
		regRespCh:    make(chan SipMessage, 4),
		reRegisterCh: make(chan struct{}, 1),
	}
}

// SetTestMode enables test mode which skips REGISTER lifecycle.
func (s *Server) SetTestMode() {
	s.testMode = true
}

// Start starts the GB28181 server SIP listener and lifecycle.
func (s *Server) Start(ctx context.Context) error {
	// Initialize device context for MANSCDP responses
	s.devCtx = DeviceContext{
		DeviceID:     s.cfg.DeviceID,
		ChannelID:    s.cfg.ChannelID,
		Name:         s.deviceCfg.Name,
		Manufacturer: s.deviceCfg.Manufacturer,
		Model:        s.deviceCfg.Model,
		Firmware:     s.deviceCfg.Firmware,
		LocalIP:      localIP(),
		LocalPort:    s.cfg.LocalSIPPort,
	}

	// Create child context with cancel for lifecycle management
	ctx, cancel := context.WithCancel(ctx)
	s.cancel = cancel

	// TCP transport: listen for inbound SIP connections. The platform
	// connects to us and drives the dialog (no outbound REGISTER lifecycle).
	if s.cfg.Transport == "tcp" {
		return s.startTCPListener(ctx)
	}

	// Bind SIP UDP
	sipAddr := &net.UDPAddr{Port: s.cfg.LocalSIPPort}
	sipConn, err := net.ListenUDP("udp", sipAddr)
	if err != nil {
		return fmt.Errorf("binding SIP UDP on port %d: %w", s.cfg.LocalSIPPort, err)
	}
	s.sipConn = sipConn
	slog.Info("gb28181: SIP UDP listener started", "port", s.cfg.LocalSIPPort)

	// Run REGISTER lifecycle (skip in test mode)
	if !s.testMode {
		if err := s.runRegisterLifecycle(ctx); err != nil {
			return fmt.Errorf("REGISTER lifecycle failed: %w", err)
		}
	}

	// Keepalive + registration lifecycle (skip in test mode)
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
						failures := s.keepaliveFailures.Add(1)
						slog.Warn("gb28181: keepalive send failed", "error", err, "failures", failures)
						if failures >= int32(s.cfg.HeartbeatTimeoutCount) {
							slog.Warn("gb28181: too many keepalive failures, re-registering")
							s.signalReRegister()
							s.keepaliveFailures.Store(0)
						}
					}
				}
			}
		}()

		// Re-registration lifecycle: periodic refresh before Expires and
		// immediate retry when the platform rejects us (401/403 keepalive —
		// e.g. after an NVR restart that forgot our registration).
		go func() {
			registerInterval := time.Duration(s.cfg.RegisterIntervalSecs) * time.Second
			ticker := time.NewTicker(registerInterval)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					slog.Info("gb28181: periodic re-registration")
				case <-s.reRegisterCh:
					slog.Info("gb28181: re-registration triggered (keepalive rejected or failures)")
				}
				if err := s.reregister(ctx); err != nil {
					slog.Warn("gb28181: re-register failed", "error", err)
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

			// Handle responses vs requests separately. Responses have
			// StatusCode set and Method empty — the old switch never matched
			// them (the "200" case was dead code).
			if msg.StatusCode > 0 {
				s.handleResponse(msg)
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
			case "SUBSCRIBE", "NOTIFY", "INFO", "OPTIONS":
				slog.Info("gb28181: received method, responding 200 OK", "method", msg.Method, "from", addr.String())
				ok200 := Build200OK(msg, "", "")
				if _, err := s.sipConn.WriteToUDP(ok200.Serialize(), addr); err != nil {
					slog.Warn("gb28181: failed to send 200 OK", "method", msg.Method, "error", err)
				}
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
	if s.mediaTCPConn != nil {
		s.mediaTCPConn.Close()
	}
	if s.tcpListener != nil {
		s.tcpListener.Close()
	}
	if s.sipConn != nil {
		s.sipConn.Close()
	}
}

// sendSIP sends a SIP message to the given peer, dispatching to UDP or TCP.
func (s *Server) sendSIP(data []byte, addr net.Addr) error {
	if udpAddr, ok := addr.(*net.UDPAddr); ok {
		_, err := s.sipConn.WriteToUDP(data, udpAddr)
		return err
	}
	if tcpAddr, ok := addr.(*net.TCPAddr); ok {
		return s.sendToTCP(data, tcpAddr)
	}
	return fmt.Errorf("unknown address type: %T", addr)
}

// parseSDP extracts the RTP media destination and SSRC from the INVITE SDP body.
// The destination IP comes from the c= line and the port from the m=video line.
// Returns (mediaAddr "ip:port", ssrc, err).
func parseSDP(body string) (string, uint32, error) {
	var mediaIP string
	var mediaPort int
	var ssrc uint32
	var ssrcFound bool

	lines := strings.Split(body, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "c=") {
			// Connection: c=IN IP4 <address>
			parts := strings.Fields(line)
			if len(parts) >= 3 {
				mediaIP = parts[2]
			}
		} else if strings.HasPrefix(line, "m=video ") {
			// Media: m=video <port> RTP/AVP 96
			parts := strings.Fields(line)
			if len(parts) >= 2 {
				port, err := strconv.Atoi(parts[1])
				if err != nil {
					return "", 0, fmt.Errorf("invalid media port in m= line: %s: %w", parts[1], err)
				}
				mediaPort = port
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
	if mediaIP == "" || mediaPort == 0 {
		return "", 0, fmt.Errorf("SDP missing c= media IP or m=video media port")
	}

	return net.JoinHostPort(mediaIP, strconv.Itoa(mediaPort)), ssrc, nil
}

// buildDeviceSDP builds the device SDP answer for INVITE 200 OK.
// For TCP transport it emits TCP/RTP/AVP with a=setup:active (device actively
// connects to the platform media port) and a=connection:new per GB/T 28181.
func buildDeviceSDP(deviceID, localIP string, mediaPort int, ssrc uint32, transport string) string {
	if transport == "tcp" {
		// TCP/RTP/AVP per GB/T 28181 with $-framing
		return fmt.Sprintf(`v=0
o=%s 0 0 IN IP4 %s
s=Play
c=IN IP4 %s
t=0 0
m=video %d TCP/RTP/AVP 0
a=setup:active
a=connection:new
a=sendonly
a=rtpmap:96 PS/90000
y=%d`,
			deviceID, localIP, localIP, mediaPort, ssrc)
	}
	// UDP RTP/AVP (default)
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

// regResponseSource supplies REGISTER-flow responses: either direct socket
// reads (initial registration, before the recv loop exists) or the
// regRespCh channel fed by the recv loop (all later re-registrations —
// prevents the recv loop and the register flow from racing on sipConn).
type regResponseSource func(timeout time.Duration) (*SipMessage, error)

// socketResponseSource reads responses directly from the SIP socket
// (only safe before the recv loop starts).
func (s *Server) socketResponseSource() regResponseSource {
	return func(timeout time.Duration) (*SipMessage, error) {
		buf := make([]byte, 4096)
		_ = s.sipConn.SetReadDeadline(time.Now().Add(timeout))
		n, _, err := s.sipConn.ReadFromUDP(buf)
		_ = s.sipConn.SetReadDeadline(time.Time{})
		if err != nil {
			return nil, err
		}
		resp, err := Parse(buf[:n])
		if err != nil {
			return nil, err
		}
		return &resp, nil
	}
}

// channelResponseSource reads responses routed by the recv loop.
func (s *Server) channelResponseSource() regResponseSource {
	return func(timeout time.Duration) (*SipMessage, error) {
		select {
		case resp := <-s.regRespCh:
			return &resp, nil
		case <-time.After(timeout):
			return nil, fmt.Errorf("timeout waiting for REGISTER response")
		}
	}
}

// reregister runs the REGISTER lifecycle with responses routed through the
// recv loop, serialized so concurrent triggers don't interleave.
func (s *Server) reregister(ctx context.Context) error {
	s.regMu.Lock()
	defer s.regMu.Unlock()
	return s.runRegisterLifecycleWith(ctx, s.channelResponseSource())
}

// signalReRegister nudges the lifecycle goroutine (non-blocking).
func (s *Server) signalReRegister() {
	select {
	case s.reRegisterCh <- struct{}{}:
	default:
	}
}

// handleResponse processes a SIP response received by the recv loop.
func (s *Server) handleResponse(msg SipMessage) {
	// Route REGISTER responses to any active registration flow.
	if strings.Contains(msg.CSeq, "REGISTER") {
		select {
		case s.regRespCh <- msg:
		default:
		}
		return
	}

	switch msg.StatusCode {
	case 200:
		s.keepaliveFailures.Store(0)
	case 401, 403, 404, 407:
		// Platform no longer recognizes us (e.g. it restarted and lost
		// registration state) — re-register immediately. This is the
		// self-heal path for keepalive rejection.
		failures := s.keepaliveFailures.Add(1)
		slog.Warn("gb28181: request rejected by platform", "status", msg.StatusCode, "failures", failures)
		s.signalReRegister()
	default:
		slog.Debug("gb28181: unhandled SIP response", "status", msg.StatusCode)
	}
}

// runRegisterLifecycle performs the initial REGISTER authentication flow,
// reading responses directly from the socket (the recv loop is not yet
// running at this point).
func (s *Server) runRegisterLifecycle(ctx context.Context) error {
	return s.runRegisterLifecycleWith(ctx, s.socketResponseSource())
}

// runRegisterLifecycleWith performs the REGISTER authentication flow
// using the given response source.
func (s *Server) runRegisterLifecycleWith(ctx context.Context, nextResponse regResponseSource) error {
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

	// Wait for response (5s)
	resp, err := nextResponse(5 * time.Second)
	if err != nil {
		return fmt.Errorf("reading REGISTER response: %w", err)
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
		via2 := fmt.Sprintf("SIP/2.0/UDP %s:%d;branch=z9hG4bK%016x", localIPAddr, s.cfg.LocalSIPPort, time.Now().UnixNano())
		authMsg.Via = via2

		if _, err := s.sipConn.WriteToUDP(authMsg.Serialize(), platformAddr); err != nil {
			return fmt.Errorf("sending authenticated REGISTER: %w", err)
		}

		// Wait for 200 OK
		resp, err = nextResponse(5 * time.Second)
		if err != nil {
			return fmt.Errorf("reading 200 OK response: %w", err)
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
func (s *Server) handleInvite(ctx context.Context, msg SipMessage, fromAddr net.Addr) {
	slog.Info("gb28181: received INVITE", "from", fromAddr.String())

	// Parse SDP for RTP destination and SSRC — the media address comes from the
	// SDP c=/m= lines, NEVER from the SIP peer address (streaming to the SIP
	// port floods the platform's signaling socket).
	mediaAddr, ssrc, err := parseSDP(msg.Body)
	if err != nil {
		slog.Warn("gb28181: failed to parse INVITE SDP", "error", err)
		return
	}
	rtpDest, err := net.ResolveUDPAddr("udp", mediaAddr)
	if err != nil {
		slog.Warn("gb28181: invalid RTP destination from INVITE SDP", "addr", mediaAddr, "error", err)
		return
	}
	slog.Info("gb28181: parsed SSRC and RTP destination from INVITE", "ssrc", ssrc, "rtp_dest", rtpDest.String())

	// Tear down any previous media session before starting a new one.
	// Repeated INVITEs (NVR re-register auto-INVITE, NVR restart) would
	// otherwise leak a media goroutine + socket per INVITE, each continuing
	// to push a parallel RTP stream.
	s.mu.Lock()
	if s.mediaCancel != nil {
		s.mediaCancel()
		s.mediaCancel = nil
	}
	if s.sub != nil {
		s.hub.Unsubscribe(s.sub.ID)
		s.sub = nil
	}
	if s.mediaConn != nil {
		s.mediaConn.Close()
		s.mediaConn = nil
	}
	if s.mediaTCPConn != nil {
		s.mediaTCPConn.Close()
		s.mediaTCPConn = nil
	}
	s.mu.Unlock()

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

	// Build device SDP answer — use the real source IP toward the platform,
	// not an interface scan (wrong on multihomed hosts).
	localIPAddr, err := getLocalIP(fromAddr.String())
	if err != nil {
		slog.Warn("gb28181: failed to determine local IP, falling back to interface scan", "error", err)
		localIPAddr = localIP()
	}
	deviceSDP := buildDeviceSDP(s.cfg.DeviceID, localIPAddr, localMediaPort, ssrc, s.cfg.Transport)

	// Send 200 OK with SDP answer
	ok200 := Build200OK(msg, "application/sdp", deviceSDP)
	if err := s.sendSIP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK", "error", err)
		return
	}

	// For TCP transport, actively connect to the platform's media port
	// (device connects TO platform - active mode per GB/T 28181).
	var mediaTCPConn *net.TCPConn
	if s.cfg.Transport == "tcp" {
		conn, err := net.Dial("tcp", mediaAddr)
		if err != nil {
			slog.Warn("gb28181: failed to connect to TCP media port", "addr", mediaAddr, "error", err)
			return
		}
		mediaTCPConn = conn.(*net.TCPConn)
		slog.Info("gb28181: connected to TCP media port", "addr", mediaAddr)
		s.mu.Lock()
		s.mediaTCPConn = mediaTCPConn
		s.mu.Unlock()
	}

	// Subscribe to AUHub
	sub := s.hub.Subscribe(ctx)
	s.mu.Lock()
	s.sub = sub
	s.remoteRTPAddr = rtpDest
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
		pusher := NewRtpPusher(mediaConn, rtpDest)
		if mediaTCPConn != nil {
			pusher.SetTCPConn(mediaTCPConn)
		}
		slog.Info("gb28181: media goroutine started", "remote", rtpDest.String(), "transport", s.cfg.Transport)

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
func (s *Server) handleBye(ctx context.Context, msg SipMessage, fromAddr net.Addr) {
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
	if s.mediaTCPConn != nil {
		s.mediaTCPConn.Close()
		s.mediaTCPConn = nil
	}
	if s.mediaCancel != nil {
		s.mediaCancel()
		s.mediaCancel = nil
	}
	s.remoteRTPAddr = nil
	s.mu.Unlock()

	// Send 200 OK to BYE
	ok200 := Build200OK(msg, "", "")
	if err := s.sendSIP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK to BYE", "error", err)
	}
	slog.Info("gb28181: sent 200 OK to BYE")
}

// handleMessage handles MESSAGE requests - dispatch MANSCDP XML, send 200 OK, and any queued response.
func (s *Server) handleMessage(ctx context.Context, msg SipMessage, fromAddr net.Addr) {
	ok200, queuedResp, err := DispatchInboundMessage(msg, s.devCtx)
	if err != nil {
		slog.Warn("gb28181: failed to dispatch MESSAGE", "error", err)
		return
	}

	// Send 200 OK
	if err := s.sendSIP(ok200.Serialize(), fromAddr); err != nil {
		slog.Warn("gb28181: failed to send 200 OK to MESSAGE", "error", err)
	}

	// Send queued response if any
	if queuedResp != nil {
		// Fresh routing headers for this new MESSAGE request (the queued body
		// only carries MANSCDP XML — Via/CSeq/Max-Forwards are mandatory).
		queuedResp.RequestURI = fmt.Sprintf("sip:%s@%s", s.cfg.SIPDomain, s.cfg.SIPDomain)
		queuedResp.From = fmt.Sprintf("<sip:%s@%s>", s.cfg.DeviceID, s.cfg.SIPDomain)
		queuedResp.To = fmt.Sprintf("<sip:%s@%s>", s.cfg.SIPDomain, s.cfg.SIPDomain)
		queuedResp.CallID = fmt.Sprintf("%d-resp@%s", time.Now().Unix(), s.cfg.DeviceID)
		queuedResp.Via = fmt.Sprintf("SIP/2.0/UDP %s:%d;rport;branch=z9hG4bK%016x", localIP(), s.cfg.LocalSIPPort, time.Now().UnixNano())
		queuedResp.MaxForwards = "70"
		queuedResp.CSeq = "2 MESSAGE"
		if err := s.sendSIP(queuedResp.Serialize(), fromAddr); err != nil {
			slog.Warn("gb28181: failed to send queued MESSAGE response", "error", err)
		}
	}
}
