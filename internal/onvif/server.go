package onvif

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"strings"
	"time"
)

// ActionHandler handles a specific ONVIF SOAP action.
type ActionHandler func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error)

// soapResponseEnvelope wraps a response payload in a SOAP envelope.
type soapResponseEnvelope struct {
	XMLName xml.Name    `xml:"http://www.w3.org/2003/05/soap-envelope Envelope"`
	Header  SOAPHeader  `xml:"Header"`
	Body    interface{} `xml:"Body"`
}

// soapResponseBody wraps the action response element.
type soapResponseBody struct {
	Response interface{} `xml:",any"`
}

type Server struct {
	httpServer      *http.Server
	auth            *Auth
	actions         map[string]ActionHandler
	config          ConfigProvider
	discoveryHandler http.Handler // handles WS-Discovery HTTP probes
}

// ConfigProvider provides auth and media configuration. Kept as interface for testability.
type ConfigProvider interface {
	ONVIFUsername() string
	ONVIFPassword() string
	ONVIFPort() int
	RTSPPort() int
	DeviceIP() string
	CameraWidth() int
	CameraHeight() int
	CameraFPS() int
	CameraBitrate() int
}

// New creates a new ONVIF server.
func New(cfg ConfigProvider) *Server {
	return &Server{
		auth:    &Auth{Username: cfg.ONVIFUsername(), Password: cfg.ONVIFPassword()},
		actions: make(map[string]ActionHandler),
		config:  cfg,
	}
}

// RegisterAction registers a SOAP action handler.
func (s *Server) RegisterAction(action string, handler ActionHandler) {
	s.actions[action] = handler
}

// SetDiscoveryHandler sets the handler for WS-Discovery HTTP probe requests.
func (s *Server) SetDiscoveryHandler(h http.Handler) {
	s.discoveryHandler = h
}

// parseSOAPRequest parses a raw SOAP request body and extracts the action name
// from the first child element of the SOAP Body.
func parseSOAPRequest(data []byte) (action string, bodyContent []byte, err error) {
	var envelope SOAPEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return "", nil, fmt.Errorf("parsing SOAP envelope: %w", err)
	}

	trimmed := strings.TrimSpace(envelope.Body.RawXML)
	if trimmed == "" {
		return "", nil, nil
	}

	// Extract action name from first element in body
	decoder := xml.NewDecoder(strings.NewReader(trimmed))
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return "", nil, fmt.Errorf("parsing SOAP body: %w", err)
		}
		if se, ok := tok.(xml.StartElement); ok {
			return se.Name.Local, []byte(trimmed), nil
		}
	}

	return "", nil, nil
}

// parseAndAuth parses a SOAP request, authenticates, and extracts the action name.
// Unlike the old version, it accepts pre-read data bytes and returns the authResult
// explicitly — the caller decides whether auth is required for the action.
// Auth failures do NOT return an error; the authResult indicates success/failure.
func (s *Server) parseAndAuth(data []byte) (action string, bodyContent []byte, authResult *AuthResult, err error) {
	var envelope SOAPEnvelope
	if err := xml.Unmarshal(data, &envelope); err != nil {
		return "", nil, &AuthResult{}, fmt.Errorf("parsing SOAP envelope: %w", err)
	}

	// Authenticate if credentials are provided
	authResult = &AuthResult{}
	if envelope.Header.Security != nil && envelope.Header.Security.UsernameToken != nil {
		token := envelope.Header.Security.UsernameToken
		authResult.Username = token.Username
		if authErr := s.auth.Validate(token); authErr != nil {
			authResult.OK = false // credentials provided but invalid
		} else {
			authResult.OK = true // valid credentials
		}
	}
	// If no credentials provided, authResult.OK stays false

	// Extract action from body
	action, bodyContent, err = parseSOAPRequest(data)
	if err != nil {
		return "", nil, authResult, err
	}

	return action, bodyContent, authResult, nil
}

// writeSOAPResponse wraps response data in a SOAP envelope and writes it.
func writeSOAPResponse(w http.ResponseWriter, data interface{}) error {
	body := soapResponseBody{Response: data}
	env := soapResponseEnvelope{
		Header: SOAPHeader{},
		Body:   body,
	}

	output, err := xml.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling SOAP response: %w", err)
	}

	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	if _, err := w.Write(output); err != nil {
		return fmt.Errorf("writing response: %w", err)
	}
	return nil
}

// writeSOAPFault returns a SOAP 1.2 fault response with the given HTTP status code.
func writeSOAPFault(w http.ResponseWriter, faultCode, reason string, httpStatus int) error {
	fault := SOAPFault{
		Code: SOAPFaultCode{Value: faultCode},
		Reason: SOAPFaultReason{Text: reason},
	}
	env := soapFaultEnvelope{
		Header: SOAPHeader{},
		Body:   soapFaultBody{Fault: fault},
	}

	output, err := xml.MarshalIndent(env, "", "  ")
	if err != nil {
		return fmt.Errorf("marshalling SOAP fault: %w", err)
	}

	w.Header().Set("Content-Type", "application/soap+xml; charset=utf-8")
	w.WriteHeader(httpStatus)
	if _, err := w.Write(output); err != nil {
		return fmt.Errorf("writing fault response: %w", err)
	}
	return nil
}

// isAuthRequired returns true if the ONVIF action requires authentication.
// Write operations (Set, Remove, Create, Go prefix, plus explicit PTZ moves and Stop)
// require valid WS-UsernameToken credentials. Read operations are open.
func isAuthRequired(action string) bool {
	switch action {
	case "ContinuousMove", "AbsoluteMove", "RelativeMove", "Stop":
		return true
	}
	if strings.HasPrefix(action, "Set") || strings.HasPrefix(action, "Remove") ||
		strings.HasPrefix(action, "Create") || strings.HasPrefix(action, "Go") {
		return true
	}
	return false
}

	func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		writeSOAPFault(w, "soap:Sender", fmt.Sprintf("unsupported method: %s", r.Method), http.StatusInternalServerError)
		return
	}

	// Read body once for both discovery check and SOAP processing
	data, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		writeSOAPFault(w, "soap:Sender", "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(data))

	// Parse SOAP and authenticate
	action, bodyContent, authResult, err := s.parseAndAuth(data)
	if err != nil {
		writeSOAPFault(w, "soap:Client", err.Error(), http.StatusBadRequest)
		return
	}

	// WS-Discovery probe interception — check action name after SOAP parsing
	if s.discoveryHandler != nil && action == "Probe" {
		s.discoveryHandler.ServeHTTP(w, r)
		return
	}

	if action == "" {
		writeSOAPFault(w, "soap:Client", "no action found in SOAP body", http.StatusBadRequest)
		return
	}

	handler, ok := s.actions[action]
	if !ok {
		writeSOAPFault(w, "soap:Sender", fmt.Sprintf("unsupported action: %s", action), http.StatusBadRequest)
		return
	}

	// Auth check: write operations require valid credentials
	if isAuthRequired(action) && !authResult.OK {
		writeSOAPFault(w, "soap:Sender", fmt.Sprintf("authentication required for action: %s", action), http.StatusUnauthorized)
		return
	}

	result, err := handler(r.Context(), bodyContent, authResult)
	if err != nil {
		writeSOAPFault(w, "soap:Receiver", err.Error(), http.StatusInternalServerError)
		return
	}

	if err := writeSOAPResponse(w, result); err != nil {
		log.Printf("onvif: failed to write response for %s: %v", action, err)
	}
}

// Start starts the ONVIF HTTP server.
func (s *Server) Start(ctx context.Context) error {
	addr := fmt.Sprintf(":%d", s.config.ONVIFPort())
	s.httpServer = &http.Server{
		Addr:              addr,
		Handler:           s,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// ConnContext fires for every accepted connection. We use it to
		// capture the local IP of the RPi interface that accepted the
		// connection — this is what the NVR needs in XAddr/RTSP URIs to
		// reach us back, regardless of which interface the NVR used.
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return WithServerIP(ctx, ServerIPFromConn(c))
		},
	}
	errCh := make(chan error, 1)
	go func() {
		log.Printf("onvif: server starting on %s", addr)
		if err := s.httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case <-ctx.Done():
		return s.Stop()
	case err := <-errCh:
		return err
	}
}

// Stop stops the server gracefully.
func (s *Server) Stop() error {
	if s.httpServer == nil {
		return nil
	}
	return s.httpServer.Close()
}

// MarshalSOAP helper for tests: marshals data into a SOAP envelope bytes.
func MarshalSOAP(data interface{}) ([]byte, error) {
	body := soapResponseBody{Response: data}
	env := soapResponseEnvelope{
		Header: SOAPHeader{},
		Body:   body,
	}
	var buf bytes.Buffer
	buf.WriteString(xml.Header)
	enc := xml.NewEncoder(&buf)
	enc.Indent("", "  ")
	if err := enc.Encode(env); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
