[中文](zh/architecture.md)

# Architecture - MiBee Eye ONVIF Camera Server

Measured on single-board computers at 720p@15fps:

MiBee Eye is a lightweight Go application providing ONVIF-compliant camera services for single-board computers (Raspberry Pi, Banana Pi, Orange Pi). It replaces MediaMTX with a custom implementation to add missing ONVIF server capabilities while maintaining low resource usage (~20MB RAM, measured on device: MiBee Eye 9MB + mtxrpicam 10MB). The system supports ONVIF Device/Media/Imaging services, RTSP streaming, RTMP push, and WS-Discovery for NVR integration.

## Component Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                     Main Server                             │
│                   (cmd/server/main.go)                     │
└───────────────────────┬───────────────────────────────────────┘
                       │
┌───────────────────────▼───────────────────────────────────────┐
│                 ONVIF Server                                │
│              (internal/onvif/server.go)                     │
│  ┌─────────────────┐  ┌─────────────────┐  ┌─────────────────┐│
│  │ Device Service  │  │ Media Service  │  │ Imaging Service│
│  │                 │  │                 │  │                ││
│  │ - Device Info   │  │ - Get Profiles  │  │ - Brightness    │
│  │ - Capabilities  │  │ - Get StreamUri│  │ - Contrast      │
│  │ - WS-Discovery  │  │ - Get Snapshot │  │ - Saturation    │
│  │                 │  │                 │  │ - Sharpness     │
│  └─────────────────┘  └─────────────────┘  └─────────────────┘│
└───────────────────────┬───────────────────────────────────────┘
                       │
┌───────────────────────▼───────────────────────────────────────┐
│                 AUHub (internal/h264/hub.go)               │
│                        Frame Fan-out                        │
└───────────────────────┬───────────────────────────────────────┘
                       │
          ┌──────────────┼──────────────┐
          │              │              │
┌────────▼────────┐ ┌────▼─────┐ ┌────▼────────┐
│ RTSP Server     │ │Snapshot │ │ RTMP Push   │
│ (gortsplib v5)  │ │Handler  │ │             │
└─────────────────┘ └──────────┘ └─────────────┘
          │                       
          ▼                       
┌─────────────────────────────────────────────────────────────┐
│                 HLS Server                                │
│              (internal/hls/server.go)                     │
│     Pure Go MPEG-TS segmenter (in-memory segments)
└─────────────────────────────────────────────────────────────┘

│
│                 Metrics Endpoint
│              (internal/metrics/metrics.go)
│     Prometheus metrics for operational visibility
┌─────────────────────────────────────────────────────────────┐
│                 Web UI Server                            │
│              (internal/web/web.go)                        │
│     Admin panel with video player, controls, monitoring    │
└─────────────────────────────────────────────────────────────┘
```

## Key Components

### ONVIF Server (`internal/onvif/server.go`)

The ONVIF server implements a single-endpoint SOAP framework handling multiple ONVIF services:

- **Service Routing**: All SOAP actions dispatched to `/onvif/device_service`
- **Authentication**: WS-Security UsernameToken digest authentication
- **WS-Discovery**: Supports both UDP multicast and HTTP probe requests
- **SOAP Processing**: XML envelope parsing, action routing, fault handling
- **Configuration**: Interface-based config provider for auth and media parameters

Services implemented:
- **Device**: Device information, capabilities, WS-Discovery
- **Media**: Profiles, stream URI, snapshot access

- **Imaging**: Camera parameter control (brightness, contrast, etc.)

### Camera Subsystem (`internal/camera/camera.go`)

Camera capture supports two modes (configurable via `camera.mode`):

**mtxrpicam mode (default)**: Uses MediaMTX's proven `mtxrpicam` C binary (from
[mediamtx-rpicamera](https://github.com/bluenviron/mediamtx-rpicamera)) via subprocess. It bundles its own `libcamera.so.9.9` to avoid version conflicts with system libcamera.

**rtsp mode**: Consumes an external RTSP URL for testing without camera hardware (RTSPSource implementation in `internal/camera/`).

For mtxrpicam mode:

- **Pipe Protocol**: 4-byte LE framed protocol (config and video frames)
- **Subprocess Isolation**: Spawned with `Setpgid=true` for signal isolation
- **Parameter Control**: Real-time camera parameter updates via config pipe
- **Error Handling**: Process monitoring and graceful shutdown
Required files in `deploy/bin/`:
`mtxrpicam`, `libcamera.so.9.9`, `libcamera-base.so.9.9`,
`ipa_module/ipa_rpi_vc4.so`, `ipa_module/ipa_rpi_vc4.so.sign`,
`libpisp/backend_default_config.json`, `ipa_conf/`

`LD_LIBRARY_PATH` must include `deploy/bin/` so mtxrpicam finds bundled libcamera. The interface supports start/stop, parameter updates, and buffered frame delivery (2s at 15fps).

### H.264 AUHub (`internal/h264/hub.go`)

AUHub provides frame distribution to multiple consumers with fan-out pattern:

- **Thread Safety**: Internal mutex for concurrent subscriber management
- **Non-blocking Delivery**: Drops frames to prevent writer blocking
- **Subscriber Management**: Automatic cleanup on context cancellation
- **Access Unit Format**: H.264 access units with timestamp and keyframe detection

Consumers include:
- RTSP server for video streaming
- Snapshot handler for JPEG capture
- RTMP push for cloud services
- **HLS server**: pure Go MPEG-TS segmenter, in-memory segments (no subprocess)
### RTSP Server (`internal/rtsp/server.go`)

RTSP server built on `gortsplib v5` for H.264 streaming:

- **Protocol Support**: RTSP 1.0 with DESCRIBE, SETUP, PLAY commands
- **Authentication**: Optional digest authentication for stream access
- **On-demand Streaming**: Starts frame consumption only when clients connect
- **Media Description**: Dynamic H.264 format with SPS/PPS updates
- **Timestamp Synchronization**: NTP-adjusted timestamps for accurate playback

Key features:
- Single-port RTSP serving
- Client connection management
- RTP packet encoding and transmission
- Stream resource cleanup
### HLS Server (`internal/hls/server.go`)

HLS Server provides live streaming via pure Go MPEG-TS segmenter:

- **Pure Go Implementation**: MPEG-TS segmenter in `internal/hls/muxts.go` (no subprocess)
- **In-Memory Segments**: Segments kept in memory, no .ts files written to disk
- **HTTP Serving**: Serves .m3u8 playlist and .ts segments via HTTP endpoints
- **Configuration**: Configurable `segment_duration` (default 2s) and playlist size
- **Integration**: Consumes H.264 frames directly from AUHub
- **Low Overhead**: Minimal memory usage (in-memory segment buffer, a few MB)

Key features:
- HLS media playlist generation with sequence numbers
- Pure Go MPEG-TS segmenter with no external dependencies
- HTTP-based delivery to web browsers
- Support for hls.js player integration
- Sub-second latency streaming with configurable parameters

### Web UI Server (`internal/web/web.go`)

Web UI Server provides browser-based camera management interface:

- **Authentication**: Token-based auth with login/logout functionality
- **i18n Support**: English/Chinese language switching
- **Themes**: Dark/light theme preferences
- **Video Player**: MSE (Media Source Extensions) for sub-second latency preview, plus HLS (hls.js) for compatibility
- **Camera Controls**: Real-time brightness, contrast, saturation, sharpness adjustment

- **Snapshot**: JPEG capture with download capability
- **Monitoring**: Real-time parameter updates via WebSocket
- **Server Config**: Configuration viewer and editor with ONVIF credentials management


## Data Flow Pipeline

OV5647 Camera → mtxrpicam → H.264 NALUs → Parser → AUHub → Subscribers
                                       ↓
                             ┌─────────────┼─────────────┐
                             │             │             │
                       RTSP Server     Snapshot Handler   RTMP Push
                       (rpicam-still + IDR fallback)
```

1. **Capture**: mtxrpicam subprocess captures frames from OV5647 CSI camera
2. **Transport**: H.264 data transferred via binary pipe to Go process
3. **Processing**: Parser extracts NALUs and timestamps, detects keyframes
4. **Distribution**: AUHub fans out access units to multiple consumers
5. **Streaming**: RTSP server serves video via gortsplib to NVR clients
6. **Snapshot**: Dual-tier strategy — tries rpicam-still JPEG capture (3s timeout), falls back to raw H.264 IDR frame
7. **Control**: ONVIF services provide camera control and discovery
8. **HLS**: Pure Go MPEG-TS segmenter produces in-memory segments for browser playback
## Resource Usage

Measured on Raspberry Pi 3B at 720p@15fps:

|| Process | RSS Memory | Purpose |
||---------|------------|---------|
| MiBee Eye | ~9MB | Go main process (ONVIF + RTSP + pipeline) |
|| mtxrpicam | ~10MB | Camera capture subprocess |
- **Total** | **~19MB** | (HLS adds minimal memory — in-memory segment buffer, a few MB)

- **CPU**: ~2% for MiBee Eye, ~12% for mtxrpicam at 720p@15fps
- **Network**: ~2Mbps for 720p@15fps H.264 stream

## Dependencies

- **gortsplib v5**: RTSP server functionality (same library as MediaMTX)
- **pion/rtp**: RTP packet handling for H.264 streaming
- **yaml.v3**: Configuration file parsing
- **onvif-go**: ONVIF server implementation (indirect dependency via research)
- **mtxrpicam**: Camera capture subprocess with bundled libcamera (from bluenviron/mediamtx-rpicamera v2.6.0)
## Deployment Architecture

The system runs as a single systemd service with:

- **Process Isolation**: Camera capture in subprocess, main service in Go process
- **Resource Usage**: ~20MB RAM measured on SBCs
- **Cross-compilation**: Build from x86 workstation to aarch64 RPi
- **Configuration**: YAML-based config with environment variable overrides
- **Monitoring**: Prometheus metrics for operational visibility

### Camera Capture Dependencies

|| Component | Type | Size | Purpose |
||-----------|------|------|---------|
|| mtxrpicam | C binary (arm64) | 1.7MB | Camera capture + H.264 encoding |
|| libcamera.so.9.9 | Shared library (bundled) | 5.7MB | Camera framework (from mediamtx-rpicamera) |
|| libcamera-base.so.9.9 | Shared library (bundled) | 140KB | libcamera base support |
|| ipa_module/ipa_rpi_vc4.so | IPA module | 690KB | RPi VC4 image processing |
|| libpisp/backend_default_config.json | Config | 11KB | PiSP backend configuration |
These dependencies are bundled from mediamtx-rpicamera releases and do NOT depend on the system-installed libcamera. This avoids version conflicts between Debian's libcamera (0.7.0) and the version mtxrpicam was compiled against.

- **HLS**: Pure Go MPEG-TS segmenter produces in-memory segments for browser playback (Web UI uses hls.js, no .ts files on disk)

This architecture replaces MediaMTX entirely to provide ONVIF compliance while maintaining the proven camera capture and RTSP streaming components.