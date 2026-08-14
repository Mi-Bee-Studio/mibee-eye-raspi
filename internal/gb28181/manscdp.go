// Package gb28181 implements MANSCDP (XML body) command handling —
// device catalog, keepalive, and control command XML between the
// device and the GB/T 28181 platform.
package gb28181

import (
	"encoding/xml"
	"log/slog"
)

// Query represents a MANSCDP Query request.
type Query struct {
	XMLName  xml.Name `xml:"Query"`
	CmdType  string   `xml:"CmdType,attr"`
	SN       string   `xml:"SN,attr"`
	DeviceID string   `xml:"DeviceID"`
}

// Response represents a MANSCDP Response message.
type Response struct {
	XMLName    xml.Name    `xml:"Response"`
	CmdType    string      `xml:"CmdType,attr"`
	SN         string      `xml:"SN,attr"`
	DeviceID   string      `xml:"DeviceID"`
	SumNum     *int        `xml:"SumNum,omitempty"`
	DeviceList *DeviceList `xml:"DeviceList,omitempty"`
	Device     *DeviceItem `xml:"Device,omitempty"`
}

// DeviceList contains a list of device/channel items.
type DeviceList struct {
	XMLName xml.Name      `xml:"DeviceList"`
	Item    []ChannelItem `xml:"Item"`
}

// ChannelItem represents a device channel catalog entry.
// ALL mandatory fields per GB/T 28181-2022 Annex A.2.1
type ChannelItem struct {
	DeviceID     string  `xml:"DeviceID"`
	Name         string  `xml:"Name"`
	Manufacturer string  `xml:"Manufacturer"`
	Model        string  `xml:"Model"`
	Owner        string  `xml:"Owner"`
	CivilCode    string  `xml:"CivilCode"`
	Address      string  `xml:"Address"`
	Parental     int     `xml:"Parental"`
	ParentID     string  `xml:"ParentID"`
	SafetyWay    int     `xml:"SafetyWay"`
	RegisterWay  int     `xml:"RegisterWay"`
	Secrecy      int     `xml:"Secrecy"`
	Status       string  `xml:"Status"`
	IPAddress    string  `xml:"IPAddress"`
	Port         int     `xml:"Port"`
	Longitude    float64 `xml:"Longitude"`
	Latitude     float64 `xml:"Latitude"`
}

// DeviceItem represents basic device information.
type DeviceItem struct {
	DeviceID     string `xml:"DeviceID"`
	Name         string `xml:"Name"`
	Manufacturer string `xml:"Manufacturer"`
	Model        string `xml:"Model"`
	Firmware     string `xml:"Firmware"`
}

// Notify represents a MANSCDP Notify message.
type Notify struct {
	XMLName  xml.Name `xml:"Notify"`
	CmdType  string   `xml:"CmdType,attr"`
	SN       string   `xml:"SN,attr"`
	DeviceID string   `xml:"DeviceID"`
	Status   string   `xml:"Status,omitempty"`
}

// BuildCatalogResponseMessage creates a SIP MESSAGE with Catalog response.
func BuildCatalogResponseMessage(sn, deviceID string, items []ChannelItem) SipMessage {
	sumNum := len(items)
	resp := Response{
		CmdType:  "Catalog",
		SN:       sn,
		DeviceID: deviceID,
		SumNum:   &sumNum,
		DeviceList: &DeviceList{
			Item: items,
		},
	}
	xmlData, err := xml.Marshal(resp)
	if err != nil {
		// This is a programming error — log and return empty message
		slog.Error("Failed to marshal Catalog response", "error", err)
		return SipMessage{}
	}
	return SipMessage{
		Method:      "MESSAGE",
		ContentType: "Application/MANSCDP+xml",
		Body:        string(xmlData),
		UserAgent:   "MiBee-GB28181/1.0",
		Headers:     make(map[string]string),
	}
}

// BuildDeviceInfoResponseMessage creates a SIP MESSAGE with DeviceInfo response.
func BuildDeviceInfoResponseMessage(sn, deviceID string, info DeviceItem) SipMessage {
	resp := Response{
		CmdType:  "DeviceInfo",
		SN:       sn,
		DeviceID: deviceID,
		Device:   &info,
	}
	xmlData, err := xml.Marshal(resp)
	if err != nil {
		slog.Error("Failed to marshal DeviceInfo response", "error", err)
		return SipMessage{}
	}
	return SipMessage{
		Method:      "MESSAGE",
		ContentType: "Application/MANSCDP+xml",
		Body:        string(xmlData),
		UserAgent:   "MiBee-GB28181/1.0",
		Headers:     make(map[string]string),
	}
}

// BuildKeepaliveMessage creates a SIP MESSAGE with Keepalive notify.
func BuildKeepaliveMessage(sn, deviceID, status string) SipMessage {
	if status == "" {
		status = "OK"
	}
	notify := Notify{
		CmdType:  "Keepalive",
		SN:       sn,
		DeviceID: deviceID,
		Status:   status,
	}
	xmlData, err := xml.Marshal(notify)
	if err != nil {
		slog.Error("Failed to marshal Keepalive message", "error", err)
		return SipMessage{}
	}
	return SipMessage{
		Method:      "MESSAGE",
		ContentType: "Application/MANSCDP+xml",
		Body:        string(xmlData),
		UserAgent:   "MiBee-GB28181/1.0",
		Headers:     make(map[string]string),
	}
}

// DispatchInboundMessage parses inbound MANSCDP XML and returns appropriate responses.
// Returns (ok200, queued_response_or_nil, error).
func DispatchInboundMessage(msg SipMessage) (SipMessage, *SipMessage, error) {
	if msg.Body == "" {
		// No body to parse — just acknowledge
		return SipMessage{}, nil, nil
	}

	// Try to parse as Query (Catalog, DeviceInfo)
	var query Query
	if err := xml.Unmarshal([]byte(msg.Body), &query); err == nil {
		switch query.CmdType {
		case "Catalog":
			// Return 200 OK + queue Catalog response
			ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
			catalogResp := BuildCatalogResponseMessage(query.SN, query.DeviceID, []ChannelItem{})
			return ok200, &catalogResp, nil
		case "DeviceInfo":
			// Return 200 OK + queue DeviceInfo response
			ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
			deviceInfo := DeviceItem{DeviceID: query.DeviceID, Name: "MiBee Eye", Manufacturer: "MiBee", Model: "Eye-RPi", Firmware: "1.0"}
			deviceInfoResp := BuildDeviceInfoResponseMessage(query.SN, query.DeviceID, deviceInfo)
			return ok200, &deviceInfoResp, nil
		default:
			slog.Warn("Unknown Query CmdType", "cmdtype", query.CmdType)
			ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
			return ok200, nil, nil
		}
	}

	// Try to parse as Notify (Keepalive ack from platform)
	var notify Notify
	if err := xml.Unmarshal([]byte(msg.Body), &notify); err == nil {
		switch notify.CmdType {
		case "Keepalive":
			// Keepalive ack from platform — just 200 OK, no queued response
			ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
			return ok200, nil, nil
		default:
			slog.Warn("Unknown Notify CmdType", "cmdtype", notify.CmdType)
			ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
			return ok200, nil, nil
		}
	}

	// Unknown/unparseable body — log warning and return 200 OK (graceful degradation)
	slog.Warn("Failed to parse MANSCDP XML body", "body", msg.Body)
	ok200 := Build200OK(msg.RequestURI, msg.To, msg.From, msg.CallID, msg.CSeq, msg.Contact, "", "")
	return ok200, nil, nil
}
