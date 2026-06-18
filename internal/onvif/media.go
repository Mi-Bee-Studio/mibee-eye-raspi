package onvif

import (
	"context"
	"encoding/xml"
	"fmt"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
)

// ---------------------------------------------------------------------------
// Response types — GetProfiles
// ---------------------------------------------------------------------------

// GetProfilesResponse is the ONVIF GetProfiles SOAP response.
type GetProfilesResponse struct {
	XMLName  xml.Name  `xml:"GetProfilesResponse"`
	Profiles []Profile `xml:"Profiles"`
}

// Profile represents an ONVIF media profile.
// Element name is "Profiles" (plural) to match onvif-go client's expected XML tag.
type Profile struct {
	XMLName                   xml.Name                   `xml:"Profiles"`
	Token                     string                     `xml:"token,attr"`
	Name                      string                     `xml:"Name"`
	VideoSourceConfiguration  *VideoSourceConfiguration  `xml:"VideoSourceConfiguration"`
	VideoEncoderConfiguration *VideoEncoderConfiguration `xml:"VideoEncoderConfiguration"`
}

// VideoSourceConfiguration represents a video source configuration.
type VideoSourceConfiguration struct {
	Token       string `xml:"token,attr"`
	Name        string `xml:"Name"`
	SourceToken string `xml:"SourceToken"`
	UseCount    int    `xml:"UseCount"`
	Bounds      Bounds `xml:"Bounds"`
}

// Bounds defines the visible region of a video source.
// Fields are XML attributes to match onvif-go client expectations.
type Bounds struct {
	X      int `xml:"x,attr"`
	Y      int `xml:"y,attr"`
	Width  int `xml:"width,attr"`
	Height int `xml:"height,attr"`
}

// VideoEncoderConfiguration represents a video encoder configuration.
type VideoEncoderConfiguration struct {
	Token       string      `xml:"token,attr"`
	Name        string      `xml:"Name"`
	UseCount    int         `xml:"UseCount"`
	Encoding    string      `xml:"Encoding"`
	Resolution  Resolution  `xml:"Resolution"`
	RateControl RateControl `xml:"RateControl"`
}

// Resolution defines video encoder resolution.
type Resolution struct {
	Width  int `xml:"Width"`
	Height int `xml:"Height"`
}

// RateControl defines video encoder rate control settings.
type RateControl struct {
	FrameRateLimit   int `xml:"FrameRateLimit"`
	BitrateLimit     int `xml:"BitrateLimit"`
	EncodingInterval int `xml:"EncodingInterval"`
}

// ---------------------------------------------------------------------------
// Response types — GetStreamUri
// ---------------------------------------------------------------------------

// GetStreamUriResponse is the ONVIF GetStreamUri SOAP response.
//
// The XML element names MUST match what the NVR's raw SOAP fallback expects:
//   - Local name "GetStreamUriResponse" (lowercase 'r' in Uri)
//   - Child "MediaUri" containing "Uri" element
//
// This is parsed by Go encoding/xml in the NVR's getRawStreamURI():
//
//	var envelope struct {
//	    Body struct {
//	        GetStreamURIResponse struct {
//	            MediaURI struct {
//	                URI string `xml:"Uri"`
//	            } `xml:"MediaUri"`
//	        } `xml:"GetStreamUriResponse"`
//	    } `xml:"Body"`
//	}
type GetStreamUriResponse struct {
	XMLName  xml.Name `xml:"trt:GetStreamUriResponse"`
	MediaUri MediaUri `xml:"tt:MediaUri"`
}

// MediaUri holds the stream URI and its validity constraints.
type MediaUri struct {
	Uri                 string `xml:"Uri"`
	InvalidAfterConnect string `xml:"InvalidAfterConnect"`
	InvalidAfterReboot  string `xml:"InvalidAfterReboot"`
	Timeout             string `xml:"Timeout"`
}

// ---------------------------------------------------------------------------
// Response types — GetVideoSources
// ---------------------------------------------------------------------------

// GetVideoSourcesResponse is the ONVIF GetVideoSources SOAP response.
type GetVideoSourcesResponse struct {
	XMLName      xml.Name      `xml:"trt:GetVideoSourcesResponse"`
	VideoSources []VideoSource `xml:"trt:VideoSources"`
}

// VideoSource represents a physical video input source.
type VideoSource struct {
	Token string `xml:"token,attr"`
	Name  string `xml:"tt:Name"`
}

// ---------------------------------------------------------------------------
// Handler registration
// ---------------------------------------------------------------------------

// RegisterMediaHandlers registers all Media service handlers on the ONVIF server.
// RegisterMediaHandlers registers all Media service handlers on the ONVIF server.
// It reads camera and RTSP configuration from the server's config provider.
// to reach this device — the per-request client IP — so the URL is reachable
// from the NVR regardless of which interface was used.
func RegisterMediaHandlers(s *Server) {
	cfg := s.config

	s.RegisterAction("GetProfiles", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetProfiles(cfg), nil
	})

	s.RegisterAction("GetStreamUri", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetStreamUri(ctx, cfg), nil
	})

	s.RegisterAction("GetVideoSources", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return handleGetVideoSources(cfg), nil
	})
}

// handleGetProfiles returns the media profiles configured on this device.
func handleGetProfiles(cfg config.ConfigProvider) *GetProfilesResponse {
	w, h, fps, bitrate := cfg.CameraWidth(), cfg.CameraHeight(), cfg.CameraFPS(), cfg.CameraBitrate()

	return &GetProfilesResponse{
		Profiles: []Profile{{
			Name:  "main",
			Token: "main",
			VideoSourceConfiguration: &VideoSourceConfiguration{
				Name:        "VideoSourceConfig",
				Token:       "videoSrc0",
				SourceToken: "videoSrc0",
				UseCount:    1,
				Bounds: Bounds{
					Width:  w,
					Height: h,
				},
			},
			VideoEncoderConfiguration: &VideoEncoderConfiguration{
				Name:     "VideoEncoderConfig",
				Token:    "enc0",
				UseCount: 1,
				Encoding: "H264",
				Resolution: Resolution{
					Width:  w,
					Height: h,
				},
				RateControl: RateControl{
					FrameRateLimit: fps,
					BitrateLimit:   bitrate,
				},
			},
		}},
	}
}

// handleGetStreamUri returns the RTSP stream URL for the given profile.
// The IP portion of the URL is taken from the per-request context (i.e. the
// NVR's source IP), falling back to cfg.DeviceIP() when no client IP is set
// (e.g. in unit tests calling this function directly).
func handleGetStreamUri(ctx context.Context, cfg config.ConfigProvider) *GetStreamUriResponse {
	ip := ServerIPFromContext(ctx, cfg.DeviceIP())
	uri := fmt.Sprintf("rtsp://%s:%d/stream", ip, cfg.RTSPPort())

	return &GetStreamUriResponse{
		MediaUri: MediaUri{
			Uri:                 uri,
			InvalidAfterConnect: "false",
			InvalidAfterReboot:  "false",
			Timeout:             "PT0S",
		},
	}
}

// handleGetVideoSources returns the video source information.
func handleGetVideoSources(cfg config.ConfigProvider) *GetVideoSourcesResponse {
	return &GetVideoSourcesResponse{
		VideoSources: []VideoSource{{
			Token: "videoSrc0",
			Name:  "Pi Camera",
		}},
	}
}
