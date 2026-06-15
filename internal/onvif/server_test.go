package onvif

import (
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPasswordText(t *testing.T) {
	auth := &Auth{Username: "admin", Password: "testpass"}

	t.Run("correct password", func(t *testing.T) {
		token := &UsernameToken{
			Username: "admin",
			Password: "testpass",
		}
		if err := auth.Validate(token); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		token := &UsernameToken{
			Username: "admin",
			Password: "wrongpassword",
		}
		if err := auth.Validate(token); err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("wrong username", func(t *testing.T) {
		token := &UsernameToken{
			Username: "wronguser",
			Password: "testpass",
		}
		if err := auth.Validate(token); err == nil {
			t.Fatal("expected error, got nil")
		}
	})
}

func TestPasswordDigest(t *testing.T) {
	password := "testpass"
	nonce := "bm9uY2UxMjM0NTY=" // base64("nonce123456")
	created := "2024-01-01T00:00:00.000Z"

	expected := CheckDigest(nonce, created, password)

	// Verify the digest is correct by manual computation
	nonceBytes, _ := base64.StdEncoding.DecodeString(nonce)
	h := sha1.New()
	h.Write(nonceBytes)
	h.Write([]byte(created))
	h.Write([]byte(password))
	manualDigest := base64.StdEncoding.EncodeToString(h.Sum(nil))

	if expected != manualDigest {
		t.Fatalf("CheckDigest mismatch: got %s, want %s", expected, manualDigest)
	}

	auth := &Auth{Username: "admin", Password: password}

	t.Run("correct digest", func(t *testing.T) {
		token := &UsernameToken{
			Username: "admin",
			Password: expected,
			Nonce:    nonce,
			Created:  created,
		}
		if err := auth.Validate(token); err != nil {
			t.Fatalf("expected no error, got: %v", err)
		}
	})

	t.Run("wrong password", func(t *testing.T) {
		// Compute digest with wrong password
		wrongDigest := CheckDigest(nonce, created, "wrongpassword")
		token := &UsernameToken{
			Username: "admin",
			Password: wrongDigest,
			Nonce:    nonce,
			Created:  created,
		}
		if err := auth.Validate(token); err == nil {
			t.Fatal("expected error for wrong password digest, got nil")
		}
	})
}

func TestPasswordDigestWrongPassword(t *testing.T) {
	auth := &Auth{Username: "admin", Password: "testpass"}

	nonce := "bm9uY2UxMjM0NTY="
	created := "2024-01-01T00:00:00.000Z"
	wrongDigest := CheckDigest(nonce, created, "totallywrong")

	token := &UsernameToken{
		Username: "admin",
		Password: wrongDigest,
		Nonce:    nonce,
		Created:  created,
	}

	if err := auth.Validate(token); err == nil {
		t.Fatal("expected error for wrong password, got nil")
	}
}

func TestEmptyToken(t *testing.T) {
	auth := &Auth{Username: "admin", Password: "testpass"}

	t.Run("nil token", func(t *testing.T) {
		if err := auth.Validate(nil); err != ErrMissingToken {
			t.Fatalf("expected ErrMissingToken, got: %v", err)
		}
	})

	t.Run("empty username", func(t *testing.T) {
		token := &UsernameToken{
			Username: "",
			Password: "testpass",
		}
		if err := auth.Validate(token); err != ErrEmptyUsername {
			t.Fatalf("expected ErrEmptyUsername, got: %v", err)
		}
	})
}

func TestCheckDigest(t *testing.T) {
	// Known test vector
	nonce := "dGVzdA==" // base64("test")
	created := "2024-01-01T00:00:00Z"
	password := "secret"

	nonceBytes, _ := base64.StdEncoding.DecodeString(nonce)
	h := sha1.New()
	h.Write(nonceBytes)
	h.Write([]byte(created))
	h.Write([]byte(password))
	expected := base64.StdEncoding.EncodeToString(h.Sum(nil))

	result := CheckDigest(nonce, created, password)
	if result != expected {
		t.Fatalf("CheckDigest: got %s, want %s", result, expected)
	}
}

func TestCheckDigestEmptyNonce(t *testing.T) {
	// Empty nonce should still produce deterministic output
	result := CheckDigest("", "2024-01-01T00:00:00Z", "secret")
	if result == "" {
		t.Fatal("expected non-empty digest for empty nonce")
	}
}

// mockConfig implements ConfigProvider for tests.
type mockConfig struct {
	username string
	password string
	port     int
}

func (m *mockConfig) ONVIFUsername() string { return m.username }
func (m *mockConfig) ONVIFPassword() string { return m.password }
func (m *mockConfig) ONVIFPort() int        { return m.port }
func (m *mockConfig) RTSPPort() int        { return 8554 }
func (m *mockConfig) DeviceIP() string       { return "192.168.1.100" }
func (m *mockConfig) CameraWidth() int      { return 1280 }
func (m *mockConfig) CameraHeight() int     { return 720 }
func (m *mockConfig) CameraFPS() int         { return 15 }
func (m *mockConfig) CameraBitrate() int    { return 2_000_000 }

func TestParseSOAP(t *testing.T) {
	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	action, bodyContent, err := parseSOAPRequest([]byte(soapReq))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "GetDeviceInformation" {
		t.Fatalf("expected action GetDeviceInformation, got %s", action)
	}
	if bodyContent == nil {
		t.Fatal("expected non-nil body content")
	}
	if !strings.Contains(string(bodyContent), "GetDeviceInformation") {
		t.Fatalf("body content missing action element: %s", string(bodyContent))
	}
}

func TestParseSOAPWithAuth(t *testing.T) {
	nonce := "dGVzdA=="
	created := "2024-01-01T00:00:00Z"
	digest := CheckDigest(nonce, created, "testpass")

	soapReq := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:wsse="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-secext-1.0.xsd"
            xmlns:wsu="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-wssecurity-utility-1.0.xsd">
  <s:Header>
    <wsse:Security>
      <wsse:UsernameToken>
        <wsse:Username>admin</wsse:Username>
        <wsse:Password Type="http://docs.oasis-open.org/wss/2004/01/oasis-200401-wss-username-token-profile-1.0#PasswordDigest">%s</wsse:Password>
        <wsse:Nonce>%s</wsse:Nonce>
        <wsu:Created>%s</wsu:Created>
      </wsse:UsernameToken>
    </wsse:Security>
  </s:Header>
  <s:Body>
    <tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`, digest, nonce, created)

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)

	data := []byte(soapReq)
	action, bodyContent, authResult, err := srv.parseAndAuth(data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !authResult.OK {
		t.Fatalf("expected auth OK, got false")
	}
	if authResult.Username != "admin" {
		t.Fatalf("expected username admin, got %s", authResult.Username)
	}
	if action != "GetDeviceInformation" {
		t.Fatalf("expected GetDeviceInformation, got %s", action)
	}
	if bodyContent == nil {
		t.Fatal("expected non-nil body content")
	}
	if !strings.Contains(string(bodyContent), "GetDeviceInformation") {
		t.Fatalf("body content missing action element: %s", string(bodyContent))
	}
}


func TestParseInvalidXML(t *testing.T) {
	_, _, err := parseSOAPRequest([]byte("not xml at all <><<<"))
	if err == nil {
		t.Fatal("expected error for invalid XML, got nil")
	}
}

func TestParseEmptyBody(t *testing.T) {
	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
  </s:Body>
</s:Envelope>`

	action, bodyContent, err := parseSOAPRequest([]byte(soapReq))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != "" {
		t.Fatalf("expected empty action, got %s", action)
	}
	if bodyContent != nil {
		t.Fatalf("expected nil body content for empty body, got %s", string(bodyContent))
	}
}

func TestWriteSOAPFault(t *testing.T) {
	w := httptest.NewRecorder()
	err := writeSOAPFault(w, "soap:Sender", "test error", http.StatusInternalServerError)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected status 500, got %d", w.Code)
	}

	var env soapFaultEnvelope
	if err := xml.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse fault response: %v", err)
	}
	if env.Body.Fault.Code.Value != "soap:Sender" {
		t.Fatalf("expected fault code soap:Sender, got %s", env.Body.Fault.Code.Value)
	}
	if env.Body.Fault.Reason.Text != "test error" {
		t.Fatalf("expected fault reason 'test error', got %s", env.Body.Fault.Reason.Text)
	}
}

func TestWriteSOAPResponse(t *testing.T) {
	type TestResponse struct {
		XMLName xml.Name `xml:"tds:GetDeviceInformationResponse"`
		Message string   `xml:"Message"`
	}

	w := httptest.NewRecorder()
	data := &TestResponse{Message: "hello"}
	err := writeSOAPResponse(w, data)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if w.Code != http.StatusOK {
		t.Fatalf("expected status 200, got %d", w.Code)
	}

	body := w.Body.String()
	if !strings.Contains(body, "Envelope") {
		t.Fatalf("response missing SOAP envelope: %s", body)
	}
	if !strings.Contains(body, "GetDeviceInformationResponse") {
		t.Fatalf("response missing action element: %s", body)
	}
}

func TestServeHTTPUnsupportedMethod(t *testing.T) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)

	req := httptest.NewRequest(http.MethodGet, "/onvif/device_service", nil)
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for unsupported method, got %d", w.Code)
	}
}

func TestServeHTTPUnknownAction(t *testing.T) {
	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:UnknownAction xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for unknown action, got %d", w.Code)
	}
}

func TestServeHTTPWithHandler(t *testing.T) {
	type EchoResponse struct {
		XMLName xml.Name `xml:"echo:EchoResponse xmlns:echo=\"http://example.com/echo\""`
		Message string   `xml:"Message"`
	}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <echo:EchoRequest xmlns:echo="http://example.com/echo">
      <Message>hello</Message>
    </echo:EchoRequest>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)

	srv.RegisterAction("EchoRequest", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return &EchoResponse{Message: "hello"}, nil
	})

	req := httptest.NewRequest(http.MethodPost, "/echo", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "EchoResponse") {
		t.Fatalf("response missing EchoResponse: %s", body)
	}
}
