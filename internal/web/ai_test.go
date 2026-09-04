package web

import (
	"net/http"
	"testing"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ai"
)

func aiTestServer(t *testing.T, active bool) *Server {
	t.Helper()
	var svc *ai.Service
	if active {
		svc = ai.NewService(ai.Options{Enabled: true}, nil, func(ai.Options) (ai.Detector, error) {
			return &fakeAIDetector{}, nil
		})
		if svc == nil {
			t.Fatal("service must be active with a working detector")
		}
	}
	return New(Config{
		Port:        8088,
		Username:    "admin",
		Password:    "spec-pass-1",
		OnvifConfig: &mockOnvifConfig{port: 8080, username: "onvif-user", password: "onvif-pass"},
		Version:     "test",
		AI:          svc,
	})
}

type fakeAIDetector struct{}

func (fakeAIDetector) Detect(*ai.Frame, uint32, uint32) ([]ai.Detection, error) {
	return []ai.Detection{{Label: "chair", Confidence: 0.7, BBox: [4]uint32{8, 9, 10, 11}}}, nil
}

func (fakeAIDetector) ModelName() string { return "fake.onnx" }

func TestDetectionsDisabledReportsEnabledFalse(t *testing.T) {
	s := aiTestServer(t, false)
	cookie, _ := specLogin(t, s)
	rec := doReq(t, s, http.MethodGet, "/api/detections", "", map[string]string{"Cookie": cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	body := decode(t, rec)
	data := body["data"].(map[string]interface{})
	if data["enabled"] != false {
		t.Fatalf("data = %v, want enabled:false", data)
	}
}

func TestCapabilitiesAIRelated(t *testing.T) {
	s := aiTestServer(t, false)
	cookie, _ := specLogin(t, s)
	rec := doReq(t, s, http.MethodGet, "/api/capabilities", "", map[string]string{"Cookie": cookie})
	data := decode(t, rec)["data"].(map[string]interface{})
	if data["ai"] != false {
		t.Fatalf("ai capability = %v, want false without service", data["ai"])
	}
	events := data["events"].([]interface{})
	for _, e := range events {
		if e == "ai_detection" {
			t.Fatal("ai_detection must not be advertised when AI is off")
		}
	}
}

func TestDetectionsRequiresAuth(t *testing.T) {
	s := aiTestServer(t, true)
	rec := doReq(t, s, http.MethodGet, "/api/detections", "", nil)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated status = %d, want 401", rec.Code)
	}
}

func TestDetectionsActiveServesSnapshot(t *testing.T) {
	s := aiTestServer(t, true)
	cookie, _ := specLogin(t, s)
	rec := doReq(t, s, http.MethodGet, "/api/detections", "", map[string]string{"Cookie": cookie})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body %s", rec.Code, rec.Body.String())
	}
	body := decode(t, rec)
	data := body["data"].(map[string]interface{})
	if data["model"] != "fake.onnx" {
		t.Fatalf("model = %v", data["model"])
	}
	if _, ok := data["detections"]; !ok {
		t.Fatalf("detections key missing: %v", data)
	}
}
