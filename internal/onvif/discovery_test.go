package onvif

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/netutil"
)

// NVR probe template (matches NVR's wsDiscoveryProbe with 2005/04 namespace).
const nvrProbeTemplate = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://schemas.xmlsoap.org/ws/2004/08/addressing">
  <s:Header>
    <a:Action s:mustUnderstand="1">http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe</a:Action>
    <a:MessageID>uuid:%s</a:MessageID>
    <a:ReplyTo><a:Address>http://schemas.xmlsoap.org/ws/2004/08/addressing/role/anonymous</a:Address></a:ReplyTo>
    <a:To s:mustUnderstand="1">urn:schemas-xmlsoap-org:ws:2005:04:discovery</a:To>
  </s:Header>
  <s:Body>
    <Probe xmlns="http://schemas.xmlsoap.org/ws/2005/04/discovery">
      <d:Types xmlns:d="http://schemas.xmlsoap.org/ws/2005/04/discovery" xmlns:dp0="http://www.onvif.org/ver10/network/wsdl">dp0:NetworkVideoTransmitter</d:Types>
    </Probe>
  </s:Body>
</s:Envelope>`

// 2004/09 namespace probe (standard WS-Discovery).
const probe2004Template = `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing">
  <s:Header>
    <a:Action s:mustUnderstand="1">http://schemas.xmlsoap.org/ws/2004/09/discovery/Probe</a:Action>
    <a:MessageID>uuid:%s</a:MessageID>
    <a:To s:mustUnderstand="1">urn:schemas-xmlsoap-org:ws:2004:09:discovery</a:To>
  </s:Header>
  <s:Body>
    <Probe xmlns="http://schemas.xmlsoap.org/ws/2004/09/discovery"/>
  </s:Body>
</s:Envelope>`

func testDiscovery() *Discovery {
	cfg := config.DefaultConfig()
	return NewDiscovery(cfg, "192.168.1.100")
}

func TestBuildProbeMatches(t *testing.T) {
	d := testDiscovery()
	msgID := "uuid:12345678-1234-1234-1234-123456789abc"
	// Empty client IP -> falls back to device's own address (192.168.1.100).
	resp := d.buildProbeMatches(msgID, "")

	body := string(resp)

	// Verify XML declaration
	if !strings.Contains(body, `<?xml version="1.0" encoding="UTF-8"?>`) {
		t.Fatal("missing XML declaration")
	}

	// Verify ProbeMatches action
	if !strings.Contains(body, ProbeMatchesAction) {
		t.Fatalf("missing ProbeMatches action: %s", body)
	}

	// Verify RelatesTo contains original message ID
	if !strings.Contains(body, msgID) {
		t.Fatalf("missing RelatesTo message ID: %s", body)
	}

	// Verify UUID
	if !strings.Contains(body, d.uuid) {
		t.Fatalf("missing UUID in response: %s", body)
	}

	// Verify Scopes
	for _, scope := range d.scopes {
		if !strings.Contains(body, scope) {
			t.Fatalf("missing scope %s in response: %s", scope, body)
		}
	}

	// Verify XAddr
	if !strings.Contains(body, "192.168.1.100:8080/onvif/device_service") {
		t.Fatalf("missing XAddr in response: %s", body)
	}

	// Verify Types
	if !strings.Contains(body, "NetworkVideoTransmitter") {
		t.Fatalf("missing NetworkVideoTransmitter type: %s", body)
	}

	// Verify MetadataVersion
	if !strings.Contains(body, "<d:MetadataVersion>1</d:MetadataVersion>") {
		t.Fatalf("missing MetadataVersion: %s", body)
	}

	// Verify ProbeMatch structure
	if !strings.Contains(body, "<d:ProbeMatch>") {
		t.Fatal("missing ProbeMatch element")
	}
}

func TestBuildProbeMatchesPerHostIP(t *testing.T) {
	d := testDiscovery()
	msgID := "uuid:per-host-test"

	// Empty hostIP -> device's own fallback IP.
	resp := d.buildProbeMatches(msgID, "")
	if !strings.Contains(string(resp), "192.168.1.100:8080/onvif/device_service") {
		t.Errorf("expected fallback IP in ProbeMatches, got: %s", resp)
	}

	// Explicit server IP (e.g. from ConnContext) -> used in XAddrs.
	// This happens for HTTP probes where ConnContext captures the server's local IP.
	resp = d.buildProbeMatches(msgID, "192.168.1.100")
	if !strings.Contains(string(resp), "192.168.1.100:8080/onvif/device_service") {
		t.Errorf("expected server IP in ProbeMatches, got: %s", resp)
	}
}

func TestHandleHTTPProbe(t *testing.T) {
	d := testDiscovery()
	msgID := "test-msg-id-001"
	probeMsg := fmt.Sprintf(nvrProbeTemplate, msgID)

	t.Run("valid NVR probe returns ProbeMatches", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(probeMsg))
		req.Header.Set("Content-Type", "application/soap+xml; charset=utf-8")
		w := httptest.NewRecorder()

		d.HandleHTTPProbe(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}

		body := w.Body.String()
		if !strings.Contains(body, "ProbeMatches") {
			t.Fatalf("response missing ProbeMatches: %s", body)
		}
		if !strings.Contains(body, msgID) {
			t.Fatalf("response missing RelatesTo with message ID: %s", body)
		}
	})

	t.Run("2004/09 namespace probe also works", func(t *testing.T) {
		probeMsg2004 := fmt.Sprintf(probe2004Template, msgID)
		req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(probeMsg2004))
		w := httptest.NewRecorder()

		d.HandleHTTPProbe(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if !strings.Contains(w.Body.String(), "ProbeMatches") {
			t.Fatalf("response missing ProbeMatches: %s", w.Body.String())
		}
	})

	t.Run("GET method returns 405", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/onvif/device_service", nil)
		w := httptest.NewRecorder()

		d.HandleHTTPProbe(w, req)

		if w.Code != http.StatusMethodNotAllowed {
			t.Fatalf("expected 405, got %d", w.Code)
		}
	})

	t.Run("non-probe POST returns 200 empty", func(t *testing.T) {
		soapMsg := `<?xml version="1.0" encoding="UTF-8"?>
<s:Envelope xmlns:s="http://www.w3.org/2003/05/soap-envelope" xmlns:a="http://www.w3.org/2005/08/addressing">
  <s:Header>
    <a:Action>http://some/other/action</a:Action>
    <a:MessageID>uuid:abc</a:MessageID>
  </s:Header>
  <s:Body></s:Body>
</s:Envelope>`
		req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader(soapMsg))
		w := httptest.NewRecorder()

		d.HandleHTTPProbe(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "ProbeMatches") {
			t.Fatal("non-probe should not return ProbeMatches")
		}
	})

	t.Run("invalid XML returns 200 empty", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/onvif/device_service", strings.NewReader("not xml"))
		w := httptest.NewRecorder()

		d.HandleHTTPProbe(w, req)

		if w.Code != http.StatusOK {
			t.Fatalf("expected 200, got %d", w.Code)
		}
	})
}

func TestScopeFormat(t *testing.T) {
	cfg := config.DefaultConfig()
	d := NewDiscovery(cfg, "192.168.1.100")

	// Verify scopes contain /name/ prefix
	var hasName, hasHardware bool
	for _, scope := range d.scopes {
		if strings.Contains(scope, "/name/") {
			hasName = true
			// NVR extracts last segment: scope = "onvif://www.onvif.org/name/Pi Camera V1"
			parts := strings.Split(scope, "/")
			name := parts[len(parts)-1]
			if name != "Pi Camera V1" {
				t.Fatalf("name scope wrong, got %q, want %q", name, "Pi Camera V1")
			}
		}
		if strings.Contains(scope, "/hardware/") {
			hasHardware = true
			parts := strings.Split(scope, "/")
			hw := parts[len(parts)-1]
			if hw != "OV5647" {
				t.Fatalf("hardware scope wrong, got %q, want %q", hw, "OV5647")
			}
		}
	}

	if !hasName {
		t.Fatal("scopes missing /name/ entry")
	}
	if !hasHardware {
		t.Fatal("scopes missing /hardware/ entry")
	}
}

func TestDetectLocalIP(t *testing.T) {
	ip := netutil.DetectLocalIP()

	if ip == "" {
		t.Fatal("detectLocalIP returned empty string")
	}

	// Should be a valid IPv4 address format (simple check)
	parts := strings.Split(ip, ".")
	if len(parts) != 4 {
		t.Fatalf("expected IPv4 format, got %s", ip)
	}
}

func TestIsProbeAction(t *testing.T) {
	tests := []struct {
		action string
		want   bool
	}{
		{"http://schemas.xmlsoap.org/ws/2004/09/discovery/Probe", true},
		{"http://schemas.xmlsoap.org/ws/2005/04/discovery/Probe", true},
		{"http://some/other/action", false},
		{"", false},
		{"Probe", false},
	}

	for _, tt := range tests {
		got := isProbeAction(tt.action)
		if got != tt.want {
			t.Errorf("isProbeAction(%q) = %v, want %v", tt.action, got, tt.want)
		}
	}
}

func TestHandleProbeWithMessageID(t *testing.T) {
	d := testDiscovery()
	msgID := "uuid:aaaa-bbbb-cccc-dddd"
	probeMsg := fmt.Sprintf(nvrProbeTemplate, msgID)
	resp := d.handleProbe([]byte(probeMsg), "")

	if resp == nil {
		t.Fatal("handleProbe returned nil for valid probe")
	}

	body := string(resp)
	if !strings.Contains(body, msgID) {
		t.Fatalf("response missing message ID %q: %s", msgID, body)
	}
	if !strings.Contains(body, "RelatesTo") {
		t.Fatal("response missing RelatesTo element")
	}
}

func TestDiscoveryDefaultsWithEmptyConfig(t *testing.T) {
	// Verify NewDiscovery handles zero values gracefully
	cfg := &config.Config{}
	d := NewDiscovery(cfg, "10.0.0.1")

	if d.uuid == "" {
		t.Fatal("UUID should not be empty")
	}
	if !strings.HasPrefix(d.uuid, "uuid:") {
		t.Fatalf("UUID should have uuid: prefix, got %s", d.uuid)
	}
	if len(d.scopes) != 2 {
		t.Fatalf("expected 2 scopes, got %d", len(d.scopes))
	}
	xaddrs := d.XAddrs("")
	if len(xaddrs) != 1 {
		t.Fatalf("expected 1 XAddr, got %d", len(xaddrs))
	}
	if !strings.Contains(xaddrs[0], "10.0.0.1") {
		t.Fatalf("XAddr should contain provided IP, got %s", xaddrs[0])
	}
}
