package onvif

import (
"context"
"encoding/xml"
	"fmt"
	"net"
"time"
)

// DeviceInfo contains device identification information for ONVIF responses.
type DeviceInfo struct {
	Name         string
	Manufacturer string
	Model        string
	Firmware     string
	HardwareID   string
	SerialNumber string
}

// ---------------------------------------------------------------------------
// Response types — GetSystemDateAndTime
// ---------------------------------------------------------------------------

// GetSystemDateAndTimeResponse is the SOAP response for GetSystemDateAndTime.
type GetSystemDateAndTimeResponse struct {
	XMLName          xml.Name         `xml:"tds:GetSystemDateAndTimeResponse"`
	SystemDateAndTime SystemDateAndTime `xml:"tds:SystemDateAndTime"`
}

// SystemDateAndTime holds the ONVIF system date/time.
type SystemDateAndTime struct {
	DateTimeType    string      `xml:"tt:DateTimeType"`
	DaylightSavings bool        `xml:"tt:DaylightSavings"`
	TimeZone        TimeZone    `xml:"tt:TimeZone"`
	UTCDateTime     UTCDateTime `xml:"tt:UTCDateTime"`
}

// TimeZone represents a time zone.
type TimeZone struct {
	TZ string `xml:"tt:TZ"`
}

// UTCDateTime holds the UTC date and time.
type UTCDateTime struct {
	Time TimeOfDay  `xml:"tt:Time"`
	Date DateOfYear `xml:"tt:Date"`
}

// TimeOfDay holds hour/minute/second.
type TimeOfDay struct {
	Hour   int `xml:"tt:Hour"`
	Minute int `xml:"tt:Minute"`
	Second int `xml:"tt:Second"`
}

// DateOfYear holds year/month/day.
type DateOfYear struct {
	Year  int `xml:"tt:Year"`
	Month int `xml:"tt:Month"`
	Day   int `xml:"tt:Day"`
}

// ---------------------------------------------------------------------------
// Response types — GetDeviceInformation
// ---------------------------------------------------------------------------

// GetDeviceInformationResponse is the SOAP response for GetDeviceInformation.
type GetDeviceInformationResponse struct {
	XMLName         xml.Name `xml:"tds:GetDeviceInformationResponse"`
	Manufacturer    string   `xml:"tds:Manufacturer"`
	Model           string   `xml:"tds:Model"`
	FirmwareVersion string   `xml:"tds:FirmwareVersion"`
	SerialNumber    string   `xml:"tds:SerialNumber"`
	HardwareId      string   `xml:"tds:HardwareId"`
}

// ---------------------------------------------------------------------------
// Response types — GetCapabilities
// ---------------------------------------------------------------------------

// GetCapabilitiesResponse is the SOAP response for GetCapabilities.
type GetCapabilitiesResponse struct {
	XMLName      xml.Name     `xml:"tds:GetCapabilitiesResponse"`
	Capabilities Capabilities `xml:"tds:Capabilities"`
}

// Capabilities holds the ONVIF device capabilities.
type Capabilities struct {
	Device *DeviceCapabilities `xml:"tt:Device,omitempty"`
	Media  *MediaCapabilities  `xml:"tt:Media,omitempty"`
	Imaging *ImagingCapabilities `xml:"tt:Imaging,omitempty"`
}

// DeviceCapabilities describes the Device service capabilities.
type DeviceCapabilities struct {
	XAddr string `xml:"tt:XAddr"`
}

// MediaCapabilities describes the Media service capabilities.
type MediaCapabilities struct {
	XAddr string `xml:"tt:XAddr"`
}

// ImagingCapabilities describes the Imaging service capabilities.
type ImagingCapabilities struct {
	XAddr string `xml:"tt:XAddr"`
}

// ---------------------------------------------------------------------------
// Response types — GetServices
// ---------------------------------------------------------------------------

// GetServicesResponse is the SOAP response for GetServices.
type GetServicesResponse struct {
	XMLName  xml.Name  `xml:"tds:GetServicesResponse"`
	Services []Service `xml:"tds:Service"`
}

// Service represents an ONVIF service entry.
type Service struct {
	Namespace string  `xml:"tds:Namespace"`
	XAddr     string  `xml:"tds:XAddr"`
	Version   Version `xml:"tds:Version"`
}

// Version holds a major/minor version pair.
type Version struct {
	Major int `xml:"tt:Major"`
	Minor int `xml:"tt:Minor"`
}

// ---------------------------------------------------------------------------
// Response types — GetScopes
// ---------------------------------------------------------------------------

// GetScopesResponse is the SOAP response for GetScopes.
type GetScopesResponse struct {
	XMLName xml.Name `xml:"tds:GetScopesResponse"`
	Scopes  []string `xml:"tt:ScopeItem"`
}

// ---------------------------------------------------------------------------
// Handler registration
// ---------------------------------------------------------------------------

// RegisterDeviceHandlers registers all Device service handlers on the ONVIF server.
//
// fallbackHost is the device's own address (e.g. "192.168.1.10:8080"). It is used
// to build XAddr URLs ONLY when the request's client IP can't be determined;
// in practice every real ONVIF client reaches us from an addressable interface,
// so handlers will prefer the per-request IP via ServerIPFromContext.
func RegisterDeviceHandlers(s *Server, fallbackHost string, info DeviceInfo) {
	fallbackIP := stripPort(fallbackHost)
	onvifPort := s.config.ONVIFPort()

	s.RegisterAction("GetSystemDateAndTime", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		now := time.Now().UTC()
		return &GetSystemDateAndTimeResponse{
			SystemDateAndTime: SystemDateAndTime{
				DateTimeType:    "Manual",
				DaylightSavings: false,
				TimeZone:        TimeZone{TZ: "UTC"},
				UTCDateTime: UTCDateTime{
					Time: TimeOfDay{
						Hour:   now.Hour(),
						Minute: now.Minute(),
						Second: now.Second(),
					},
					Date: DateOfYear{
						Year:  now.Year(),
						Month: int(now.Month()),
						Day:   now.Day(),
					},
				},
			},
		}, nil
	})

	s.RegisterAction("GetDeviceInformation", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return &GetDeviceInformationResponse{
			Manufacturer:    info.Manufacturer,
			Model:           info.Model,
			FirmwareVersion: info.Firmware,
			SerialNumber:    info.SerialNumber,
			HardwareId:      info.HardwareID,
		}, nil
	})

	s.RegisterAction("GetCapabilities", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		baseURL := baseURLForRequest(ctx, fallbackIP, onvifPort)
		return &GetCapabilitiesResponse{
			Capabilities: Capabilities{
				Device: &DeviceCapabilities{
					XAddr: baseURL + "/device_service",
				},
				Media: &MediaCapabilities{
					XAddr: baseURL + "/media_service",
				},
				Imaging: &ImagingCapabilities{
					XAddr: baseURL + "/device_service",
				},
			},
		}, nil
	})

	s.RegisterAction("GetServices", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		baseURL := baseURLForRequest(ctx, fallbackIP, onvifPort)
return &GetServicesResponse{
Services: []Service{
{
Namespace: ONVIFDeviceNS,
XAddr:     baseURL + "/device_service",
Version:   Version{Major: 1, Minor: 0},
},
{
Namespace: ONVIFMediaNS,
XAddr:     baseURL + "/media_service",
Version:   Version{Major: 1, Minor: 0},
},
{
Namespace: ONVIFImgNS,
XAddr:     baseURL + "/device_service",
Version:   Version{Major: 1, Minor: 0},
},
},
}, nil
})

	s.RegisterAction("GetScopes", func(ctx context.Context, body []byte, auth *AuthResult) (interface{}, error) {
		return &GetScopesResponse{
			Scopes: []string{
				"onvif://www.onvif.org/type/video_encoder",
				fmt.Sprintf("onvif://www.onvif.org/name/%s", info.Name),
				fmt.Sprintf("onvif://www.onvif.org/hardware/%s", info.HardwareID),
			},
		}, nil
	})
}

// baseURLForRequest returns "http://<ip>:<port>/onvif" using the per-request
// client IP if available, otherwise the fallback.
func baseURLForRequest(ctx context.Context, fallbackIP string, port int) string {
	ip := ServerIPFromContext(ctx, fallbackIP)
	return fmt.Sprintf("http://%s:%d/onvif", ip, port)
}

// stripPort removes an optional :port suffix from a host string.
func stripPort(host string) string {
	if host == "" {
		return ""
	}
	if h, _, err := net.SplitHostPort(host); err == nil {
		return h
	}
	return host
}
