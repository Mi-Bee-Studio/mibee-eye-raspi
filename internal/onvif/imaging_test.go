package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"
)

// mockImagingCamera is a test double that records SetParam calls for imaging tests.
type mockImagingCamera struct {
	mu     sync.Mutex
	params map[string]interface{}
}

func newMockImagingCamera() *mockImagingCamera {
	return &mockImagingCamera{
		params: map[string]interface{}{
			"brightness": float64(0.0),
			"contrast":   float64(1.0),
			"saturation": float64(1.0),
			"sharpness":  float64(1.0),
			"exposure":   float64(0),
		},
	}
}

func (m *mockImagingCamera) Start(_ context.Context) error { return nil }
func (m *mockImagingCamera) Stop() error               { return nil }
func (m *mockImagingCamera) Frames() <-chan camera.Frame { return nil }

func (m *mockImagingCamera) SetParam(name string, value interface{}) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.params[name] = value
	return nil
}

func (m *mockImagingCamera) GetParam(name string) (interface{}, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	v, ok := m.params[name]
	if !ok {
		return 0, nil
	}
	return v, nil
}

func (m *mockImagingCamera) Info() camera.CameraInfo {
	return camera.CameraInfo{}
}

func (m *mockImagingCamera) getParam(name string) interface{} {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.params[name]
}

// wrapWithAuth wraps a SOAP body envelope with WS-UsernameToken digest auth.
// Used by HTTP-level tests for write operations (Set*, Remove*, etc.) that require auth.
func wrapWithAuth(soapBody string) string {
	nonce := "dGVzdA=="
	created := "2024-01-01T00:00:00.000Z"
	digest := CheckDigest(nonce, created, "testpass")
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
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
    %s
  </s:Body>
</s:Envelope>`, digest, nonce, created, soapBody)
}

// newImagingTestServer creates a Server with imaging handlers registered.
func newImagingTestServer(mock *mockImagingCamera) *Server {
	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	pm := camera.NewParamManager(mock)
	RegisterImagingHandlers(srv, pm)
	return srv
}

// ---------------------------------------------------------------------------
// Unit tests for response builders
// ---------------------------------------------------------------------------

func TestGetImagingSettings(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	resp, err := handleGetImagingSettings(pm)
	if err != nil {
		t.Fatalf("handleGetImagingSettings failed: %v", err)
	}

	s := resp.Settings

	if s.Brightness == nil || s.Brightness.Value != 0.0 {
		t.Errorf("expected Brightness=0.0, got %v", s.Brightness)
	}
	if s.Contrast == nil || s.Contrast.Value != 1.0 {
		t.Errorf("expected Contrast=1.0, got %v", s.Contrast)
	}
	if s.Saturation == nil || s.Saturation.Value != 1.0 {
		t.Errorf("expected Saturation=1.0, got %v", s.Saturation)
	}
	if s.Sharpness == nil || s.Sharpness.Value != 1.0 {
		t.Errorf("expected Sharpness=1.0, got %v", s.Sharpness)
	}
	if s.Exposure == nil || s.Exposure.Mode != "AUTO" {
		t.Errorf("expected Exposure.Mode=AUTO, got %v", s.Exposure)
	}
	if s.WhiteBalance == nil || s.WhiteBalance.Mode != "AUTO" {
		t.Errorf("expected WhiteBalance.Mode=AUTO, got %v", s.WhiteBalance)
	}
}

func TestSetImagingSettingsBrightness(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	soapBody := `<?xml version="1.0" encoding="UTF-8"?>
<timg:SetImagingSettings xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"
                         xmlns:tt="http://www.onvif.org/ver10/schema">
  <timg:Settings>
    <tt:Brightness tt:Value="0.5"/>
  </timg:Settings>
</timg:SetImagingSettings>`

	err := handleSetImagingSettings(pm, []byte(soapBody))
	if err != nil {
		t.Fatalf("handleSetImagingSettings failed: %v", err)
	}

	got := mock.getParam("brightness")
	if got != float64(0.5) {
		t.Errorf("expected camera brightness=0.5, got %v (%T)", got, got)
	}
}

func TestSetImagingSettingsContrast(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	soapBody := `<?xml version="1.0" encoding="UTF-8"?>
<timg:SetImagingSettings xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"
                         xmlns:tt="http://www.onvif.org/ver10/schema">
  <timg:Settings>
    <tt:Contrast tt:Value="2.5"/>
  </timg:Settings>
</timg:SetImagingSettings>`

	err := handleSetImagingSettings(pm, []byte(soapBody))
	if err != nil {
		t.Fatalf("handleSetImagingSettings failed: %v", err)
	}

	got := mock.getParam("contrast")
	if got != float64(2.5) {
		t.Errorf("expected camera contrast=2.5, got %v (%T)", got, got)
	}
}

func TestSetImagingSettingsMultipleParams(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	soapBody := `<?xml version="1.0" encoding="UTF-8"?>
<timg:SetImagingSettings xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"
                         xmlns:tt="http://www.onvif.org/ver10/schema">
  <timg:Settings>
    <tt:Brightness tt:Value="-0.3"/>
    <tt:Contrast tt:Value="1.5"/>
    <tt:ColorSaturation tt:Value="1.2"/>
    <tt:Sharpness tt:Value="0.8"/>
  </timg:Settings>
</timg:SetImagingSettings>`

	err := handleSetImagingSettings(pm, []byte(soapBody))
	if err != nil {
		t.Fatalf("handleSetImagingSettings failed: %v", err)
	}

	tests := []struct {
		name  string
		param string
		want  float64
	}{
		{"brightness", "brightness", -0.3},
		{"contrast", "contrast", 1.5},
		{"saturation", "saturation", 1.2},
		{"sharpness", "sharpness", 0.8},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := mock.getParam(tt.param)
			if got != tt.want {
				t.Errorf("expected %s=%v, got %v (%T)", tt.param, tt.want, got, got)
			}
		})
	}
}

func TestSetImagingSettingsOutOfRange(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	// Brightness max is 1.0, 2.0 should fail
	soapBody := `<?xml version="1.0" encoding="UTF-8"?>
<timg:SetImagingSettings xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl"
                         xmlns:tt="http://www.onvif.org/ver10/schema">
  <timg:Settings>
    <tt:Brightness tt:Value="2.0"/>
  </timg:Settings>
</timg:SetImagingSettings>`

	err := handleSetImagingSettings(pm, []byte(soapBody))
	if err == nil {
		t.Fatal("expected error for out-of-range Brightness=2.0")
	}
}

func TestSetImagingSettingsInvalidXML(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	err := handleSetImagingSettings(pm, []byte("not xml at all"))
	if err == nil {
		t.Fatal("expected error for invalid XML")
	}
}

func TestGetOptions(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	resp, err := handleGetOptions(pm)
	if err != nil {
		t.Fatalf("handleGetOptions failed: %v", err)
	}

	opts := resp.ImagingOptions

	// Verify ranges match ParamRanges
	br := camera.ParamRanges["Brightness"]
	if opts.Brightness == nil || opts.Brightness.Min != br.Min || opts.Brightness.Max != br.Max {
		t.Errorf("expected Brightness range [%.1f, %.1f], got %v", br.Min, br.Max, opts.Brightness)
	}
	cr := camera.ParamRanges["Contrast"]
	if opts.Contrast == nil || opts.Contrast.Min != cr.Min || opts.Contrast.Max != cr.Max {
		t.Errorf("expected Contrast range [%.1f, %.1f], got %v", cr.Min, cr.Max, opts.Contrast)
	}
	sr := camera.ParamRanges["Saturation"]
	if opts.Saturation == nil || opts.Saturation.Min != sr.Min || opts.Saturation.Max != sr.Max {
		t.Errorf("expected Saturation range [%.1f, %.1f], got %v", sr.Min, sr.Max, opts.Saturation)
	}
	shr := camera.ParamRanges["Sharpness"]
	if opts.Sharpness == nil || opts.Sharpness.Min != shr.Min || opts.Sharpness.Max != shr.Max {
		t.Errorf("expected Sharpness range [%.1f, %.1f], got %v", shr.Min, shr.Max, opts.Sharpness)
	}

	// Exposure options
	if opts.Exposure == nil {
		t.Fatal("expected Exposure options, got nil")
	}
	er := camera.ParamRanges["ExposureTime"]
	if opts.Exposure.MinExposureTime != er.Min {
		t.Errorf("expected MinExposureTime=%.1f, got %.1f", er.Min, opts.Exposure.MinExposureTime)
	}
	if opts.Exposure.MaxExposureTime != er.Max {
		t.Errorf("expected MaxExposureTime=%.1f, got %.1f", er.Max, opts.Exposure.MaxExposureTime)
	}

	// White balance options
	if opts.WhiteBalance == nil {
		t.Fatal("expected WhiteBalance options, got nil")
	}
	if opts.WhiteBalance.Mode == nil || !opts.WhiteBalance.Mode.Auto {
		t.Error("expected WhiteBalance mode Auto=true")
	}
}

// ---------------------------------------------------------------------------
// End-to-end HTTP tests
// ---------------------------------------------------------------------------

func TestGetImagingSettingsViaHTTP(t *testing.T) {
	srv := newImagingTestServer(newMockImagingCamera())

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl">
  <s:Body>
    <timg:GetImagingSettings>
      <timg:VideoSourceToken>videoSrc0</timg:VideoSourceToken>
    </timg:GetImagingSettings>
  </s:Body>
</s:Envelope>`

	req := httptest.NewRequest(http.MethodPost, "/onvif/imaging_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	// Parse response
	var env struct {
		XMLName xml.Name `xml:"Envelope"`
		Body    struct {
			GetImagingSettingsResponse struct {
				Settings struct {
					Brightness struct {
						Value float64 `xml:"Value,attr"`
					} `xml:"Brightness"`
					Contrast struct {
						Value float64 `xml:"Value,attr"`
					} `xml:"Contrast"`
					Saturation struct {
						Value float64 `xml:"Value,attr"`
					} `xml:"ColorSaturation"`
					Sharpness struct {
						Value float64 `xml:"Value,attr"`
					} `xml:"Sharpness"`
				} `xml:"Settings"`
			} `xml:"GetImagingSettingsResponse"`
		} `xml:"Body"`
	}

	if err := xml.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("failed to parse response: %v\nBody:\n%s", err, w.Body.String())
	}

	s := env.Body.GetImagingSettingsResponse.Settings
	if s.Brightness.Value != 0.0 {
		t.Errorf("expected Brightness=0.0, got %v", s.Brightness.Value)
	}
	if s.Contrast.Value != 1.0 {
		t.Errorf("expected Contrast=1.0, got %v", s.Contrast.Value)
	}
	if s.Saturation.Value != 1.0 {
		t.Errorf("expected Saturation=1.0, got %v", s.Saturation.Value)
	}
	if s.Sharpness.Value != 1.0 {
		t.Errorf("expected Sharpness=1.0, got %v", s.Sharpness.Value)
	}
}

func TestSetImagingSettingsViaHTTP(t *testing.T) {
	mock := newMockImagingCamera()
	srv := newImagingTestServer(mock)

	soapBody := `<timg:SetImagingSettings xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
      <timg:Settings>
        <tt:Brightness tt:Value="0.5"/>
      </timg:Settings>
    </timg:SetImagingSettings>`
	soapReq := wrapWithAuth(soapBody)

	req := httptest.NewRequest(http.MethodPost, "/onvif/imaging_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	got := mock.getParam("brightness")
	if got != float64(0.5) {
		t.Errorf("expected camera brightness=0.5, got %v (%T)", got, got)
	}
}

func TestSetImagingSettingsOutOfRangeViaHTTP(t *testing.T) {
	mock := newMockImagingCamera()
	srv := newImagingTestServer(mock)

	soapBody := `<timg:SetImagingSettings xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl" xmlns:tt="http://www.onvif.org/ver10/schema">
      <timg:Settings>
        <tt:Brightness tt:Value="2.0"/>
      </timg:Settings>
    </timg:SetImagingSettings>`
	soapReq := wrapWithAuth(soapBody)

	req := httptest.NewRequest(http.MethodPost, "/onvif/imaging_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	// Should get 500 SOAP fault for out-of-range
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 for out-of-range, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "out of range") {
		t.Errorf("expected 'out of range' in fault, got: %s", body)
	}

	// Verify camera was NOT called
	if got := mock.getParam("brightness"); got != float64(0.0) {
		t.Errorf("camera should not have been updated, got %v", got)
	}
}

func TestGetOptionsViaHTTP(t *testing.T) {
	srv := newImagingTestServer(newMockImagingCamera())

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope"
            xmlns:timg="http://www.onvif.org/ver20/imaging/wsdl">
  <s:Body>
    <timg:GetOptions>
      <timg:VideoSourceToken>videoSrc0</timg:VideoSourceToken>
    </timg:GetOptions>
  </s:Body>
</s:Envelope>`

	req := httptest.NewRequest(http.MethodPost, "/onvif/imaging_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()

	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d. Body: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	required := []string{
		"GetOptionsResponse",
		"ImagingOptions",
		"Brightness",
		"Contrast",
		"ColorSaturation",
		"Sharpness",
		"Exposure",
		"WhiteBalance",
		"-1",    // Brightness min
		"32",    // Contrast/Saturation max
		"16",    // Sharpness max
		"1e+06",  // ExposureTime max (Go XML renders 1000000 as 1e+06)
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("response XML missing %q\nBody:\n%s", want, body)
		}
	}
}

func TestGetImagingSettingsMarshalling(t *testing.T) {
	mock := newMockImagingCamera()
	pm := camera.NewParamManager(mock)

	resp, err := handleGetImagingSettings(pm)
	if err != nil {
		t.Fatalf("handleGetImagingSettings failed: %v", err)
	}

	data, err := MarshalSOAP(resp)
	if err != nil {
		t.Fatalf("MarshalSOAP failed: %v", err)
	}

	body := string(data)
	required := []string{
		"GetImagingSettingsResponse",
		"Brightness",
		"Contrast",
		"ColorSaturation",
		"Sharpness",
		"Exposure",
		"WhiteBalance",
		"AUTO",
	}
	for _, want := range required {
		if !strings.Contains(body, want) {
			t.Errorf("response XML missing %q\nBody:\n%s", want, body)
		}
	}
}
