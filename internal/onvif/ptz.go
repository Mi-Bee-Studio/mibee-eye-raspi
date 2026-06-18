package onvif

import (
	"context"
	"encoding/xml"
	"fmt"
	"strings"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ptz"
)

// ---------------------------------------------------------------------------
// ONVIF PTZ request/response types
// ---------------------------------------------------------------------------

// PTZVelocity represents an ONVIF PTZ velocity vector.
type PTZVelocity struct {
	PanTilt *Vector2D `xml:"PanTilt,omitempty"`
	Zoom    *Vector1D `xml:"Zoom,omitempty"`
}

// PTZPosition represents an ONVIF PTZ position vector.
type PTZPosition struct {
	PanTilt *Vector2D `xml:"PanTilt,omitempty"`
	Zoom    *Vector1D `xml:"Zoom,omitempty"`
}

// Vector2D represents a 2D vector (PanTilt) in ONVIF schema.
type Vector2D struct {
	X float64 `xml:"x,attr"`
	Y float64 `xml:"y,attr"`
	Space string `xml:"space,attr,omitempty"`
}

// Vector1D represents a 1D vector (Zoom) in ONVIF schema.
type Vector1D struct {
	X     float64 `xml:"x,attr"`
	Space string  `xml:"space,attr,omitempty"`
}

// PTZStatus holds the PTZ move status and position.
type PTZStatus struct {
	PanTilt *PTZStatusVector `xml:"PanTilt,omitempty"`
	Zoom    *PTZStatusVector `xml:"Zoom,omitempty"`
}

// PTZStatusVector holds position and move status for one axis.
type PTZStatusVector struct {
	Position  float64 `xml:"Position>Vector>x"`
	MoveStatus string `xml:"MoveStatus"`
}

// ---------------------------------------------------------------------------
// Response types — PTZ operations
// ---------------------------------------------------------------------------

// ContinuousMoveResponse is the ONVIF ContinuousMove SOAP response.
type ContinuousMoveResponse struct {
	XMLName xml.Name `xml:"tptz:ContinuousMoveResponse"`
}

// AbsoluteMoveResponse is the ONVIF AbsoluteMove SOAP response.
type AbsoluteMoveResponse struct {
	XMLName xml.Name `xml:"tptz:AbsoluteMoveResponse"`
}

// RelativeMoveResponse is the ONVIF RelativeMove SOAP response.
type RelativeMoveResponse struct {
	XMLName xml.Name `xml:"tptz:RelativeMoveResponse"`
}

// StopResponse is the ONVIF Stop SOAP response.
type StopResponse struct {
	XMLName xml.Name `xml:"tptz:StopResponse"`
}

// GetStatusResponse is the ONVIF GetStatus SOAP response.
type GetStatusResponse struct {
	XMLName    xml.Name  `xml:"tptz:GetStatusResponse"`
	PTZStatus  PTZStatus `xml:"tptz:PTZStatus"`
}

// GetPresetsResponse is the ONVIF GetPresets SOAP response.
type GetPresetsResponse struct {
	XMLName xml.Name  `xml:"tptz:GetPresetsResponse"`
	Preset  []PTZPreset `xml:"tptz:Preset"`
}

// PTZPreset represents a preset in the GetPresets response.
type PTZPreset struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"tt:Name"`
}

// SetPresetResponse is the ONVIF SetPreset SOAP response.
type SetPresetResponse struct {
	XMLName   xml.Name `xml:"tptz:SetPresetResponse"`
	PresetToken string `xml:"tptz:PresetToken"`
}

// GotoPresetResponse is the ONVIF GotoPreset SOAP response.
type GotoPresetResponse struct {
	XMLName xml.Name `xml:"tptz:GotoPresetResponse"`
}

// RemovePresetResponse is the ONVIF RemovePreset SOAP response.
type RemovePresetResponse struct {
	XMLName xml.Name `xml:"tptz:RemovePresetResponse"`
}

// GetNodesResponse is the ONVIF GetNodes SOAP response.
type GetNodesResponse struct {
	XMLName xml.Name   `xml:"tptz:GetNodesResponse"`
	PTZNode []PTZNode `xml:"tptz:PTZNode"`
}

// PTZNode describes a PTZ node (no mechanical limits for digital PTZ).
type PTZNode struct {
	Name          string          `xml:"tt:Name"`
	FixedHomePosition bool       `xml:"tt:FixedHomePosition"`
	SupportedPTZSpaces PTZSpaces `xml:"tt:SupportedPTZSpaces"`
}

// PTZSpaces defines the supported coordinate spaces for PTZ.
type PTZSpaces struct {
	AbsolutePanTiltSpace *Space2D `xml:"AbsolutePanTiltSpace,omitempty"`
	AbsoluteZoomSpace    *Space1D `xml:"AbsoluteZoomSpace,omitempty"`
	RelativePanTiltSpace *Space2D `xml:"RelativePanTiltSpace,omitempty"`
	RelativeZoomSpace    *Space1D `xml:"RelativeZoomSpace,omitempty"`
	ContinuousPanTiltSpace *Space2D `xml:"ContinuousPanTiltSpace,omitempty"`
	ContinuousZoomSpace    *Space1D `xml:"ContinuousZoomSpace,omitempty"`
}

// Space2D defines a 2D coordinate space.
type Space2D struct {
	URI    string     `xml:"URI"`
	XRange FloatRange `xml:"XRange"`
	YRange FloatRange `xml:"YRange"`
}

// Space1D defines a 1D coordinate space.
type Space1D struct {
	URI   string    `xml:"URI"`
	XRange FloatRange `xml:"XRange"`
}


// GetConfigurationsResponse is the ONVIF GetConfigurations SOAP response.
type GetConfigurationsResponse struct {
	XMLName        xml.Name        `xml:"tptz:GetConfigurationsResponse"`
	PTZConfiguration []PTZConfiguration `xml:"tptz:PTZConfiguration"`
}

// PTZConfiguration describes a PTZ configuration.
type PTZConfiguration struct {
	Name         string        `xml:"tt:Name"`
	UseCount     int           `xml:"tt:UseCount"`
	NodeToken    string        `xml:"tt:NodeToken"`
	DefaultPTZSpeed PTZSpeed   `xml:"tt:DefaultPTZSpeed"`
	DefaultAbsolutePTZSpeed PTZSpeed `xml:"tt:DefaultAbsolutePTZSpeed"`
}

// PTZSpeed holds velocity in PTZ configuration.
type PTZSpeed struct {
	PanTilt *Vector2D `xml:"PanTilt,omitempty"`
	Zoom    *Vector1D `xml:"Zoom,omitempty"`
}

	// ---------------------------------------------------------------------------
	// PTZ request parsing types (namespace-agnostic, used with unmarshalAnyNS)
	// ---------------------------------------------------------------------------

	type continuousMoveRequest struct {
		ProfileToken string         `xml:"ProfileToken"`
		Velocity     velocityBody   `xml:"Velocity"`
	}

	type absoluteMoveRequest struct {
		ProfileToken string        `xml:"ProfileToken"`
		Position     positionBody  `xml:"Position"`
	}

	type relativeMoveRequest struct {
		ProfileToken string        `xml:"ProfileToken"`
		Translation  velocityBody  `xml:"Translation"`
	}

	type velocityBody struct {
		PanTilt *panTiltBody `xml:"PanTilt"`
		Zoom    *zoomBody    `xml:"Zoom"`
	}

	type positionBody struct {
		PanTilt *panTiltBody `xml:"PanTilt"`
		Zoom    *zoomBody    `xml:"Zoom"`
	}

	type panTiltBody struct {
		X float64 `xml:"x,attr"`
		Y float64 `xml:"y,attr"`
	}

	type zoomBody struct {
		X float64 `xml:"x,attr"`
	}

// ---------------------------------------------------------------------------
// Handler registration
// ---------------------------------------------------------------------------

// RegisterPTZHandlers registers all PTZ service handlers on the ONVIF server.
func RegisterPTZHandlers(s *Server, ptzState *ptz.State) {
	s.RegisterAction("ContinuousMove", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleContinuousMove(body, ptzState)
	})

	s.RegisterAction("AbsoluteMove", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleAbsoluteMove(body, ptzState)
	})

	s.RegisterAction("RelativeMove", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleRelativeMove(body, ptzState)
	})

	s.RegisterAction("Stop", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleStop(ptzState)
	})

	s.RegisterAction("GetStatus", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetStatus(ptzState), nil
	})

	s.RegisterAction("GetPresets", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetPresets(ptzState), nil
	})

	s.RegisterAction("SetPreset", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleSetPreset(body, ptzState)
	})

	s.RegisterAction("GotoPreset", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGotoPreset(body, ptzState)
	})

	s.RegisterAction("RemovePreset", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleRemovePreset(body, ptzState)
	})

	s.RegisterAction("GetNodes", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetNodes(), nil
	})

	s.RegisterAction("GetConfigurations", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetConfigurations(), nil
	})

	s.RegisterAction("GetConfigurationsOptions", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetConfigurationsOptions(), nil
	})
}

// ---------------------------------------------------------------------------
// SOAP action handlers
// ---------------------------------------------------------------------------


// parsePresetToken extracts preset token from SOAP body.
func parsePresetToken(body []byte) string {
	s := string(body)

	// Look for <PresetToken> element content
	tag := "PresetToken"
	start := strings.Index(s, "<"+tag)
	if start == -1 {
		start = strings.Index(s, "<tptz:"+tag)
	}
	if start == -1 {
		return ""
	}
	// Skip past the tag name
	content := s[start:]
	gt := strings.Index(content, ">")
	if gt == -1 {
		return ""
	}
	content = content[gt+1:]
	end := strings.Index(content, "<")
	if end == -1 {
		return ""
	}
	return strings.TrimSpace(content[:end])
}


func handleContinuousMove(body []byte, state *ptz.State) (interface{}, error) {
	var req continuousMoveRequest
	if err := unmarshalAnyNS(body, &req); err != nil {
		return nil, fmt.Errorf("malformed PTZ request: %w", err)
	}
	vel := ptz.Velocity{}
	if req.Velocity.PanTilt != nil {
		vel.Pan = req.Velocity.PanTilt.X
		vel.Tilt = req.Velocity.PanTilt.Y
	}
	if req.Velocity.Zoom != nil {
		vel.Zoom = req.Velocity.Zoom.X
	}
	state.ContinuousMove(vel)
	return &ContinuousMoveResponse{}, nil
}

func handleAbsoluteMove(body []byte, state *ptz.State) (interface{}, error) {
	var req absoluteMoveRequest
	if err := unmarshalAnyNS(body, &req); err != nil {
		return nil, fmt.Errorf("malformed PTZ request: %w", err)
	}
	pos := ptz.Position{}
	if req.Position.PanTilt != nil {
		pos.Pan = req.Position.PanTilt.X
		pos.Tilt = req.Position.PanTilt.Y
	}
	if req.Position.Zoom != nil {
		pos.Zoom = req.Position.Zoom.X
	}
	state.AbsoluteMove(pos)
	return &AbsoluteMoveResponse{}, nil
}

func handleRelativeMove(body []byte, state *ptz.State) (interface{}, error) {
	var req relativeMoveRequest
	if err := unmarshalAnyNS(body, &req); err != nil {
		return nil, fmt.Errorf("malformed PTZ request: %w", err)
	}
	vel := ptz.Velocity{}
	if req.Translation.PanTilt != nil {
		vel.Pan = req.Translation.PanTilt.X
		vel.Tilt = req.Translation.PanTilt.Y
	}
	if req.Translation.Zoom != nil {
		vel.Zoom = req.Translation.Zoom.X
	}
	state.RelativeMove(vel)
	return &RelativeMoveResponse{}, nil
}

func handleStop(state *ptz.State) (interface{}, error) {
	state.Stop()
	return &StopResponse{}, nil
}

func handleGetStatus(state *ptz.State) *GetStatusResponse {
	pos := state.GetPosition()
	status := state.GetStatus()

	return &GetStatusResponse{
		PTZStatus: PTZStatus{
			PanTilt: &PTZStatusVector{
				Position:   pos.Pan,
				MoveStatus: status,
			},
			Zoom: &PTZStatusVector{
				Position:   pos.Zoom,
				MoveStatus: status,
			},
		},
	}
}

func handleGetPresets(state *ptz.State) *GetPresetsResponse {
	tokens := state.GetPresets()
	presets := make([]PTZPreset, 0, len(tokens))
	for _, token := range tokens {
		presets = append(presets, PTZPreset{
			Token: token,
			Name:  token,
		})
	}
	return &GetPresetsResponse{Preset: presets}
}

func handleSetPreset(body []byte, state *ptz.State) (interface{}, error) {
	token := parsePresetToken(body)
	if token == "" {
		token = fmt.Sprintf("preset-%d", len(state.GetPresets())+1)
	}
	err := state.SetPreset(token, token)
	if err != nil {
		return nil, err
	}
	return &SetPresetResponse{PresetToken: token}, nil
}

func handleGotoPreset(body []byte, state *ptz.State) (interface{}, error) {
	token := parsePresetToken(body)
	if token == "" {
		return nil, fmt.Errorf("missing preset token")
	}
	err := state.GotoPreset(token)
	if err != nil {
		return nil, err
	}
	return &GotoPresetResponse{}, nil
}

func handleRemovePreset(body []byte, state *ptz.State) (interface{}, error) {
	token := parsePresetToken(body)
	if token == "" {
		return nil, fmt.Errorf("missing preset token")
	}
	err := state.RemovePreset(token)
	if err != nil {
		return nil, err
	}
	return &RemovePresetResponse{}, nil
}

func handleGetNodes() *GetNodesResponse {
	return &GetNodesResponse{
		PTZNode: []PTZNode{
			{
				Name:             "Main PTZ Node",
				FixedHomePosition: false,
				SupportedPTZSpaces: PTZSpaces{
					AbsolutePanTiltSpace: &Space2D{
						URI: SchemaNS + "/Polygon",
						XRange: FloatRange{Min: -1, Max: 1},
						YRange: FloatRange{Min: -1, Max: 1},
					},
					AbsoluteZoomSpace: &Space1D{
						URI: SchemaNS + "/Zoom",
						XRange: FloatRange{Min: 0, Max: 1},
					},
					RelativePanTiltSpace: &Space2D{
						URI: SchemaNS + "/Polygon",
						XRange: FloatRange{Min: -1, Max: 1},
						YRange: FloatRange{Min: -1, Max: 1},
					},
					RelativeZoomSpace: &Space1D{
						URI: SchemaNS + "/Zoom",
						XRange: FloatRange{Min: -1, Max: 1},
					},
					ContinuousPanTiltSpace: &Space2D{
						URI: SchemaNS + "/Polygon",
						XRange: FloatRange{Min: -1, Max: 1},
						YRange: FloatRange{Min: -1, Max: 1},
					},
					ContinuousZoomSpace: &Space1D{
						URI: SchemaNS + "/Zoom",
						XRange: FloatRange{Min: 0, Max: 1},
					},
				},
			},
		},
	}
}

func handleGetConfigurations() *GetConfigurationsResponse {
	return &GetConfigurationsResponse{
		PTZConfiguration: []PTZConfiguration{
			{
				Name:      "Default PTZ Configuration",
				UseCount:  1,
				NodeToken: "PTZNode_01",
				DefaultPTZSpeed: PTZSpeed{
					PanTilt: &Vector2D{X: 1, Y: 1},
					Zoom:    &Vector1D{X: 1},
				},
				DefaultAbsolutePTZSpeed: PTZSpeed{
					PanTilt: &Vector2D{X: 1, Y: 1},
					Zoom:    &Vector1D{X: 1},
				},
			},
		},
	}
}

// GetConfigurationsOptionsResponse is the ONVIF GetConfigurationsOptions response.
type GetConfigurationsOptionsResponse struct {
	XMLName xml.Name `xml:"tptz:GetConfigurationsOptionsResponse"`
	PTZConfigurationOptions PTZConfigurationOptions `xml:"tptz:PTZConfigurationOptions"`
}

// PTZConfigurationOptions describes PTZ configuration options.
type PTZConfigurationOptions struct {
	MoveStatus string `xml:"tt:MoveStatus"`
}

func handleGetConfigurationsOptions() *GetConfigurationsOptionsResponse {
	return &GetConfigurationsOptionsResponse{
		PTZConfigurationOptions: PTZConfigurationOptions{
			MoveStatus: "IDLE MOVING",
		},
	}
}
