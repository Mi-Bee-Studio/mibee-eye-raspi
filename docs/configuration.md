# Configuration Documentation

[中文](zh/configuration.md)

The MiBee Eye configuration is written in YAML format and controls all aspects of the camera service, including capture settings, streaming protocols, and device identification.

## Configuration File

### File Location
Configuration is loaded from `configs/config.yaml` by default. Create this file by copying `configs/config.example.yaml` and modifying it for your setup.

### File Format
```yaml
# Comments use the # symbol
# Top-level sections define functional areas
camera:        # Camera capture settings
rtsp:          # RTSP streaming server
onvif:         # ONVIF device services
rtmp:          # RTMP push streaming
web:           # Web UI configuration
device:        # Device identification
logging:       # Logging configuration
```

## Configuration Sections

### Camera Configuration

Camera capture settings control how video frames are captured from the camera device.

```yaml
camera:
  # Capture mode: "mtxrpicam" (default, uses subprocess) or "rtsp" (consumes external RTSP URL)
  mode: mtxrpicam

  # Path to mtxrpicam binary (camera capture subprocess)
  # This binary and its bundled libcamera libraries must be present at this path
  bin_path: deploy/bin/mtxrpicam

  # Camera device path (V4L2 or libcamera)
  device: /dev/video0
  
  # Capture resolution (width x height)
  # Supported resolutions: 640x480, 1296x972, 1920x1080, 2592x1944
  width: 1280
  height: 720
  
  # Frames per second (max 30 for OV5647 sensor)
  fps: 15
  
  # Video codec (h264 or h265)
  codec: h264
  
  # Target bitrate in bits per second
  # Example: 2000000 = 2 Mbps
  bitrate: 2000000
  
  # Image controls (hardware-specific ranges apply)
  # Brightness: -1.0 to 1.0 (0.0 = default, negative = darker, positive = brighter)
  brightness: 0.0
  
  # Contrast: 0.0 to 32.0 (1.0 = default)
  contrast: 1.0
  
  # Saturation: 0.0 to 32.0 (1.0 = default)
  saturation: 1.0
  
  # Sharpness: 0.0 to 16.0 (1.0 = default)
  sharpness: 1.0
  
  # External RTSP URL when mode=rtsp (ignored when mode=mtxrpicam)
  rtsp_url: ""
  
  # Keyframe interval (1=every frame, 15=every 15th frame)
  idr_period: 15
  
  # Frame channel buffer capacity (frames)
  frame_buffer_size: 30
  
  # Maximum subprocess restart backoff duration
  max_backoff: 30s
```

### RTSP Configuration

RTSP server settings for video streaming clients.

```yaml
rtsp:
  # RTSP server port (default: 8554)
  port: 8554
  
  # Optional RTSP authentication
  # Leave empty strings for no authentication
  username: ""
  password: ""
  
  # AUHub subscriber channel buffer size
  subscriber_buffer_size: 64
  
  # gortsplib write queue size (256 default too small for WiFi)
  write_queue_size: 2048
  
  # Enable UDP transport (needed for NVR clients)
  enable_udp: true
  
  # UDP RTP port (default: 8000)
  udp_rtp_port: 8000
  
  # UDP RTCP port (default: 8001)
  udp_rtcp_port: 8001
```

### ONVIF Configuration

ONVIF server settings for device discovery and control via NVR systems.

```yaml
onvif:
  # ONVIF HTTP/SOAP port (default: 8080)
  port: 8080
  
  # ONVIF WS-UsernameToken authentication
  # Required for MiBee NVR integration
  username: "admin"
  
  # ONVIF password (MUST be set for production)
  password: ""

```

### Web UI Configuration

Web UI settings for the built-in browser-based admin panel with live preview and camera configuration.

```yaml
web:
  # Enable Web admin UI (default: true)
  enabled: true

  # Web UI HTTP port (default: 8088)
  port: 8088

  # Web UI authentication
  # Uses ONVIF credentials when username/password are empty
  username: "admin"
  password: ""
  
  # CORS allowed origins (use specific origins in production)
  allowed_origins:
    - "*"
  
  # HTTP server timeouts
  read_header_timeout: 5s
  read_timeout: 10s
  write_timeout: 30s
  idle_timeout: 120s
```



### RTMP Configuration

RTMP push settings for streaming to cloud services.

```yaml
rtmp:
  # Enable RTMP push streaming
  enabled: false
  
  # RTMP push URL for cloud services
  # Examples:
  # - rtmp://push-server/app/stream
  # - rtmp://live.twitch.tv/app/channel-key
  url: "rtmp://push-server/app/stream"
  
  # Maximum reconnection attempts (0 = unlimited)
  max_retries: 10
```

### Device Configuration

Device information exposed via ONVIF GetDeviceInformation service.

```yaml
device:
  # Friendly camera name for display in NVR
  name: "Pi Camera V1"
  
  # Device manufacturer
  manufacturer: "Raspberry Pi"
  
  # Camera sensor model
  model: "OV5647"
  
  # Firmware version string
  firmware: "1.0.0"
  
  # Hardware identifier
  hardware_id: "OV5647"
  
  # Serial number (empty if not available)
  serial_number: ""
```

### Logging Configuration

Logging settings for debugging and monitoring.

```yaml
logging:
  # Log level (debug, info, warn, error)
  # debug: Most verbose, includes all debug messages
  # info: Standard operational logging
  # warn: Only warnings and errors
  # error: Only errors
  level: "info"
```

### Snapshot Configuration

Snapshot endpoint settings for JPEG/H.264 capture via HTTP.

```yaml
snapshot:
  # Enable the snapshot endpoint (default: true)
  enabled: true

  # JPEG quality 1-100 (only used for rpicam-still subprocess; H.264 IDR fallback ignores this)
  quality: 85
```

The snapshot endpoint uses a dual-tier strategy:
1. **Tier 1**: `rpicam-still` subprocess captures a real JPEG when the camera is idle
2. **Tier 2**: Falls back to the stored H.264 IDR frame (returned as `video/H264`) when the camera pipeline is busy

### Metrics Configuration

Prometheus metrics exporter settings.

```yaml
metrics:
  # Enable the metrics HTTP endpoint (default: true)
  enabled: true

  # Metrics HTTP server port (default: 9100)
  # NOTE: 9100 conflicts with Prometheus node_exporter — change port or disable if both run on same host
  port: 9100
```

### HLS Configuration

HLS live streaming settings for browser playback.

```yaml
hls:
  # Enable HLS server (default: false)
  enabled: false

  # Target segment duration (default: 2s)
  segment_duration: 2s
```

The HLS server uses a pure Go MPEG-TS segmenter — no ffmpeg subprocess. Segments are kept in-memory.


## Default Values Reference

| Section | Field | Default Value | Type | Description |
|---------|-------|---------------|------|-------------|
| **camera** | mode | `"mtxrpicam"` | string | Capture mode (mtxrpicam or rtsp) |
| | bin_path | `"deploy/bin/mtxrpicam"` | string | Path to mtxrpicam binary |
| | device | `/dev/video0` | string | Camera device path |
| | width | `1280` | int | Capture width in pixels |
| | height | `720` | int | Capture height in pixels |
| | fps | `15` | int | Frames per second |
| | codec | `"h264"` | string | Video codec |
| | bitrate | `2000000` | int | Bitrate in bits per second |
| | brightness | `0.0` | float | Brightness control |
| | contrast | `1.0` | float | Contrast control |
| | saturation | `1.0` | float | Saturation control |
| | sharpness | `1.0` | float | Sharpness control |
| | rtsp_url | `""` | string | External RTSP URL (when mode=rtsp) |
| | idr_period | `15` | int | Keyframe interval |
| | frame_buffer_size | `30` | int | Frame channel buffer capacity |
| | max_backoff | `"30s"` | string | Maximum subprocess restart backoff |
| **rtsp** | port | `8554` | int | RTSP server port |
| | username | `""` | string | RTSP username |
| | password | `""` | string | RTSP password |
| | subscriber_buffer_size | `64` | int | AUHub subscriber channel buffer size |
| | write_queue_size | `2048` | int | gortsplib write queue size |
| | enable_udp | `true` | bool | Enable UDP transport |
| | udp_rtp_port | `8000` | int | UDP RTP port |
| | udp_rtcp_port | `8001` | int | UDP RTCP port |
| **onvif** | port | `8080` | int | ONVIF HTTP port |
| | username | `"admin"` | string | ONVIF username |
| | password | `""` | string | ONVIF password |
| **device** | name | `"Pi Camera V1"` | string | Friendly camera name |
| | manufacturer | `"Raspberry Pi"` | string | Device manufacturer |
| | model | `"OV5647"` | string | Camera sensor model |
| | firmware | `"1.0.0"` | string | Firmware version |
| | hardware_id | `"OV5647"` | string | Hardware identifier |
| | serial_number | `""` | string | Device serial number |
| **logging** | level | `"info"` | string | Log level |
| **web** | enabled | `true` | bool | Enable Web UI |
| | port | `8088` | int | Web UI HTTP port |
| | username | `""` | string | Web UI username (defaults to onvif.username) |
| | password | `""` | string | Web UI password (defaults to onvif.password) |
| | allowed_origins | `["*"]` | []string | CORS allowed origins |
| | read_header_timeout | `"5s"` | string | HTTP read header timeout |
| | read_timeout | `"10s"` | string | HTTP read timeout |
| | write_timeout | `"30s"` | string | HTTP write timeout |
| | idle_timeout | `"120s"` | string | HTTP idle timeout |
| **metrics** | enabled | `true` | bool | Enable metrics endpoint |
| | port | `9100` | int | Metrics HTTP server port |
| **snapshot** | enabled | `true` | bool | Enable snapshot endpoint |
| | quality | `85` | int | JPEG quality 1-100 |
| **rtmp** | enabled | `false` | bool | Enable RTMP push |
| | url | `"rtmp://push-server/app/stream"` | string | RTMP push URL |
| | max_retries | `10` | int | Maximum reconnection attempts |
| **hls** | enabled | `false` | bool | Enable HLS server |
| | segment_duration | `"2s"` | string | Target segment duration |

## Environment Variable Overrides

All configuration values can be overridden using environment variables with the `MIBEE_EYE_` prefix. This is useful for deployment, testing, and containerized environments.

### Format
Environment variables follow the pattern: `MIBEE_EYE_<SECTION>_<FIELD>`

### Examples

```bash
# Override camera resolution
MIBEE_EYE_CAMERA_WIDTH=1920 MIBEE_EYE_CAMERA_HEIGHT=1080 ./mibee-eye

# Set ONVIF password for production
MIBEE_EYE_ONVIF_PASSWORD=securepassword123 ./mibee-eye

# Change RTSP port
MIBEE_EYE_RTSP_PORT=554 ./mibee-eye

# Enable debug logging
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye

# Web UI access and credentials
MIBEE_EYE_WEB_ENABLED=true ./mibee-eye

# Set web UI credentials (separate from ONVIF)
MIBEE_EYE_WEB_USERNAME=admin MIBEE_EYE_WEB_PASSWORD=webpass ./mibee-eye
# Set ONVIF password for production
MIBEE_EYE_ONVIF_PASSWORD=securepassword123 ./mibee-eye

# Set device information
MIBEE_EYE_DEVICE_NAME="Office Camera" ./mibee-eye
```

### All Environment Variables

| Section | Field | Environment Variable |
|---------|-------|---------------------|
| **camera** | device | `MIBEE_EYE_CAMERA_DEVICE` |
| | width | `MIBEE_EYE_CAMERA_WIDTH` |
| | height | `MIBEE_EYE_CAMERA_HEIGHT` |
| | fps | `MIBEE_EYE_CAMERA_FPS` |
| | codec | `MIBEE_EYE_CAMERA_CODEC` |
| | bitrate | `MIBEE_EYE_CAMERA_BITRATE` |
| | brightness | `MIBEE_EYE_CAMERA_BRIGHTNESS` |
| | contrast | `MIBEE_EYE_CAMERA_CONTRAST` |
| | saturation | `MIBEE_EYE_CAMERA_SATURATION` |
| | sharpness | `MIBEE_EYE_CAMERA_SHARPNESS` |
| | idr_period | `MIBEE_EYE_CAMERA_IDR_PERIOD` |
| | bin_path | `MIBEE_EYE_CAMERA_BIN_PATH` |
| | frame_buffer_size | `MIBEE_EYE_CAMERA_FRAME_BUFFER_SIZE` |
| | max_backoff | `MIBEE_EYE_CAMERA_MAX_BACKOFF` |
| **rtsp** | port | `MIBEE_EYE_RTSP_PORT` |
| | username | `MIBEE_EYE_RTSP_USERNAME` |
| | password | `MIBEE_EYE_RTSP_PASSWORD` |
| | subscriber_buffer_size | `MIBEE_EYE_RTSP_SUBSCRIBER_BUFFER_SIZE` |
| | write_queue_size | `MIBEE_EYE_RTSP_WRITE_QUEUE_SIZE` |
| | enable_udp | `MIBEE_EYE_RTSP_ENABLE_UDP` |
| | udp_rtp_port | `MIBEE_EYE_RTSP_UDP_RTP_PORT` |
| | udp_rtcp_port | `MIBEE_EYE_RTSP_UDP_RTCP_PORT` |
| **onvif** | port | `MIBEE_EYE_ONVIF_PORT` |
| | username | `MIBEE_EYE_ONVIF_USERNAME` |
| | password | `MIBEE_EYE_ONVIF_PASSWORD` |
| **device** | name | `MIBEE_EYE_DEVICE_NAME` |
| | manufacturer | `MIBEE_EYE_DEVICE_MANUFACTURER` |
| | model | `MIBEE_EYE_DEVICE_MODEL` |
| | firmware | `MIBEE_EYE_DEVICE_FIRMWARE` |
| | hardware_id | `MIBEE_EYE_DEVICE_HARDWAREID` |
| | serial_number | `MIBEE_EYE_DEVICE_SERIALNUMBER` |
| **logging** | level | `MIBEE_EYE_LOGGING_LEVEL` |
| **web** | enabled | `MIBEE_EYE_WEB_ENABLED` |
| | port | `MIBEE_EYE_WEB_PORT` |
| | username | `MIBEE_EYE_WEB_USERNAME` |
| | password | `MIBEE_EYE_WEB_PASSWORD` |
| | allowed_origins | `MIBEE_EYE_WEB_ALLOWED_ORIGINS` |
| | read_header_timeout | `MIBEE_EYE_WEB_READ_HEADER_TIMEOUT` |
| | read_timeout | `MIBEE_EYE_WEB_READ_TIMEOUT` |
| | write_timeout | `MIBEE_EYE_WEB_WRITE_TIMEOUT` |
| | idle_timeout | `MIBEE_EYE_WEB_IDLE_TIMEOUT` |
| **metrics** | enabled | `MIBEE_EYE_METRICS_ENABLED` |
| | port | `MIBEE_EYE_METRICS_PORT` |
| **snapshot** | enabled | `MIBEE_EYE_SNAPSHOT_ENABLED` |
| | quality | `MIBEE_EYE_SNAPSHOT_QUALITY` |
| **rtmp** | enabled | `MIBEE_EYE_RTMP_ENABLED` |
| | url | `MIBEE_EYE_RTMP_URL` |
| | max_retries | `MIBEE_EYE_RTMP_MAX_RETRIES` |
| **hls** | enabled | `MIBEE_EYE_HLS_ENABLED` |
| | segment_duration | `MIBEE_EYE_HLS_SEGMENT_DURATION` |

## Example Configurations

### Basic Configuration (Default Settings)

```yaml
# configs/config.yaml
camera:
  device: /dev/video0
  width: 1280
  height: 720
  fps: 15
  codec: h264
  bitrate: 2000000
  brightness: 0.0
  contrast: 1.0
  saturation: 1.0
  sharpness: 1.0

rtsp:
  port: 8554
  username: ""
  password: ""

onvif:
  port: 8080
  username: "admin"
  password: ""

rtmp:
  enabled: false
  url: "rtmp://push-server/app/stream"

device:
  name: "Pi Camera V1"
  manufacturer: "Raspberry Pi"
  model: "OV5647"
  firmware: "1.0.0"
  hardware_id: "OV5647"
  serial_number: ""

logging:
  level: "info"

web:
  enabled: true
  port: 8088
  username: "admin"
  password: ""

```

### High-Resolution Configuration

```yaml
camera:
  device: /dev/video0
  width: 1920
  height: 1080
  fps: 25
  codec: h264
  bitrate: 4000000  # 4 Mbps
  brightness: 0.2
  contrast: 1.5
  saturation: 1.2
  sharpness: 2.0

rtsp:
  port: 8554
  username: "stream"
  password: "streampass"

onvif:
  port: 8080
  username: "admin"
  password: "onvif123"

device:
  name: "HD Security Camera"
  manufacturer: "Raspberry Pi"
  model: "OV5647"
  firmware: "2.0.0"
  hardware_id: "OV5647-HD"
  serial_number: "SN-2024-001"

web:
  enabled: true
  port: 8088
  username: "admin"
  password: ""

### Cloud Streaming Configuration
### Cloud Streaming Configuration

```yaml
camera:
  width: 1280
  height: 720
  fps: 15
  codec: h264
  bitrate: 2000000

rtsp:
  port: 8554
  username: ""
  password: ""

onvif:
  port: 8080
  username: "admin"
  password: "secure123"

rtmp:
  enabled: true
  url: "rtmp://live.example.com/live/stream-key"

device:
  name: "Cloud Stream Camera"
  manufacturer: "Raspberry Pi"
  model: "OV5647"
  firmware: "1.2.0"
  hardware_id: "OV5647-CLOUD"

logging:
  level: "warn"

web:
  enabled: true
  port: 8088
  username: "admin"
  password: ""

### Low-Bandwidth Configuration
### Low-Bandwidth Configuration

```yaml
camera:
  width: 640
  height: 480
  fps: 10
  codec: h264
  bitrate: 500000  # 0.5 Mbps
  brightness: 0.0
  contrast: 1.0
  saturation: 1.0
  sharpness: 1.0

rtsp:
  port: 8554
  username: ""
  password: ""

onvif:
  port: 8080
  username: "admin"
  password: "lowpass"

device:
  name: "Low Bandwidth Camera"
  manufacturer: "Raspberry Pi"
  model: "OV5647"
  firmware: "1.0.0"
  hardware_id: "OV5647-LBW"

logging:
  level: "error"

web:
  enabled: true
  port: 8088
  username: "admin"
  password: ""

## Configuration Tips
## Configuration Tips

1. **Camera Compatibility**: Not all resolutions and settings work with all camera modules. Test your configuration with your specific camera hardware.

2. **Performance**: Higher resolutions and bitrates require more CPU and bandwidth. On SBCs, 720p @ 15fps is the recommended balance.

3. **Security**: Always set a strong password for ONVIF authentication in production environments.

4. **Network**: RTSP streaming can consume significant bandwidth. Ensure your network infrastructure can handle the chosen bitrate.

5. **Debugging**: Use `MIBEE_EYE_LOGGING_LEVEL=debug` to troubleshoot configuration issues.

6. **Environment Variables**: Use environment variables for sensitive data like passwords to avoid storing them in configuration files.

7. **Validation**: The service will validate configuration values against hardware constraints. Invalid settings will be logged or defaulted.

8. **Web UI Access**: The web admin panel is available at http://<device-ip>:8088/. Use ONVIF credentials (or web-specific credentials if configured) to log in.

9. **Camera Binary**: The `bin_path` must point to a valid mtxrpicam binary. The directory containing this binary must also contain the bundled libcamera shared libraries (libcamera.so.9.9, libcamera-base.so.9.9) and IPA modules. See deployment documentation for details.
