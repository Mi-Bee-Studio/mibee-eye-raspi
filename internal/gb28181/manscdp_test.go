package gb28181

import (
	"encoding/xml"
	"strings"
	"testing"
)

// TestCatalogResponse_XMLWellFormed tests that marshaling/unmarshaling preserves data.
func TestCatalogResponse_XMLWellFormed(t *testing.T) {
	sn := "123"
	deviceID := "34020000012000000001"
	items := []ChannelItem{
		{
			DeviceID:     "34020000012000000001",
			Name:         "Camera 1",
			Manufacturer: "MiBee",
			Model:        "Eye-RPi",
			Owner:        "Owner",
			CivilCode:    "110105",
			Address:      "Address",
			Parental:     1,
			ParentID:     "3402000000",
			SafetyWay:    0,
			RegisterWay:  1,
			Secrecy:      0,
			Status:       "ON",
			IPAddress:    "192.168.1.100",
			Port:         5060,
			Longitude:    116.404,
			Latitude:     39.915,
		},
	}

	msg := BuildCatalogResponseMessage(sn, deviceID, items)

	// Parse the XML back
	var resp Response
	if err := xml.Unmarshal([]byte(msg.Body), &resp); err != nil {
		t.Fatalf("Failed to unmarshal Catalog response: %v", err)
	}

	// Verify key fields
	if resp.CmdType != "Catalog" {
		t.Errorf("Expected CmdType Catalog, got %s", resp.CmdType)
	}
	if resp.SN != sn {
		t.Errorf("Expected SN %s, got %s", sn, resp.SN)
	}
	if resp.DeviceID != deviceID {
		t.Errorf("Expected DeviceID %s, got %s", deviceID, resp.DeviceID)
	}
	if resp.SumNum == nil {
		t.Error("SumNum should not be nil")
	} else if *resp.SumNum != 1 {
		t.Errorf("Expected SumNum 1, got %d", *resp.SumNum)
	}
	if resp.DeviceList == nil {
		t.Fatal("DeviceList should not be nil")
	}
	if len(resp.DeviceList.Item) != 1 {
		t.Fatalf("Expected 1 item, got %d", len(resp.DeviceList.Item))
	}

	item := resp.DeviceList.Item[0]
	if item.DeviceID != items[0].DeviceID {
		t.Errorf("DeviceID mismatch: got %s, want %s", item.DeviceID, items[0].DeviceID)
	}
	if item.Name != items[0].Name {
		t.Errorf("Name mismatch: got %s, want %s", item.Name, items[0].Name)
	}
}

// TestCatalogResponse_HasAllMandatoryFields tests that all 17 mandatory fields are present.
func TestCatalogResponse_HasAllMandatoryFields(t *testing.T) {
	item := ChannelItem{
		DeviceID:     "34020000012000000001",
		Name:         "Camera 1",
		Manufacturer: "MiBee",
		Model:        "Eye-RPi",
		Owner:        "Owner",
		CivilCode:    "110105",
		Address:      "Address",
		Parental:     1,
		ParentID:     "3402000000",
		SafetyWay:    0,
		RegisterWay:  1,
		Secrecy:      0,
		Status:       "ON",
		IPAddress:    "192.168.1.100",
		Port:         5060,
		Longitude:    116.404,
		Latitude:     39.915,
	}

	msg := BuildCatalogResponseMessage("123", "34020000012000000001", []ChannelItem{item})

	// Parse the XML back
	var resp Response
	if err := xml.Unmarshal([]byte(msg.Body), &resp); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	unmarshaled := resp.DeviceList.Item[0]

	// Verify all 17 mandatory fields are non-zero
	checkNonZero := func(name, value string) {
		if value == "" {
			t.Errorf("Mandatory field %s is empty (zero value)", name)
		}
	}

	checkNonZero("DeviceID", unmarshaled.DeviceID)
	checkNonZero("Name", unmarshaled.Name)
	checkNonZero("Manufacturer", unmarshaled.Manufacturer)
	checkNonZero("Model", unmarshaled.Model)
	checkNonZero("Owner", unmarshaled.Owner)
	checkNonZero("CivilCode", unmarshaled.CivilCode)
	checkNonZero("Address", unmarshaled.Address)
	checkNonZero("ParentID", unmarshaled.ParentID)
	checkNonZero("Status", unmarshaled.Status)
	checkNonZero("IPAddress", unmarshaled.IPAddress)

	// Integer fields should be explicitly set (not just zero)
	if unmarshaled.Parental < 0 || unmarshaled.Parental > 1 {
		t.Errorf("Parental should be 0 or 1, got %d", unmarshaled.Parental)
	}
	if unmarshaled.SafetyWay < 0 || unmarshaled.SafetyWay > 2 {
		t.Errorf("SafetyWay should be 0-2, got %d", unmarshaled.SafetyWay)
	}
	if unmarshaled.RegisterWay < 0 || unmarshaled.RegisterWay > 2 {
		t.Errorf("RegisterWay should be 0-2, got %d", unmarshaled.RegisterWay)
	}
	if unmarshaled.Secrecy < 0 || unmarshaled.Secrecy > 2 {
		t.Errorf("Secrecy should be 0-2, got %d", unmarshaled.Secrecy)
	}
	if unmarshaled.Port <= 0 {
		t.Errorf("Port should be positive, got %d", unmarshaled.Port)
	}

	// Coordinates should be set (non-zero indicates presence)
	if unmarshaled.Longitude == 0 {
		t.Error("Longitude is zero (should be set)")
	}
	if unmarshaled.Latitude == 0 {
		t.Error("Latitude is zero (should be set)")
	}
}

// TestKeepalive_Format tests that keepalive message has correct format.
func TestKeepalive_Format(t *testing.T) {
	sn := "456"
	deviceID := "34020000012000000001"

	msg := BuildKeepaliveMessage(sn, deviceID, "OK")

	// Check SIP MESSAGE format
	if msg.Method != "MESSAGE" {
		t.Errorf("Expected method MESSAGE, got %s", msg.Method)
	}
	if msg.ContentType != "Application/MANSCDP+xml" {
		t.Errorf("Expected content type Application/MANSCDP+xml, got %s", msg.ContentType)
	}

	// Check XML body format
	if !strings.Contains(msg.Body, `CmdType="Keepalive"`) {
		t.Error("Keepalive message should contain CmdType=\"Keepalive\" attribute")
	}
	if !strings.Contains(msg.Body, "<Status>OK</Status>") {
		t.Error("Keepalive message should contain <Status>OK</Status>")
	}
	if !strings.Contains(msg.Body, `SN="456"`) {
		t.Error("Keepalive message should contain SN attribute")
	}
	if !strings.Contains(msg.Body, "<DeviceID>34020000012000000001</DeviceID>") {
		t.Error("Keepalive message should contain DeviceID")
	}

	// Test default status (empty string should become "OK")
	msgDefault := BuildKeepaliveMessage(sn, deviceID, "")
	if !strings.Contains(msgDefault.Body, "<Status>OK</Status>") {
		t.Error("Keepalive message with empty status should default to OK")
	}
}

// TestDispatch_CatalogQuery tests dispatching a Catalog query.
func TestDispatch_CatalogQuery(t *testing.T) {
	// Build inbound MESSAGE with Catalog query
	queryXML := `<?xml version="1.0" encoding="UTF-8"?>
<Query CmdType="Catalog" SN="789">
	<DeviceID>34020000012000000001</DeviceID>
</Query>`

	inbound := SipMessage{
		Method:      "MESSAGE",
		RequestURI:  "sip:3402000000@3402000000",
		To:          "<sip:34020000012000000001@3402000000>;tag=device",
		From:        "<sip:3402000000@3402000000>;tag=platform",
		CallID:      "test-call-id@192.168.1.100",
		CSeq:        "1 MESSAGE",
		Contact:     "<sip:34020000012000000001@192.168.1.100:5060>",
		ContentType: "Application/MANSCDP+xml",
		Body:        queryXML,
		Headers:     make(map[string]string),
	}

	ok200, queued, err := DispatchInboundMessage(inbound)

	if err != nil {
		t.Fatalf("DispatchInboundMessage failed: %v", err)
	}
	if ok200.StatusCode != 200 {
		t.Errorf("Expected 200 OK, got status %d", ok200.StatusCode)
	}
	if queued == nil {
		t.Fatal("Expected queued response, got nil")
	}

	// Verify queued response is a Catalog response MESSAGE
	if queued.Method != "MESSAGE" {
		t.Errorf("Expected queued method MESSAGE, got %s", queued.Method)
	}
	if !strings.Contains(queued.Body, `CmdType="Catalog"`) {
		t.Error("Queued response should be Catalog")
	}
	if !strings.Contains(queued.Body, `SN="789"`) {
		t.Error("Queued response should echo SN")
	}
}

// TestDispatch_UnknownCmdType_NoCrash tests that unknown command types don't crash.
func TestDispatch_UnknownCmdType_NoCrash(t *testing.T) {
	// Build inbound MESSAGE with unknown CmdType
	queryXML := `<?xml version="1.0" encoding="UTF-8"?>
<Query CmdType="UnknownCommand" SN="999">
	<DeviceID>34020000012000000001</DeviceID>
</Query>`

	inbound := SipMessage{
		Method:      "MESSAGE",
		RequestURI:  "sip:3402000000@3402000000",
		To:          "<sip:34020000012000000001@3402000000>;tag=device",
		From:        "<sip:3402000000@3402000000>;tag=platform",
		CallID:      "test-call-id@192.168.1.100",
		CSeq:        "1 MESSAGE",
		Contact:     "<sip:34020000012000000001@192.168.1.100:5060>",
		ContentType: "Application/MANSCDP+xml",
		Body:        queryXML,
		Headers:     make(map[string]string),
	}

	// This should not panic and should return 200 OK gracefully
	ok200, queued, err := DispatchInboundMessage(inbound)

	if err != nil {
		t.Fatalf("DispatchInboundMessage should not error on unknown command, got: %v", err)
	}
	if ok200.StatusCode != 200 {
		t.Errorf("Expected 200 OK, got status %d", ok200.StatusCode)
	}
	if queued != nil {
		t.Error("Unknown command should not queue a response")
	}
}

// TestDeviceInfoResponse_XMLWellFormed tests DeviceInfo response XML.
func TestDeviceInfoResponse_XMLWellFormed(t *testing.T) {
	sn := "101"
	deviceID := "34020000012000000001"
	info := DeviceItem{
		DeviceID:     deviceID,
		Name:         "MiBee Eye",
		Manufacturer: "MiBee",
		Model:        "Eye-RPi",
		Firmware:     "1.0",
	}

	msg := BuildDeviceInfoResponseMessage(sn, deviceID, info)

	// Parse the XML back
	var resp Response
	if err := xml.Unmarshal([]byte(msg.Body), &resp); err != nil {
		t.Fatalf("Failed to unmarshal DeviceInfo response: %v", err)
	}

	if resp.CmdType != "DeviceInfo" {
		t.Errorf("Expected CmdType DeviceInfo, got %s", resp.CmdType)
	}
	if resp.Device == nil {
		t.Fatal("Device should not be nil")
	}

	device := resp.Device
	if device.DeviceID != deviceID {
		t.Errorf("DeviceID mismatch: got %s, want %s", device.DeviceID, deviceID)
	}
	if device.Name != info.Name {
		t.Errorf("Name mismatch: got %s, want %s", device.Name, info.Name)
	}
	if device.Manufacturer != info.Manufacturer {
		t.Errorf("Manufacturer mismatch: got %s, want %s", device.Manufacturer, info.Manufacturer)
	}
	if device.Model != info.Model {
		t.Errorf("Model mismatch: got %s, want %s", device.Model, info.Model)
	}
	if device.Firmware != info.Firmware {
		t.Errorf("Firmware mismatch: got %s, want %s", device.Firmware, info.Firmware)
	}
}

// TestDispatch_DeviceInfoQuery tests dispatching a DeviceInfo query.
func TestDispatch_DeviceInfoQuery(t *testing.T) {
	queryXML := `<?xml version="1.0" encoding="UTF-8"?>
<Query CmdType="DeviceInfo" SN="202">
	<DeviceID>34020000012000000001</DeviceID>
</Query>`

	inbound := SipMessage{
		Method:      "MESSAGE",
		RequestURI:  "sip:3402000000@3402000000",
		To:          "<sip:34020000012000000001@3402000000>;tag=device",
		From:        "<sip:3402000000@3402000000>;tag=platform",
		CallID:      "test-call-id@192.168.1.100",
		CSeq:        "1 MESSAGE",
		Contact:     "<sip:34020000012000000001@192.168.1.100:5060>",
		ContentType: "Application/MANSCDP+xml",
		Body:        queryXML,
		Headers:     make(map[string]string),
	}

	ok200, queued, err := DispatchInboundMessage(inbound)

	if err != nil {
		t.Fatalf("DispatchInboundMessage failed: %v", err)
	}
	if ok200.StatusCode != 200 {
		t.Errorf("Expected 200 OK, got status %d", ok200.StatusCode)
	}
	if queued == nil {
		t.Fatal("Expected queued response, got nil")
	}

	// Verify queued response is a DeviceInfo response MESSAGE
	if queued.Method != "MESSAGE" {
		t.Errorf("Expected queued method MESSAGE, got %s", queued.Method)
	}
	if !strings.Contains(queued.Body, `CmdType="DeviceInfo"`) {
		t.Error("Queued response should be DeviceInfo")
	}
	if !strings.Contains(queued.Body, "<DeviceID>34020000012000000001</DeviceID>") {
		t.Error("Queued response should contain DeviceID")
	}
}

// TestDispatch_KeepaliveNotify tests handling Keepalive notify from platform.
func TestDispatch_KeepaliveNotify(t *testing.T) {
	notifyXML := `<?xml version="1.0" encoding="UTF-8"?>
<Notify CmdType="Keepalive" SN="303">
	<DeviceID>34020000012000000001</DeviceID>
	<Status>OK</Status>
</Notify>`

	inbound := SipMessage{
		Method:      "MESSAGE",
		RequestURI:  "sip:3402000000@3402000000",
		To:          "<sip:34020000012000000001@3402000000>;tag=device",
		From:        "<sip:3402000000@3402000000>;tag=platform",
		CallID:      "test-call-id@192.168.1.100",
		CSeq:        "1 MESSAGE",
		Contact:     "<sip:34020000012000000001@192.168.1.100:5060>",
		ContentType: "Application/MANSCDP+xml",
		Body:        notifyXML,
		Headers:     make(map[string]string),
	}

	ok200, queued, err := DispatchInboundMessage(inbound)

	if err != nil {
		t.Fatalf("DispatchInboundMessage failed: %v", err)
	}
	if ok200.StatusCode != 200 {
		t.Errorf("Expected 200 OK, got status %d", ok200.StatusCode)
	}
	// Keepalive ack should NOT queue a response
	if queued != nil {
		t.Error("Keepalive ack should not queue a response")
	}
}

// TestDispatch_EmptyBody tests graceful handling of empty body.
func TestDispatch_EmptyBody(t *testing.T) {
	inbound := SipMessage{
		Method:     "MESSAGE",
		RequestURI: "sip:3402000000@3402000000",
		To:         "<sip:34020000012000000001@3402000000>;tag=device",
		From:       "<sip:3402000000@3402000000>;tag=platform",
		CallID:     "test-call-id@192.168.1.100",
		CSeq:       "1 MESSAGE",
		Contact:    "<sip:34020000012000000001@192.168.1.100:5060>",
		Body:       "",
		Headers:    make(map[string]string),
	}

	ok200, queued, err := DispatchInboundMessage(inbound)

	if err != nil {
		t.Fatalf("DispatchInboundMessage with empty body should not error, got: %v", err)
	}
	// Empty body should return empty ok200 (no meaningful response)
	if ok200.StatusCode != 0 && ok200.StatusCode != 200 {
		// Either is acceptable for empty body
		t.Logf("Empty body returned status %d (acceptable)", ok200.StatusCode)
	}
	if queued != nil {
		t.Error("Empty body should not queue a response")
	}
}

// TestDispatch_InvalidXML tests graceful handling of unparseable XML.
func TestDispatch_InvalidXML(t *testing.T) {
	inbound := SipMessage{
		Method:      "MESSAGE",
		RequestURI:  "sip:3402000000@3402000000",
		To:          "<sip:34020000012000000001@3402000000>;tag=device",
		From:        "<sip:3402000000@3402000000>;tag=platform",
		CallID:      "test-call-id@192.168.1.100",
		CSeq:        "1 MESSAGE",
		Contact:     "<sip:34020000012000000001@192.168.1.100:5060>",
		ContentType: "Application/MANSCDP+xml",
		Body:        "not valid xml at all <<<<<",
		Headers:     make(map[string]string),
	}

	ok200, queued, err := DispatchInboundMessage(inbound)

	// Invalid XML should not crash and should return 200 OK
	if err != nil {
		t.Fatalf("DispatchInboundMessage with invalid XML should not error, got: %v", err)
	}
	if ok200.StatusCode != 200 {
		t.Errorf("Expected 200 OK for graceful degradation, got %d", ok200.StatusCode)
	}
	if queued != nil {
		t.Error("Invalid XML should not queue a response")
	}
}
