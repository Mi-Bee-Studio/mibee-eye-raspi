package onvif

import (
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestDeviceGetInformation(t *testing.T) {
	host := "192.168.1.100:8080"
	info := DeviceInfo{
		Name:         "Pi Camera V1",
		Manufacturer: "Raspberry Pi",
		Model:        "OV5647",
		Firmware:     "1.0.0",
		HardwareID:   "OV5647",
		SerialNumber: "",
	}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetDeviceInformationResponse") {
		t.Fatalf("response missing GetDeviceInformationResponse: %s", body)
	}
	if !strings.Contains(body, "Manufacturer") {
		t.Fatalf("response missing Manufacturer: %s", body)
	}

	// Unmarshal and verify fields
	var resp struct {
		Body struct {
			Response struct {
				Manufacturer    string `xml:"Manufacturer"`
				Model           string `xml:"Model"`
				FirmwareVersion string `xml:"FirmwareVersion"`
				SerialNumber    string `xml:"SerialNumber"`
				HardwareId      string `xml:"HardwareId"`
			} `xml:"GetDeviceInformationResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	if resp.Body.Response.Manufacturer != "Raspberry Pi" {
		t.Errorf("expected Manufacturer=Raspberry Pi, got %q", resp.Body.Response.Manufacturer)
	}
	if resp.Body.Response.Model != "OV5647" {
		t.Errorf("expected Model=OV5647, got %q", resp.Body.Response.Model)
	}
	if resp.Body.Response.FirmwareVersion != "1.0.0" {
		t.Errorf("expected FirmwareVersion=1.0.0, got %q", resp.Body.Response.FirmwareVersion)
	}
	if resp.Body.Response.SerialNumber != "" {
		t.Errorf("expected SerialNumber empty, got %q", resp.Body.Response.SerialNumber)
	}
	if resp.Body.Response.HardwareId != "OV5647" {
		t.Errorf("expected HardwareId=OV5647, got %q", resp.Body.Response.HardwareId)
	}
}

func TestDeviceGetSystemDateAndTime(t *testing.T) {
	host := "192.168.1.100:8080"
	info := DeviceInfo{Name: "Test", HardwareID: "TEST"}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetSystemDateAndTime xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetSystemDateAndTimeResponse") {
		t.Fatalf("response missing GetSystemDateAndTimeResponse: %s", body)
	}
	if !strings.Contains(body, "UTCDateTime") {
		t.Fatalf("response missing UTCDateTime: %s", body)
	}
	if !strings.Contains(body, "DateTimeType") {
		t.Fatalf("response missing DateTimeType: %s", body)
	}

	// Verify the time fields make sense (within 5 seconds of now)
	var resp struct {
		Body struct {
			Response struct {
				SystemDateAndTime struct {
					DateTimeType string `xml:"DateTimeType"`
					UTCDateTime  struct {
						Time struct {
							Hour   int `xml:"Hour"`
							Minute int `xml:"Minute"`
							Second int `xml:"Second"`
						} `xml:"Time"`
						Date struct {
							Year  int `xml:"Year"`
							Month int `xml:"Month"`
							Day   int `xml:"Day"`
						} `xml:"Date"`
					} `xml:"UTCDateTime"`
				} `xml:"SystemDateAndTime"`
			} `xml:"GetSystemDateAndTimeResponse"`
		} `xml:"Body"`
	}
	if err := xml.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to unmarshal response: %v", err)
	}

	now := time.Now().UTC()
	if resp.Body.Response.SystemDateAndTime.DateTimeType != "Manual" {
		t.Errorf("expected DateTimeType=Manual, got %q", resp.Body.Response.SystemDateAndTime.DateTimeType)
	}

	d := resp.Body.Response.SystemDateAndTime.UTCDateTime.Date
	ti := resp.Body.Response.SystemDateAndTime.UTCDateTime.Time

	// Check date matches today
	if d.Year != now.Year() || d.Month != int(now.Month()) || d.Day != now.Day() {
		t.Errorf("date mismatch: got %d-%02d-%02d, want %d-%02d-%02d",
			d.Year, d.Month, d.Day, now.Year(), now.Month(), now.Day())
	}

	// Check time is within 5 seconds of now
	responseTime := time.Date(d.Year, time.Month(d.Month), d.Day, ti.Hour, ti.Minute, ti.Second, 0, time.UTC)
	diff := now.Sub(responseTime)
	if diff < 0 {
		diff = -diff
	}
	if diff > 5*time.Second {
		t.Errorf("response time too far from now: diff=%v", diff)
	}
}

func TestDeviceGetCapabilities(t *testing.T) {
	host := "192.168.1.100:8080"
	info := DeviceInfo{Name: "Test", HardwareID: "TEST"}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetCapabilities xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	// Simulate the NVR reaching the RPi. In production, http.Server.ConnContext
	// injects the server-side local IP (the RPi interface that accepted the
	// connection). In tests, we do that injection manually.
	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	req.RemoteAddr = "192.168.1.101:55123"
	req = req.WithContext(WithServerIP(req.Context(), "192.168.1.100"))
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetCapabilitiesResponse") {
		t.Fatalf("response missing GetCapabilitiesResponse: %s", body)
	}
	if !strings.Contains(body, "XAddr") {
		t.Fatalf("response missing XAddr: %s", body)
	}
	// Verify the RPi interface IP is in all XAddrs (not the NVR's source IP).
	if !strings.Contains(body, "http://192.168.1.100:8080/onvif/media_service") {
		t.Errorf("response missing Media XAddr with server IP: %s", body)
	}
	if !strings.Contains(body, "http://192.168.1.100:8080/onvif/device_service") {
		t.Errorf("response missing Device XAddr with server IP: %s", body)
	}
	if !strings.Contains(body, "tt:Imaging") {
		t.Errorf("response missing tt:Imaging in Capabilities: %s", body)
	}
}

func TestDeviceGetCapabilitiesFallback(t *testing.T) {
	// When the request has no client IP (empty RemoteAddr), the device's own
	// fallbackHost should be used.
	host := "192.168.1.100:8080"
	info := DeviceInfo{Name: "Test", HardwareID: "TEST"}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetCapabilities xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	req.RemoteAddr = "" // no client IP — must fall back to host
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "http://192.168.1.100:8080/onvif/media_service") {
		t.Errorf("fallback should use device's own address: %s", body)
	}
}

func TestDeviceGetServices(t *testing.T) {
	host := "192.168.1.100:8080"
	info := DeviceInfo{Name: "Test", HardwareID: "TEST"}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetServices xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetServicesResponse") {
		t.Fatalf("response missing GetServicesResponse: %s", body)
	}
	if !strings.Contains(body, "http://www.onvif.org/ver10/device/wsdl") {
		t.Errorf("response missing Device namespace: %s", body)
	}
	if !strings.Contains(body, "http://www.onvif.org/ver10/media/wsdl") {
		t.Errorf("response missing Media namespace: %s", body)
	}
	if !strings.Contains(body, "http://www.onvif.org/ver20/imaging/wsdl") {
		t.Errorf("response missing Imaging namespace: %s", body)
	}
}

func TestDeviceGetScopes(t *testing.T) {
	host := "192.168.1.100:8080"
	info := DeviceInfo{
		Name:       "Pi Camera V1",
		HardwareID: "OV5647",
	}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetScopes xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "GetScopesResponse") {
		t.Fatalf("response missing GetScopesResponse: %s", body)
	}
	if !strings.Contains(body, "onvif://www.onvif.org/name/Pi Camera V1") {
		t.Errorf("response missing name scope: %s", body)
	}
	if !strings.Contains(body, "onvif://www.onvif.org/hardware/OV5647") {
		t.Errorf("response missing hardware scope: %s", body)
	}
	if !strings.Contains(body, "onvif://www.onvif.org/type/video_encoder") {
		t.Errorf("response missing type scope: %s", body)
	}
}

func TestDeviceGetInformationDefaultConfig(t *testing.T) {
	host := "192.168.1.100:8080"
	info := DeviceInfo{
		Name:         "Pi Camera V1",
		Manufacturer: "Raspberry Pi",
		Model:        "OV5647",
		Firmware:     "1.0.0",
		HardwareID:   "OV5647",
		SerialNumber: "",
	}

	soapReq := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope">
  <s:Body>
    <tds:GetDeviceInformation xmlns:tds="http://www.onvif.org/ver10/device/wsdl"/>
  </s:Body>
</s:Envelope>`

	cfg := &mockConfig{username: "admin", password: "testpass", port: 8080}
	srv := New(cfg)
	RegisterDeviceHandlers(srv, host, info)

	req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapReq))
	req.Header.Set("Content-Type", "application/soap+xml")
	w := httptest.NewRecorder()
	srv.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	body := w.Body.String()
	if !strings.Contains(body, "Raspberry Pi") {
		t.Errorf("expected Manufacturer 'Raspberry Pi' in response: %s", body)
	}
}
