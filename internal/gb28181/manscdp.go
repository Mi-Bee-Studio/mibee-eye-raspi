// Package gb28181 implements MANSCDP (XML body) command handling —
// device catalog, keepalive, and control command XML between the
// device and the GB/T 28181 platform.
package gb28181

import (
	"encoding/xml"
	"fmt"
	"log/slog"
	"time"
)

// Query represents a MANSCDP Query request.
type Query struct {
	XMLName  xml.Name `xml:"Query"`
	CmdType  string   `xml:"CmdType,attr"`
	SN       string   `xml:"SN,attr"`
	DeviceID string   `xml:"DeviceID"`
}

// QueryElem represents a MANSCDP Query request with child-element format (for NVR compatibility).
type QueryElem struct {
	XMLName  xml.Name `xml:"Query"`
	CmdType  string   `xml:"CmdType"`
	SN       string   `xml:"SN"`
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

// NotifyElem represents a MANSCDP Notify message with child-element format (for NVR compatibility).
type NotifyElem struct {
	XMLName  xml.Name `xml:"Notify"`
	CmdType  string   `xml:"CmdType"`
	SN       string   `xml:"SN"`
	DeviceID string   `xml:"DeviceID"`
	Status   string   `xml:"Status,omitempty"`
}

// parseQueryDual tries to parse a Query body in both attribute and child-element format.
// Returns the normalized Query struct and a bool indicating success.
func parseQueryDual(body string) (Query, bool) {
	// Try attribute format first (existing struct)
	var query Query
	if err := xml.Unmarshal([]byte(body), &query); err == nil && query.CmdType != "" {
		return query, true
	}
	// Try child-element format
	var queryElem QueryElem
	if err := xml.Unmarshal([]byte(body), &queryElem); err == nil && queryElem.CmdType != "" {
		// Normalize to Query struct
		return Query{
			CmdType:  queryElem.CmdType,
			SN:       queryElem.SN,
			DeviceID: queryElem.DeviceID,
		}, true
	}
	return Query{}, false
}

// parseNotifyDual tries to parse a Notify body in both attribute and child-element format.
// Returns the normalized Notify struct and a bool indicating success.
func parseNotifyDual(body string) (Notify, bool) {
	// Try attribute format first (existing struct)
	var notify Notify
	if err := xml.Unmarshal([]byte(body), &notify); err == nil && notify.CmdType != "" {
		return notify, true
	}
	// Try child-element format
	var notifyElem NotifyElem
	if err := xml.Unmarshal([]byte(body), &notifyElem); err == nil && notifyElem.CmdType != "" {
		// Normalize to Notify struct
		return Notify{
			CmdType:  notifyElem.CmdType,
			SN:       notifyElem.SN,
			DeviceID: notifyElem.DeviceID,
			Status:   notifyElem.Status,
		}, true
	}
	return Notify{}, false
}

// DeviceContext provides device identity and network context for MANSCDP responses.
type DeviceContext struct {
	DeviceID     string
	ChannelID    string
	Name         string
	Manufacturer string
	Model        string
	Firmware     string
	LocalIP      string
	LocalPort    int
}

// buildChannel creates a ChannelItem from DeviceContext.
func buildChannel(dev DeviceContext) ChannelItem {
	return ChannelItem{
		DeviceID:     dev.ChannelID,
		Name:         dev.ChannelID,
		Status:       "ON",
		Manufacturer: dev.Manufacturer,
		Model:        dev.Model,
		Owner:        dev.DeviceID,
		ParentID:     dev.DeviceID,
		Secrecy:      0,
		RegisterWay:  1, // platform register
		IPAddress:    dev.LocalIP,
		Port:         dev.LocalPort,
	}
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

// BuildRecordInfoResponseMessage creates a SIP MESSAGE with RecordInfo response (empty, no recordings).
func BuildRecordInfoResponseMessage(sn, deviceID string) SipMessage {
	type RecordInfoResponse struct {
		XMLName    xml.Name `xml:"Response"`
		CmdType    string   `xml:"CmdType,attr"`
		SN         string   `xml:"SN,attr"`
		DeviceID   string   `xml:"DeviceID"`
		Name       string   `xml:"Name"`
		SumNum     *int     `xml:"SumNum"`
		RecordList struct {
			Num int `xml:"Num,attr"`
		} `xml:"RecordList"`
	}
	sumNum := 0
	resp := RecordInfoResponse{
		CmdType:  "RecordInfo",
		SN:       sn,
		DeviceID: deviceID,
		Name:     "RecordInfo",
		SumNum:   &sumNum,
		RecordList: struct {
			Num int `xml:"Num,attr"`
		}{Num: 0},
	}
	xmlData, err := xml.Marshal(resp)
	if err != nil {
		slog.Error("Failed to marshal RecordInfo response", "error", err)
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

// BuildDeviceStatusResponseMessage creates a SIP MESSAGE with DeviceStatus response.
func BuildDeviceStatusResponseMessage(sn, deviceID string) SipMessage {
	body := fmt.Sprintf(`<Response CmdType="DeviceStatus" SN="%s"><DeviceID>%s</DeviceID><Result>OK</Result><Online>ONLINE</Online><Status>OK</Status><Encode>ON</Encode><Record>OFF</Record><DeviceTime>%s</DeviceTime></Response>`, sn, deviceID, time.Now().Format("2006-01-02T15:04:05"))
	return SipMessage{
		Method:      "MESSAGE",
		ContentType: "Application/MANSCDP+xml",
		Body:        body,
		UserAgent:   "MiBee-GB28181/1.0",
		Headers:     make(map[string]string),
	}
}

// BuildControlRejectResponseMessage creates a SIP MESSAGE with control rejection response.
func BuildControlRejectResponseMessage(cmdType, sn, deviceID string) SipMessage {
	body := fmt.Sprintf(`<Response CmdType="%s" SN="%s"><DeviceID>%s</DeviceID><Result>ERROR</Result></Response>`, cmdType, sn, deviceID)
	return SipMessage{
		Method:      "MESSAGE",
		ContentType: "Application/MANSCDP+xml",
		Body:        body,
		UserAgent:   "MiBee-GB28181/1.0",
		Headers:     make(map[string]string),
	}
}

// DispatchInboundMessage parses inbound MANSCDP XML and returns appropriate responses.
// Returns (ok200, queued_response_or_nil, error).
func DispatchInboundMessage(msg SipMessage, dev DeviceContext) (SipMessage, *SipMessage, error) {
	if msg.Body == "" {
		// No body to parse — just acknowledge
		return SipMessage{}, nil, nil
	}

	// Try to parse as Query (Catalog, DeviceInfo, RecordInfo, DeviceStatus, Control commands)
	if query, ok := parseQueryDual(msg.Body); ok {
		switch query.CmdType {
		case "Catalog":
			// Return 200 OK + queue Catalog response with real channel data
			ok200 := Build200OK(msg, "", "")
			channel := buildChannel(dev)
			catalogResp := BuildCatalogResponseMessage(query.SN, query.DeviceID, []ChannelItem{channel})
			return ok200, &catalogResp, nil
		case "DeviceInfo":
			// Return 200 OK + queue DeviceInfo response from dev context
			ok200 := Build200OK(msg, "", "")
			deviceInfo := DeviceItem{
				DeviceID:     dev.DeviceID,
				Name:         dev.Name,
				Manufacturer: dev.Manufacturer,
				Model:        dev.Model,
				Firmware:     dev.Firmware,
			}
			deviceInfoResp := BuildDeviceInfoResponseMessage(query.SN, query.DeviceID, deviceInfo)
			return ok200, &deviceInfoResp, nil
		case "RecordInfo":
			// Return 200 OK + queue empty RecordInfo response
			ok200 := Build200OK(msg, "", "")
			recordInfoResp := BuildRecordInfoResponseMessage(query.SN, query.DeviceID)
			return ok200, &recordInfoResp, nil
		case "DeviceStatus":
			// Return 200 OK + queue DeviceStatus response
			ok200 := Build200OK(msg, "", "")
			deviceStatusResp := BuildDeviceStatusResponseMessage(query.SN, query.DeviceID)
			return ok200, &deviceStatusResp, nil
		case "DeviceControl", "Broadcast", "DeviceConfig", "HomePosition":
			// Return 200 OK + queue ControlReject response (control not supported)
			slog.Warn("Control command not supported", "cmdtype", query.CmdType)
			ok200 := Build200OK(msg, "", "")
			controlReject := BuildControlRejectResponseMessage(query.CmdType, query.SN, query.DeviceID)
			return ok200, &controlReject, nil
		default:
			slog.Warn("Unknown Query CmdType", "cmdtype", query.CmdType)
			ok200 := Build200OK(msg, "", "")
			return ok200, nil, nil
		}
	}

	// Try to parse as Notify (Keepalive ack from platform)
	if notify, ok := parseNotifyDual(msg.Body); ok {
		switch notify.CmdType {
		case "Keepalive":
			// Keepalive ack from platform — just 200 OK, no queued response
			ok200 := Build200OK(msg, "", "")
			return ok200, nil, nil
		default:
			slog.Warn("Unknown Notify CmdType", "cmdtype", notify.CmdType)
			ok200 := Build200OK(msg, "", "")
			return ok200, nil, nil
		}
	}

	// Unknown/unparseable body — log warning and return 200 OK (graceful degradation)
	slog.Warn("Failed to parse MANSCDP XML body", "body", msg.Body)
	ok200 := Build200OK(msg, "", "")
	return ok200, nil, nil
}
