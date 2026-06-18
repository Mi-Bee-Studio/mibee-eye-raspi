package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"strings"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"
)

// ---------------------------------------------------------------------------
// Response types — GetImagingSettings
// ---------------------------------------------------------------------------

// GetImagingSettingsResponse is the ONVIF GetImagingSettings SOAP response.
type GetImagingSettingsResponse struct {
	XMLName  xml.Name        `xml:"timg:GetImagingSettingsResponse"`
	Settings ImagingSettings `xml:"timg:Settings"`
}

// ImagingSettings holds current imaging parameter values.
type ImagingSettings struct {
	Brightness   *FloatItem    `xml:"tt:Brightness,omitempty"`
	Contrast     *FloatItem    `xml:"tt:Contrast,omitempty"`
	Saturation   *FloatItem    `xml:"tt:ColorSaturation,omitempty"`
	Sharpness    *FloatItem    `xml:"tt:Sharpness,omitempty"`
	Exposure     *Exposure     `xml:"tt:Exposure,omitempty"`
	WhiteBalance *WhiteBalance `xml:"tt:WhiteBalance,omitempty"`
}

// FloatItem represents an imaging parameter with a float value.
type FloatItem struct {
	Value float64 `xml:"tt:Value,attr"`
}

// Exposure holds exposure settings.
type Exposure struct {
	Mode string  `xml:"tt:Mode"`
	Time float64 `xml:"tt:ExposureTime,omitempty"`
}

// WhiteBalance holds white balance settings.
type WhiteBalance struct {
	Mode   string  `xml:"tt:Mode"`
	CrGain float64 `xml:"tt:CrGain,omitempty"`
	CbGain float64 `xml:"tt:CbGain,omitempty"`
}

// ---------------------------------------------------------------------------
// Response types — GetOptions
// ---------------------------------------------------------------------------

// GetOptionsResponse is the ONVIF GetOptions SOAP response.
type GetOptionsResponse struct {
	XMLName        xml.Name        `xml:"timg:GetOptionsResponse"`
	ImagingOptions ImagingOptions `xml:"timg:ImagingOptions"`
}

// ImagingOptions holds supported ranges for imaging parameters.
type ImagingOptions struct {
	Brightness   *FloatRange          `xml:"tt:Brightness"`
	Contrast     *FloatRange          `xml:"tt:Contrast"`
	Saturation   *FloatRange          `xml:"tt:ColorSaturation"`
	Sharpness    *FloatRange          `xml:"tt:Sharpness"`
	Exposure     *ExposureOptions     `xml:"tt:Exposure"`
	WhiteBalance *WhiteBalanceOptions `xml:"tt:WhiteBalance"`
}

// FloatRange defines min/max range for an imaging parameter.
type FloatRange struct {
	Min float64 `xml:"tt:Min"`
	Max float64 `xml:"tt:Max"`
}

// ExposureOptions holds supported exposure options.
type ExposureOptions struct {
	MinExposureTime float64 `xml:"tt:MinExposureTime"`
	MaxExposureTime float64 `xml:"tt:MaxExposureTime"`
	MinGain         float64 `xml:"tt:MinGain"`
	MaxGain         float64 `xml:"tt:MaxGain"`
}

// WhiteBalanceOptions holds supported white balance options.
type WhiteBalanceOptions struct {
	Mode   *WhiteBalanceModeOptions `xml:"tt:Mode"`
	CrGain *WhiteBalanceRange        `xml:"tt:CrGain"`
	CbGain *WhiteBalanceRange        `xml:"tt:CbGain"`
}

// WhiteBalanceModeOptions lists supported white balance modes.
type WhiteBalanceModeOptions struct {
	Auto   bool `xml:"tt:Auto,attr"`
	Manual bool `xml:"tt:Manual,attr"`
}

// WhiteBalanceRange defines min/max for white balance gains.
type WhiteBalanceRange struct {
	Min float64 `xml:"tt:Min"`
	Max float64 `xml:"tt:Max"`
}

// ---------------------------------------------------------------------------
// Request parsing types (namespace-agnostic for incoming SOAP)
//
// Go's encoding/xml requires struct tags to use the *declared* namespace
// prefix from the struct's XMLName. Incoming SOAP body has different
// namespace prefixes (timg, tt). We use a decoder that matches on local
// element name regardless of namespace.
// ---------------------------------------------------------------------------

// setImagingSettingsRequest is used to parse incoming SetImagingSettings requests.
// Struct tags use namespace-local matching (no prefix) via custom decoder.
type setImagingSettingsRequest struct {
	Settings imagingSettingsRequest `xml:"Settings"`
}

type imagingSettingsRequest struct {
	Brightness   *floatItemRequest `xml:"Brightness"`
	Contrast     *floatItemRequest `xml:"Contrast"`
	Saturation   *floatItemRequest `xml:"ColorSaturation"`
	Sharpness    *floatItemRequest `xml:"Sharpness"`
	Exposure     *exposureRequest  `xml:"Exposure"`
	WhiteBalance *whiteBalanceRequest `xml:"WhiteBalance"`
}

type floatItemRequest struct {
	Value float64 `xml:"Value,attr"`
}

type exposureRequest struct {
	Mode string  `xml:"Mode"`
	Time float64 `xml:"ExposureTime"`
}

type whiteBalanceRequest struct {
	Mode   string  `xml:"Mode"`
	CrGain float64 `xml:"CrGain"`
	CbGain float64 `xml:"CbGain"`
}

// ---------------------------------------------------------------------------
// Handler registration
// ---------------------------------------------------------------------------

// RegisterImagingHandlers registers Imaging service handlers on the ONVIF server.
// Requires a ParamManager for camera control.
func RegisterImagingHandlers(s *Server, pm *camera.ParamManager) {
	s.RegisterAction("GetImagingSettings", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetImagingSettings(pm)
	})

	s.RegisterAction("SetImagingSettings", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return nil, handleSetImagingSettings(pm, body)
	})

	s.RegisterAction("GetOptions", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetOptions(pm)
	})
}

// handleGetImagingSettings returns current imaging settings from the camera.
func handleGetImagingSettings(pm *camera.ParamManager) (*GetImagingSettingsResponse, error) {
	brightness, err := pm.Get("Brightness")
	if err != nil {
		return nil, fmt.Errorf("get brightness: %w", err)
	}
	contrast, err := pm.Get("Contrast")
	if err != nil {
		return nil, fmt.Errorf("get contrast: %w", err)
	}
	saturation, err := pm.Get("Saturation")
	if err != nil {
		return nil, fmt.Errorf("get saturation: %w", err)
	}
	sharpness, err := pm.Get("Sharpness")
	if err != nil {
		return nil, fmt.Errorf("get sharpness: %w", err)
	}
	exposureTime, err := pm.Get("ExposureTime")
	if err != nil {
		return nil, fmt.Errorf("get exposure time: %w", err)
	}

	// Map camera exposure mode to ONVIF AUTO/MANUAL
	// Camera "normal" = ONVIF AUTO, "custom" = ONVIF MANUAL
	exposureMode := "AUTO"
	if rawMode, err := pm.Get("ExposureMode"); err == nil {
		if mode, ok := rawMode.(string); ok && mode == "custom" {
			exposureMode = "MANUAL"
		}
	}

	// Map camera AWB mode to ONVIF AUTO/MANUAL
	// Camera "auto" = ONVIF AUTO, anything else = ONVIF MANUAL
	wbMode := "AUTO"
	if rawMode, err := pm.Get("AWBMode"); err == nil {
		if mode, ok := rawMode.(string); ok && mode != "auto" {
			wbMode = "MANUAL"
		}
	}

	return &GetImagingSettingsResponse{
		Settings: ImagingSettings{
			Brightness: &FloatItem{Value: toFloat(brightness)},
			Contrast:   &FloatItem{Value: toFloat(contrast)},
			Saturation: &FloatItem{Value: toFloat(saturation)},
			Sharpness:  &FloatItem{Value: toFloat(sharpness)},
			Exposure: &Exposure{
				Mode: exposureMode,
				Time: toFloat(exposureTime),
			},
			WhiteBalance: &WhiteBalance{
				Mode: wbMode,
			},
		},
	}, nil
}

// handleSetImagingSettings parses requested settings and applies them to the camera.
func handleSetImagingSettings(pm *camera.ParamManager, body []byte) error {
	var req setImagingSettingsRequest
	if err := unmarshalAnyNS(body, &req); err != nil {
		return fmt.Errorf("parsing SetImagingSettings request: %w", err)
	}

	if req.Settings.Brightness != nil {
		if err := pm.Set("Brightness", req.Settings.Brightness.Value); err != nil {
			return err
		}
	}
	if req.Settings.Contrast != nil {
		if err := pm.Set("Contrast", req.Settings.Contrast.Value); err != nil {
			return err
		}
	}
	if req.Settings.Saturation != nil {
		if err := pm.Set("Saturation", req.Settings.Saturation.Value); err != nil {
			return err
		}
	}
	if req.Settings.Sharpness != nil {
		if err := pm.Set("Sharpness", req.Settings.Sharpness.Value); err != nil {
			return err
		}
	}
	if req.Settings.Exposure != nil {
		switch req.Settings.Exposure.Mode {
		case "MANUAL":
			if err := pm.Set("ExposureMode", "custom"); err != nil {
				return err
			}
			if err := pm.Set("ExposureTime", req.Settings.Exposure.Time); err != nil {
				return err
			}
		case "AUTO":
			if err := pm.Set("ExposureMode", "normal"); err != nil {
				return err
			}
		}
	}
	if req.Settings.WhiteBalance != nil {
		switch req.Settings.WhiteBalance.Mode {
		case "MANUAL":
			if err := pm.Set("AWBMode", "custom"); err != nil {
				return err
			}
		case "AUTO":
			if err := pm.Set("AWBMode", "auto"); err != nil {
				return err
			}
		}
	}

	return nil
}

// handleGetOptions returns supported ranges for imaging parameters.
func handleGetOptions(pm *camera.ParamManager) (*GetOptionsResponse, error) {
	return &GetOptionsResponse{
		ImagingOptions: ImagingOptions{
			Brightness: &FloatRange{
				Min: camera.ParamRanges["Brightness"].Min,
				Max: camera.ParamRanges["Brightness"].Max,
			},
			Contrast: &FloatRange{
				Min: camera.ParamRanges["Contrast"].Min,
				Max: camera.ParamRanges["Contrast"].Max,
			},
			Saturation: &FloatRange{
				Min: camera.ParamRanges["Saturation"].Min,
				Max: camera.ParamRanges["Saturation"].Max,
			},
			Sharpness: &FloatRange{
				Min: camera.ParamRanges["Sharpness"].Min,
				Max: camera.ParamRanges["Sharpness"].Max,
			},
			Exposure: &ExposureOptions{
				MinExposureTime: camera.ParamRanges["ExposureTime"].Min,
				MaxExposureTime: camera.ParamRanges["ExposureTime"].Max,
				MinGain:         camera.ParamRanges["Gain"].Min,
				MaxGain:         camera.ParamRanges["Gain"].Max,
			},
			WhiteBalance: &WhiteBalanceOptions{
				Mode:   &WhiteBalanceModeOptions{Auto: true, Manual: true},
				CrGain: &WhiteBalanceRange{Min: 0, Max: 1},
				CbGain: &WhiteBalanceRange{Min: 0, Max: 1},
			},
		},
	}, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// unmarshalAnyNS unmarshals XML data into v, matching element names by local
// name only (ignoring namespace). This handles ONVIF SOAP requests where
// namespace prefixes vary between clients.
func unmarshalAnyNS(data []byte, v interface{}) error {
	decoder := xml.NewDecoder(strings.NewReader(string(data)))
	for {
		tok, err := decoder.Token()
		if err != nil {
			if err == io.EOF {
				break
			}
			return err
		}
		if se, ok := tok.(xml.StartElement); ok {
			// Strip namespace from the first element so Unmarshal matches on local name
			se.Name.Space = ""
			return decoder.DecodeElement(v, &se)
		}
	}
	return fmt.Errorf("no XML start element found")
}

// toFloat converts interface{} to float64 for response building.
func toFloat(v interface{}) float64 {
	switch val := v.(type) {
	case float64:
		return val
	case float32:
		return float64(val)
	case int:
		return float64(val)
	case int64:
		return float64(val)
	case uint32:
		return float64(val)
	default:
		return 0
	}
}
