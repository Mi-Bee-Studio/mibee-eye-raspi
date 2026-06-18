package onvif

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ptz"
)

// ptzTestServer creates a Server with PTZ handlers and ptz state for testing.
func ptzTestServer() (*Server, *ptz.State) {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	state := ptz.NewState()
	RegisterPTZHandlers(srv, state)
	return srv, state
}

// sendPTZRequest sends a SOAP PTZ request and returns the response recorder.
func sendPTZRequest(srv *Server, soapBody string) *httptest.ResponseRecorder {
	soapReq := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
 xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"
 xmlns:tt="http://www.onvif.org/ver10/schema">
  <s:Body>
    %s
  </s:Body>
</s:Envelope>`, soapBody)

	req := httptest.NewRequest(http.MethodPost, "/onvif/ptz_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

// sendPTZRequestWithAuth sends a SOAP PTZ request with WS-UsernameToken digest auth.
func sendPTZRequestWithAuth(srv *Server, soapBody string) *httptest.ResponseRecorder {
	nonce := "dGVzdA=="
	created := "2024-01-01T00:00:00.000Z"
	digest := CheckDigest(nonce, created, "testpass")

	soapReq := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
 xmlns:tptz="http://www.onvif.org/ver20/ptz/wsdl"
 xmlns:tt="http://www.onvif.org/ver10/schema"
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
    %s
  </s:Body>
</s:Envelope>`, digest, nonce, created, soapBody)

	req := httptest.NewRequest(http.MethodPost, "/onvif/ptz_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)
	return w
}

func TestPTZContinuousMove(t *testing.T) {
	srv, state := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:ContinuousMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Velocity>
			<tt:PanTilt x="0.5" y="0"/>
			<tt:Zoom x="0"/>
		</tptz:Velocity>
	</tptz:ContinuousMove>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Verify it's moving
	time.Sleep(100 * time.Millisecond)
	status := state.GetStatus()
	if status != "MOVING" {
		t.Errorf("expected MOVING, got %s", status)
	}

	state.Stop()
}

func TestPTZContinuousMoveExtractsVelocity(t *testing.T) {
	srv, state := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:ContinuousMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Velocity>
			<tt:PanTilt x="1.0" y="0.5"/>
			<tt:Zoom x="0.8"/>
		</tptz:Velocity>
	</tptz:ContinuousMove>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Let it move a bit
	time.Sleep(150 * time.Millisecond)
	state.Stop()

	pos := state.GetPosition()
	if pos.Pan <= 0 {
		t.Errorf("expected Pan > 0 after continuous move, got %f", pos.Pan)
	}
	if pos.Tilt <= 0 {
		t.Errorf("expected Tilt > 0 after continuous move, got %f", pos.Tilt)
	}
	if pos.Zoom <= 0 {
		t.Errorf("expected Zoom > 0 after continuous move, got %f", pos.Zoom)
	}
}

func TestPTZStop(t *testing.T) {
	srv, state := ptzTestServer()

	// Start moving first
	state.ContinuousMove(ptz.Velocity{Pan: 1.0, Tilt: 0, Zoom: 0})
	time.Sleep(50 * time.Millisecond)

	if state.GetStatus() != "MOVING" {
		t.Fatalf("expected MOVING before stop")
	}

	// Send Stop via SOAP
	w := sendPTZRequestWithAuth(srv, `<tptz:Stop>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PanTilt>true</tptz:PanTilt>
		<tptz:Zoom>true</tptz:Zoom>
	</tptz:Stop>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	time.Sleep(20 * time.Millisecond)

	if state.GetStatus() != "IDLE" {
		t.Errorf("expected IDLE after SOAP Stop, got %s", state.GetStatus())
	}
}

func TestPTZAbsoluteMove(t *testing.T) {
	srv, state := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:AbsoluteMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Position>
			<tt:PanTilt x="0.5" y="-0.3"/>
			<tt:Zoom x="0.8"/>
		</tptz:Position>
	</tptz:AbsoluteMove>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Wait for easing to complete
	time.Sleep(2 * time.Second)

	pos := state.GetPosition()
	if pos.Pan != 0.5 {
		t.Errorf("expected Pan 0.5, got %f", pos.Pan)
	}
	if pos.Tilt != -0.3 {
		t.Errorf("expected Tilt -0.3, got %f", pos.Tilt)
	}
	if pos.Zoom != 0.8 {
		t.Errorf("expected Zoom 0.8, got %f", pos.Zoom)
	}
}

func TestPTZRelativeMove(t *testing.T) {
	srv, state := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:RelativeMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Translation>
			<tt:PanTilt x="0.3" y="0.2"/>
			<tt:Zoom x="0.1"/>
		</tptz:Translation>
	</tptz:RelativeMove>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	pos := state.GetPosition()
	if pos.Pan != 0.3 {
		t.Errorf("expected Pan 0.3, got %f", pos.Pan)
	}
	if pos.Tilt != 0.2 {
		t.Errorf("expected Tilt 0.2, got %f", pos.Tilt)
	}
	if pos.Zoom != 0.1 {
		t.Errorf("expected Zoom 0.1, got %f", pos.Zoom)
	}
}

func TestPTZGetStatus(t *testing.T) {
	srv, state := ptzTestServer()

	// Set position
	state.AbsoluteMove(ptz.Position{Pan: 0.6, Tilt: -0.4, Zoom: 0.7})
	time.Sleep(2 * time.Second)

	w := sendPTZRequest(srv, `<tptz:GetStatus>
		<tptz:ProfileToken>main</tptz:ProfileToken>
	</tptz:GetStatus>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	// Verify response contains expected elements
	if !strings.Contains(body, "GetStatusResponse") {
		t.Errorf("response missing GetStatusResponse\nBody: %s", body)
	}
	if !strings.Contains(body, "PTZStatus") {
		t.Errorf("response missing PTZStatus\nBody: %s", body)
	}
	if !strings.Contains(body, "IDLE") {
		t.Errorf("response missing IDLE status\nBody: %s", body)
	}
}

func TestPTZGetStatusMoving(t *testing.T) {
	srv, state := ptzTestServer()

	// Start continuous movement
	state.ContinuousMove(ptz.Velocity{Pan: 1.0, Tilt: 0, Zoom: 0})
	time.Sleep(50 * time.Millisecond)

	w := sendPTZRequest(srv, `<tptz:GetStatus>
		<tptz:ProfileToken>main</tptz:ProfileToken>
	</tptz:GetStatus>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "MOVING") {
		t.Errorf("response should contain MOVING status\nBody: %s", body)
	}

	state.Stop()
}

func TestPTZSetPreset(t *testing.T) {
	srv, state := ptzTestServer()

	// Set a known position first
	state.AbsoluteMove(ptz.Position{Pan: 0.5, Tilt: -0.3, Zoom: 0.8})
	time.Sleep(2 * time.Second)

	w := sendPTZRequestWithAuth(srv, `<tptz:SetPreset>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PresetToken>home</tptz:PresetToken>
	</tptz:SetPreset>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "SetPresetResponse") {
		t.Errorf("response missing SetPresetResponse\nBody: %s", body)
	}
	if !strings.Contains(body, "home") {
		t.Errorf("response missing preset token\nBody: %s", body)
	}

	// Verify preset was stored
	if len(state.GetPresets()) != 1 {
		t.Errorf("expected 1 preset, got %d", len(state.GetPresets()))
	}
}

func TestPTZGetPresets(t *testing.T) {
	srv, state := ptzTestServer()

	state.SetPreset("home", "Home")
	state.SetPreset("zoom1", "Zoom 1")
	state.SetPreset("wide", "Wide")

	w := sendPTZRequest(srv, `<tptz:GetPresets>
		<tptz:ProfileToken>main</tptz:ProfileToken>
	</tptz:GetPresets>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetPresetsResponse") {
		t.Errorf("response missing GetPresetsResponse\nBody: %s", body)
	}
	for _, name := range []string{"home", "zoom1", "wide"} {
		if !strings.Contains(body, name) {
			t.Errorf("response missing preset %q\nBody: %s", name, body)
		}
	}
}

func TestPTZGotoPreset(t *testing.T) {
	srv, state := ptzTestServer()

	// Store preset at known position
	state.SetPreset("home", "Home")

	w := sendPTZRequestWithAuth(srv, `<tptz:GotoPreset>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PresetToken>home</tptz:PresetToken>
	</tptz:GotoPreset>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}
}

func TestPTZGotoPresetNotFound(t *testing.T) {
	srv, _ := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:GotoPreset>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PresetToken>nonexistent</tptz:PresetToken>
	</tptz:GotoPreset>`)

	// Should return SOAP fault
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for nonexistent preset, got %d", w.Code)
	}
}

func TestPTZRemovePreset(t *testing.T) {
	srv, state := ptzTestServer()

	state.SetPreset("temp", "Temporary")

	w := sendPTZRequestWithAuth(srv, `<tptz:RemovePreset>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PresetToken>temp</tptz:PresetToken>
	</tptz:RemovePreset>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	if len(state.GetPresets()) != 0 {
		t.Errorf("expected 0 presets after removal, got %d", len(state.GetPresets()))
	}
}

func TestPTZGetNodes(t *testing.T) {
	srv, _ := ptzTestServer()

	w := sendPTZRequest(srv, `<tptz:GetNodes/>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetNodesResponse") {
		t.Errorf("response missing GetNodesResponse\nBody: %s", body)
	}
	if !strings.Contains(body, "PTZNode") {
		t.Errorf("response missing PTZNode\nBody: %s", body)
	}
	if !strings.Contains(body, "Polygon") {
		t.Errorf("response missing Polygon space\nBody: %s", body)
	}
}

func TestPTZGetConfigurations(t *testing.T) {
	srv, _ := ptzTestServer()

	w := sendPTZRequest(srv, `<tptz:GetConfigurations/>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetConfigurationsResponse") {
		t.Errorf("response missing GetConfigurationsResponse\nBody: %s", body)
	}
	if !strings.Contains(body, "PTZConfiguration") {
		t.Errorf("response missing PTZConfiguration\nBody: %s", body)
	}
}

func TestPTZContinuousMoveDefaultZero(t *testing.T) {
	srv, state := ptzTestServer()

	// ContinuousMove with no velocity elements should not move
	w := sendPTZRequestWithAuth(srv, `<tptz:ContinuousMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Velocity></tptz:Velocity>
	</tptz:ContinuousMove>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	time.Sleep(100 * time.Millisecond)
	pos := state.GetPosition()
	if pos.Pan != 0 || pos.Tilt != 0 || pos.Zoom != 0 {
		t.Errorf("expected no movement with zero velocity, got %+v", pos)
	}
	state.Stop()
}

func TestPTZEndToEndSequence(t *testing.T) {
	srv, state := ptzTestServer()

	// 1. Start continuous move
	w := sendPTZRequestWithAuth(srv, `<tptz:ContinuousMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Velocity>
			<tt:PanTilt x="1.0" y="0"/>
			<tt:Zoom x="0"/>
		</tptz:Velocity>
	</tptz:ContinuousMove>`)
	if w.Code != http.StatusOK {
		t.Fatalf("step 1 failed: %d", w.Code)
	}

	time.Sleep(100 * time.Millisecond)

	// 2. Get status — should be MOVING
	w = sendPTZRequest(srv, `<tptz:GetStatus>
		<tptz:ProfileToken>main</tptz:ProfileToken>
	</tptz:GetStatus>`)
	if w.Code != http.StatusOK {
		t.Fatalf("step 2 failed: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "MOVING") {
		t.Error("step 2: expected MOVING status")
	}

	// 3. Stop
	w = sendPTZRequestWithAuth(srv, `<tptz:Stop>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PanTilt>true</tptz:PanTilt>
		<tptz:Zoom>true</tptz:Zoom>
	</tptz:Stop>`)
	if w.Code != http.StatusOK {
		t.Fatalf("step 3 failed: %d", w.Code)
	}

	time.Sleep(20 * time.Millisecond)

	// 4. Get status — should be IDLE
	w = sendPTZRequest(srv, `<tptz:GetStatus>
		<tptz:ProfileToken>main</tptz:ProfileToken>
	</tptz:GetStatus>`)
	if w.Code != http.StatusOK {
		t.Fatalf("step 4 failed: %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "IDLE") {
		t.Error("step 4: expected IDLE status")
	}

	// 5. Set preset
	w = sendPTZRequestWithAuth(srv, `<tptz:SetPreset>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PresetToken>pos1</tptz:PresetToken>
	</tptz:SetPreset>`)
	if w.Code != http.StatusOK {
		t.Fatalf("step 5 failed: %d", w.Code)
	}

	// 6. Move away
	state.AbsoluteMove(ptz.Position{Pan: 0, Tilt: 0, Zoom: 0})
	time.Sleep(2 * time.Second)

	// 7. Go to preset
	w = sendPTZRequestWithAuth(srv, `<tptz:GotoPreset>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:PresetToken>pos1</tptz:PresetToken>
	</tptz:GotoPreset>`)
	if w.Code != http.StatusOK {
		t.Fatalf("step 7 failed: %d", w.Code)
	}
}


func TestParsePresetToken(t *testing.T) {
	tests := []struct {
		body   string
		expect string
	}{
		{`<tptz:PresetToken>home</tptz:PresetToken>`, "home"},
		{`<PresetToken>zoom1</PresetToken>`, "zoom1"},
		{`<tptz:Other/>`, ""},
	}

	for _, tt := range tests {
		got := parsePresetToken([]byte(tt.body))
		if got != tt.expect {
			t.Errorf("parsePresetToken(%q) = %q, want %q", tt.body, got, tt.expect)
		}
	}
}
func TestPTZContinuousMoveMalformedXML(t *testing.T) {
	srv, _ := ptzTestServer()

	// Non-numeric attribute value causes XML unmarshal to fail
	w := sendPTZRequestWithAuth(srv, `<tptz:ContinuousMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Velocity>
			<tt:PanTilt x="not-a-number" y="0"/>
		</tptz:Velocity>
	</tptz:ContinuousMove>`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for malformed XML, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fault") {
		t.Errorf("expected SOAP Fault, got: %s", w.Body.String())
	}
}

func TestPTZAbsoluteMoveMalformedXML(t *testing.T) {
	srv, _ := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:AbsoluteMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Position>
			<tt:PanTilt x="0.5" y="not-valid"/>
		</tptz:Position>
	</tptz:AbsoluteMove>`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for malformed XML, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fault") {
		t.Errorf("expected SOAP Fault, got: %s", w.Body.String())
	}
}

func TestPTZRelativeMoveMalformedXML(t *testing.T) {
	srv, _ := ptzTestServer()

	w := sendPTZRequestWithAuth(srv, `<tptz:RelativeMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Translation>
			<tt:Zoom x="invalid"/>
		</tptz:Translation>
	</tptz:RelativeMove>`)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500 for malformed XML, got %d. Body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Fault") {
		t.Errorf("expected SOAP Fault, got: %s", w.Body.String())
	}
}

func TestPTZContinuousMoveMissingPanTilt(t *testing.T) {
	srv, state := ptzTestServer()

	// Velocity with only Zoom, no PanTilt
	w := sendPTZRequestWithAuth(srv, `<tptz:ContinuousMove>
		<tptz:ProfileToken>main</tptz:ProfileToken>
		<tptz:Velocity>
			<tt:Zoom x="0.5"/>
		</tptz:Velocity>
	</tptz:ContinuousMove>`)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Wait and check — Pan/Tilt should be 0 (missing element), Zoom > 0
	time.Sleep(100 * time.Millisecond)
	pos := state.GetPosition()
	if pos.Pan != 0 || pos.Tilt != 0 {
		t.Errorf("expected Pan/Tilt 0 for missing PanTilt, got Pan=%f Tilt=%f", pos.Pan, pos.Tilt)
	}
	if pos.Zoom <= 0 {
		t.Errorf("expected Zoom > 0 from Zoom element, got %f", pos.Zoom)
	}
	state.Stop()
}
