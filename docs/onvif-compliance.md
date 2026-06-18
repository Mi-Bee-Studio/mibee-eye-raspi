[中文](zh/onvif-compliance.md)\n
# ONVIF Compliance Reference

This document provides detailed compliance information for the MiBee Eye ONVIF camera service. The implementation provides ONVIF Device/Media/Imaging services, WS-Discovery support, and WS-Security authentication for NVR integration.

## ONVIF Profile S Compliance

| Component | Status | Notes |
|-----------|--------|-------|
| **ONVIF Device Service** | ✅ Full | WS-Discovery, GetDeviceInformation, GetCapabilities |
| **ONVIF Media Service** | ✅ Full | GetProfiles, GetStreamUri, VideoSource support |
| **ONVIF PTZ Service** | ❌ Not implemented | PTZ removed as dead code (no camera wiring) |
| **ONVIF Imaging Service** | ✅ Full | Brightness, contrast, saturation, sharpness, exposure, white balance |
| **WS-Discovery** | ✅ Full | UDP multicast + HTTP POST probe support |
| **WS-Security** | ✅ Full | UsernameToken Text and Digest authentication |

## Supported Services

### Device Service

| Operation | Implemented | Notes |
|-----------|-------------|-------|
| `GetSystemDateAndTime` | ✅ | Returns UTC time, manual mode (no NTP sync) |
| `GetDeviceInformation` | ✅ | Manufacturer, model, firmware, serial, hardware ID from config |
| `GetCapabilities` | ✅ | Media, Device, Imaging services advertised |
| `GetServices` | ✅ | Lists all ONVIF services with XAddr endpoints |
| `GetScopes` | ✅ | Name, hardware, and type scopes |

**Device Information Response:**
```yaml
Manufacturer: "Raspberry Pi"
Model: "Camera V1" 
Firmware: "mibee-eye-v1.0.0"
SerialNumber: "OV5647-SERIAL"
HardwareId: "OV5647"
```

**Capabilities Response:**
```yaml
Device: XAddr: "http://<camera-ip>:8080/onvif/device_service"
Media: XAddr: "http://<camera-ip>:8080/onvif/media_service"  

Imaging: XAddr: "http://<camera-ip>:8080/onvif/device_service"
```

### Media Service

| Operation | Implemented | Notes |
|-----------|-------------|-------|
| `GetProfiles` | ✅ | Single profile with H.264 encoding |
| `GetStreamUri` | ✅ | Returns RTSP URL (rtsp://host:port/stream) |
| `GetVideoSources` | ✅ | Single video source (Pi Camera) |

**Profile Configuration:**
```yaml
Profiles:
  - Token: "main"
    Name: "main"
    VideoSourceConfiguration:
      Token: "videoSrc0"
      Bounds: { x: 0, y: 0, width: 1280, height: 720 }
    VideoEncoderConfiguration:
      Token: "enc0"
      Encoding: "H264"
      Resolution: { Width: 1280, Height: 720 }
      RateControl: { FrameRateLimit: 15, BitrateLimit: 2000000 }
```

**Stream URI Response:**
```yaml
MediaUri:
  Uri: "rtsp://<camera-ip>:8554/stream"
  InvalidAfterConnect: "false"
  InvalidAfterReboot: "false"
  Timeout: "PT0S"
```

### Imaging Service

| Operation | Implemented | Notes |
|-----------|-------------|-------|
| `GetImagingSettings` | ✅ | Current camera parameters |
| `SetImagingSettings` | ✅ | Brightness, contrast, saturation, sharpness, exposure |
| `GetOptions` | ✅ | Parameter ranges and available modes |

**Supported Imaging Parameters:**
- **Brightness**: -1.0 to 1.0 (real-time adjustment)
- **Contrast**: 0.0 to 32.0 (real-time adjustment)
- **Saturation**: 0.0 to 32.0 (real-time adjustment)
- **Sharpness**: 0.0 to 16.0 (real-time adjustment)
- **Exposure**: AUTO/MANUAL modes, 0.0 to 10000 μs exposure time
- **White Balance**: AUTO/MANUAL modes, 0.0 to 1.0 Cr/Cb gain

**Imaging Settings Response:**
```yaml
Settings:
  Brightness: { Value: 0.0 }
  Contrast: { Value: 1.0 }
  Saturation: { Value: 1.0 }
  Sharpness: { Value: 0.0 }
  Exposure: { Mode: "AUTO", Time: 16667 }
  WhiteBalance: { Mode: "AUTO" }
```

PTZ service was removed as dead code (state machine tracked position but never applied to camera - no ScalerCrop wiring). The OV5647 hardware has no PTZ motors.


The service supports both WS-Discovery probe methods:
### UDP Multicast (239.255.255.250:3702)
- Listens for Probe messages on multicast group
- Responds with ProbeMatches containing device metadata
- Automatically detects local IP for XAddr generation
- Scopes include /name/ and /hardware/ for NVR matching

### HTTP POST Probe (/onvif/device_service)
- Handles Probe messages via HTTP POST to device service endpoint
- Enables discovery through firewalls/proxies
- Same XML response as UDP multicast
- ProbeMatches XML is built as raw string bytes (not encoding/xml) for exact element local names
- NVR depends on exact ProbeMatch structure for device matching

**ProbeMatches Response:**
```xml
<ProbeMatch>
  <EndpointReference>
    <Address>uuid:device-uuid-here</Address>
  </EndpointReference>
  <Scopes>onvif://www.onvif.org/name/Pi Camera V1 onvif://www.onvif.org/hardware/OV5647</Scopes>
  <XAddrs>http://<camera-ip>:8080/onvif/device_service</XAddrs>
  <Types>tdn:NetworkVideoTransmitter tdn:Device</Types>
  <MetadataVersion>1</MetadataVersion>
</ProbeMatch>
```

## WS-Security Support

UsernameToken authentication is implemented with both password types:

### PasswordText Mode
- Direct password comparison
- Used when no Nonce is present
- Simple but less secure

### PasswordDigest Mode  
- SHA1 digest: `base64(SHA1(base64(Nonce) + Created + Password))`
- More secure, requires Nonce and Created timestamp
- Recommended for production use

**Authentication Flow:**
1. Parse SOAP Security header for UsernameToken
2. If Nonce present, compute digest and compare
3. If no Nonce, use direct password comparison
4. Return AuthResult with username and success status

## Known Limitations

### Functional Limitations
- **Single Profile**: Only one media profile supported (main profile)
- **No Audio**: Audio streaming not implemented
- **No Events**: Event service not supported
- **No Analytics**: Video analytics not available
- **No Extensions**: ONVIF extensions (e.g., Analytics, Search) not implemented

### Hardware Constraints  
- **OV5647 Camera**: Fixed focus, no autofocus capability
- **No IR Cut Filter**: Fixed IR filter (NoIR variant not supported)
- **No WDR**: Wide dynamic range not available on OV5647


### Protocol Limitations
- **Manual System Time**: No NTP sync, UTC time fixed at startup
- **Single Video Source**: Only one CSI camera supported
- **No HTTPS**: HTTP only (no SSL/TLS encryption)
- **Basic Authentication**: No advanced auth methods (e.g., certificates)

## Tested ONVIF Clients

### MiBee NVR
- **Protocol**: ONVIF Client (0x524a/onvif-go library)
- **Discovery**: WS-Discovery + HTTP probe
- **Authentication**: UsernameToken Digest
- **Operations**: GetDeviceInformation, GetCapabilities, GetProfiles, GetStreamUri
- **Integration Status**: ✅ Fully compatible

### Compatibility Notes
- Uses same `0x524a/onvif-go` library as server, ensuring protocol compatibility
- Supports both UDP and HTTP discovery methods
- Handles Profile S compliance requirements
- RTSP DESCRIBE works for stream validation
- SOAP fallback support for edge cases

## Service Endpoints

| Service | Endpoint | Protocol | Description |
|---------|----------|----------|-------------|
| Device Service | `/onvif/device_service` | HTTP/SOAP | Device management |
| Media Service | `/onvif/media_service` | HTTP/SOAP | Media profile/URI |
| Snapshot | `/snapshot` | HTTP | JPEG snapshots |
| RTSP Stream | `/stream` | RTSP | H.264 video stream |

## Error Handling

The service follows ONVIF SOAP 1.2 fault conventions:

- **soap:Sender**: Client request errors (invalid action, auth failure)
- **soap:Receiver**: Server processing errors (camera access, parameter invalid)
- **SOAP Fault Codes**: Standard fault codes with descriptive text

**Example Fault Response:**
```xml
<SOAP-ENV:Fault>
  <faultcode>soap:Sender</faultcode>
  <faulttext>authentication failed: password mismatch</faulttext>
</SOAP-ENV:Fault>
```