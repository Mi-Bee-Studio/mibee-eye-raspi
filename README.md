# MiBee Eye (蜂眼)

[![CI](https://github.com/Mi-Bee-Studio/mibee-eye-raspi/actions/workflows/ci.yml/badge.svg)](https://github.com/Mi-Bee-Studio/mibee-eye-raspi/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/go-1.26-blue.svg)](https://golang.org)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://github.com/Mi-Bee-Studio/mibee-eye-raspi/blob/main/LICENSE)

[中文文档](README_zh.md)

<div align="center">
  <table>
    <tr>
      <td align="center"><b>🪶 15–25 MB</b><br><sub>Memory footprint on RPi 3B</sub></td>
      <td align="center"><b>✅ ONVIF Profile S</b><br><sub>Device · Media · Imaging</sub></td>
      <td align="center"><b>🔧 Zero CGO</b><br><sub>Pure Go, painless cross-compile</sub></td>
    </tr>
  </table>
</div>


MiBee Eye is a lightweight Go ONVIF camera service for single-board computers (Raspberry Pi, Banana Pi, Orange Pi) with support for all CSI/USB cameras. It provides ONVIF Device/Media/Imaging services, RTSP streaming, RTMP push, and WS-Discovery for NVR/VMS integration.

## Features

- **ONVIF Device/Media/Imaging Services** - Full ONVIF compliance for NVR integration
- **RTSP Streaming** - H.264 video streaming at configurable resolutions and bitrates
- **RTMP Push** - Stream to cloud services like Aliyun, Twitch, YouTube
- **WS-Discovery** - Automatic camera discovery on the network
- **Web Admin UI** - Dark-themed admin panel with live preview and camera controls

- **Camera Controls** - Brightness, contrast, saturation, sharpness adjustment
- **HLS Live Streaming** - Pure Go MPEG-TS segmenter for browser playback (no ffmpeg)
- **i18n Support** - English/Chinese web UI
- **Snapshot Support** - JPEG snapshots via HTTP endpoint
- **Low Memory Footprint** - ~15-30MB RAM usage
- **Cross-Platform Build** - Compile from x86 workstation to aarch64 RPi

```bash
# Clone and build
git clone https://github.com/Mi-Bee-Studio/mibee-eye-raspi
cd mibee-eye-raspi
make build

# Copy and configure
cp configs/config.example.yaml config.yaml
# Edit config.yaml for your camera and network

# Run directly
./build/mibee-eye -config config.yaml

# Or deploy with systemd
sudo cp deploy/mibee-eye.service /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now mibee-eye
```

## Configuration

See `configs/config.example.yaml` for all configuration options. Key settings include:

- `camera.width/height` - Capture resolution (1280x720 default)
- `camera.fps` - Frames per second (15 default for SBCs)
- `camera.bitrate` - Video bitrate in bits per second
- `rtsp.port` - RTSP streaming port (8554 default)
- `onvif.port` - ONVIF HTTP/SOAP port (8080 default)
- `onvif.username/password` - ONVIF authentication credentials
- `web.enabled` - Enable Web admin UI (default: true)
- `web.port` - Web UI HTTP port (8088 default)
- `onvif.username/password` - ONVIF authentication credentials

Environment variables override any config setting with `MIBEE_EYE_` prefix:
```bash
MIBEE_EYE_ONVIF_PASSWORD=secret ./build/mibee-eye
```

## Deployment

Create a systemd service unit based on `deploy/mibee-eye.service`. Customize for your environment:

```bash
# Install and configure
sudo cp deploy/mibee-eye.service /etc/systemd/system/
# Edit paths and user for your setup
sudo systemctl daemon-reload
sudo systemctl enable --now mibee-eye
```

## Web Admin UI

The built-in web admin panel provides real-time camera management with modern streaming capabilities:

- **Live Preview** - HLS video player (hls.js library) for smooth browser playback
- **Imaging Controls** - Sliders for brightness, contrast, saturation, sharpness; dropdowns for white balance and exposure mode

- **Server Config** - View all configuration sections, edit ONVIF credentials with save-and-restart
- **WebSocket** - Real-time parameter updates without polling
- **Language Toggle** - Switch between English and Chinese interfaces
- **Theme Toggle** - Dark/light mode switching
- **Snapshot Button** - One-click JPEG capture

Access at `http://<device-ip>:8088/` with web UI credentials (token-based login). Web UI defaults reuse ONVIF credentials.

The Web UI is embedded in the binary via `//go:embed` — no additional files to deploy.
## Supported Cameras

| Module | Sensor | Resolution | Focus | DT Overlay | Notes |
|--------|--------|------------|-------|------------|-------|
| Pi Camera V1 | OV5647 | 2592×1944 | Fixed | `ov5647` | Current setup |
| Pi Camera V2 | IMX219 | 3280×2464 | Fixed | `imx219` | Better low light |
| Pi Camera V3 | IMX708 | 4608×2592 | Autofocus | `imx708` | PDAF, HDR support |
| Pi HQ Camera | IMX477 | 4056×3040 | Manual lens | `imx477` | Interchangeable lens |
| USB (UVC) | Various | Various | Various | Auto-detected | `/dev/video*` |

## Architecture

```mermaid
flowchart TB
    subgraph 相机层
        CAM["CSI/USB 相机"]
    end

    subgraph MiBee Eye
        CAP["相机捕获"]
        RTSP["RTSP 服务器"]
        HLS["HLS 桥接"]
        ONVIF["ONVIF 服务"]
        RTMP["RTMP 推流"]
        CTRL["相机控制"]
        WEBUI["Web 管理界面"]
    end

    subgraph 外部系统
        NVR["NVR/VMS 系统"]
        CLOUD["云服务"]
    end

    CAM --> CAP
    CAP --> RTSP
    CAP --> RTMP
    RTSP --> ONVIF
    RTSP --> HLS
    HLS --> WEBUI
    CTRL --> ONVIF
    WEBUI --> ONVIF
    ONVIF --> NVR
    RTMP --> CLOUD
```
Camera capture via CSI interface supports OV5647, IMX219, IMX708, IMX477 modules. RTSP server uses `gortsplib` (same as MediaMTX). ONVIF provides device discovery, media control and imaging parameter adjustment. RTMP push supports cloud services.

| Metric | MiBee Eye | MediaMTX | Improvement |
|--------|---------|----------|-------------|
| Memory Usage | **15–25 MB** | ~45 MB | 45–67% reduction |
| ONVIF Server | ✅ **Profile S** (Device/Media/Imaging) | ❌ Not supported | — |
| CGO Dependencies | **Zero** | CGO required | Painless cross-compile |
| Camera Control | ✅ Brightness, Contrast, WB, etc. | ❌ None | — |
| RTMP Push | ✅ Built-in | ❌ Extra config needed | — |
| CPU Usage (720p@15fps) | ~15% | ~24% | 37% reduction |

| Metric | MiBee Eye | MediaMTX | Improvement |
|--------|---------|----------|-------------|
| Memory Usage | **15–25 MB** | ~45 MB | 45–67% reduction |
| ONVIF Server | ✅ **Profile S** (Device/Media/Imaging) | ❌ Not supported | — |
| CGO Dependencies | **Zero** | CGO required | Painless cross-compile |
| Camera Control | ✅ Brightness, Contrast, WB, etc. | ❌ None | — |
| RTMP Push | ✅ Built-in | ❌ Extra config needed | — |
| CPU Usage (720p@15fps) | ~15% | ~24% | 37% reduction |

| Component | Library | Rationale |
|-----------|---------|-----------|
| ONVIF Server | Hand-written SOAP | Pure Go, full Device/Media/Imaging |
| RTSP Server | `bluenviron/gortsplib/v5` | Same as MediaMTX, proven compatibility |
| RTMP Push | Pure Go implementation | Active maintenance, low footprint |
| Camera Capture | MediaMTX rpicam (subprocess) | Battle-tested libcamera, no CGO |
| HLS Bridge | Pure Go MPEG-TS segmenter | No external dependencies, lightweight |
| Web UI | embedded (no external lib) + hls.js | Lightweight, no external dependencies |
| Configuration | YAML | Human-readable, easy deployment |
### Technology Stack

| Component | Library | Rationale |
|-----------|---------|-----------|
| ONVIF Server | Hand-written SOAP | Pure Go, full Device/Media/Imaging |
| RTSP Server | `bluenviron/gortsplib/v5` | Same as MediaMTX, proven compatibility |
| RTMP Push | Pure Go implementation | Active maintenance, low footprint |
| Camera Capture | MediaMTX rpicam (subprocess) | Battle-tested libcamera, no CGO |
| HLS Bridge | Pure Go MPEG-TS segmenter | No external dependencies, lightweight |
| Web UI | embedded (no external lib) + hls.js | Lightweight, no external dependencies |
| Configuration | YAML | Human-readable, easy deployment |
Built with pure Go — **zero CGO**. Camera capture uses MediaMTX's existing mtxrpicam binary via subprocess pipe for proven CSI camera support, without the CGO cross-compile pain. All protocols (ONVIF, RTMP, HLS, Snapshot) are implemented in pure Go without external libraries.

## Development

```bash
# Build on workstation
make build

# Cross-compile for aarch64 SBCs
make build GOOS=linux GOARCH=arm64

# Run tests
make test

# Deploy to remote
make deploy REMOTE_HOST=user@your-rpi-host
```

## License

MIT License - see [LICENSE](LICENSE) for details.