# AGENTS.md — MiBee Eye (蜂眼)

Go 1.26+ ONVIF camera service for single-board computers. Primary purpose: **test camera for MiBee NVR** (`/home/mickey/Projects/iot/mibee-nvr`). Implements ONVIF Device/Media/Imaging server + RTSP streaming via `mtxrpicam` subprocess (no CGO, no MediaMTX dependency at runtime).

## Device & Credentials

| Item | Value |
|------|-------|
| Host | `ssh mickey@192.168.63.162` (sudo NOPASSWD) |
| Board | RPi 3B, aarch64, 4 cores @1.2GHz, 905MB RAM (~565MB available) |
| Camera | CSI module via libcamera (currently IMX219, OV5647 also supported) |
| Network | WiFi only — **drops under sustained transfer**, 270Mbps theoretical |
| Binaries | `/home/mickey/mibee-eye/mibee-eye` + `/home/mickey/mibee-eye/deploy/bin/mtxrpicam` |
| Config | `/home/mickey/mibee-eye/config.yaml` (device-local, NOT in repo) |
| Service | `systemctl {status,start,stop,restart} mibee-eye` |

## Build & Deploy

```bash
# Cross-compile (from workstation)
GOOS=linux GOARCH=arm64 go build -o build/mibee-eye ./cmd/server
# Test
go test ./...
```

**Deploy gotchas** (learned the hard way):

1. `make deploy` is **broken** — tries to `scp configs/config.yaml` which doesn't exist (only `config.example.yaml` in repo). Deploy binary manually instead.
2. **WiFi drops under load** — stop the service before uploading, then restart:
   ```bash
   ssh mickey@192.168.63.162 'sudo systemctl stop mibee-eye'
   gzip -c build/mibee-eye | ssh mickey@192.168.63.162 'gunzip > ~/mibee-eye/mibee-eye && chmod +x ~/mibee-eye/mibee-eye'
   ssh mickey@192.168.63.162 'sudo systemctl start mibee-eye'
   ```
3. `mtxrpicam` binary must exist at `~/mibee-eye/deploy/bin/mtxrpicam` — systemd sets `LD_LIBRARY_PATH=/home/mickey/mibee-eye/deploy/bin`. Without it, camera capture fails silently.
4. Camera is **exclusive** — only one process can hold `/dev/video0`. The systemd unit has `Conflicts=mediamtx.service` to prevent conflicts.
5. **Port 9100 conflict**: metrics defaults to :9100 but Prometheus node_exporter is already there. Set `metrics.enabled: false` or change port.

## Architecture

```
mtxrpicam subprocess (CSI capture, H.264 encode)
    ↓ binary pipe (4-byte LE framed)
camera.RPiCamera → h264.Parser → [SPS/PPS cache + IDR injection]
    ↓
h264.AUHub (fan-out, non-blocking drop)
    ↓ ↓ ↓ ↓ ↓ ↓
RTSP   Web/MSE   Snapshot   HLS*   RTMP*   GB28181
(gortsplib v5)   (rpicam-still + IDR fallback)   (*disabled by default)   (SIP + RTP push)

ONVIF SOAP server (:8080) — Device, Media, Imaging services
WS-Discovery UDP multicast (239.255.255.250:3702)
GB28181 SIP server (optional) — SIP signaling, RTP PS stream push
Web Admin UI (:8088) — MSE live preview, imaging controls, config editor
```

### Startup wiring order (`cmd/server/main.go`)

1. Camera (mtxrpicam subprocess or external RTSP source)
2. H264 Parser + AUHub + SnapshotBuffer + SPS/PPS injection goroutine
3. RTSP server (gortsplib v5, skipped when consuming external RTSP)
4. ParamManager (validates ONVIF imaging params, maps to camera)
5. ONVIF server (Device + Media + Imaging + Snapshot handlers)
6. WS-Discovery (UDP multicast responder)
6.5. GB/T 28181 device (optional, SIP registration + RTP PS stream push)
7. Web UI (optional, enabled by default)
8. HLS (optional, disabled by default)
9. RTMP push (optional, disabled by default)
10. Metrics (optional, enabled by default)

**Non-obvious**: main.go caches SPS/PPS NALUs and injects them before IDR frames that lack them. mtxrpicam only sends SPS/PPS on the first frame, not on IDR refreshes — without injection, RTSP clients that connect mid-stream get video corruption.

## Camera Subsystem

Two implementations of `camera.Camera` interface:
- **`RPiCamera`** (default, `camera.Mode: "mtxrpicam"`): spawns mtxrpicam C binary as subprocess
- **`RTSPSource`** (`camera.Mode: "rtsp"`): consumes an external RTSP URL, for testing without camera hardware

### mtxrpicam wire protocol (`internal/camera/pipe.go`)

- 4-byte LE length prefix → payload
- Config command: `'c'` prefix, Quit: `'e'`
- Video frame: `'b'` (v1.11.3, 1-byte flags + NALU) **or** `'d'` (master branch, 8-byte DTS + NALU) — both supported
- Ready signal: `'r'` (waited up to 10s after subprocess start)
- Error: `'e'`
- 30s read timeout without frames → subprocess restart
- Pipes created via `syscall.Pipe` (NOT `os.Pipe` — FD_CLOEXEC would close on fork)
- Subprocess in separate process group (`Setpgid: true`)

### Param serialization (`internal/camera/params_serialize.go`)

Reflection-based: iterates `Params` struct fields → `"FieldName:Value ..."` pairs. Strings base64-encoded, bools as 0/1. Supported types: `uint32`, `float32`, `string`, `bool` — panics on others.

Param validation: `ParamRanges` (numeric min/max/default) and `ParamEnums` (allowed string values) in `manager.go`. ONVIF PascalCase names mapped to camera lowercase via `onvifToCam` map.

## ONVIF Server

Hand-written SOAP, no external library. Services: **Device, Media, Imaging** (no PTZ — removed as dead code, no camera wiring).

Key conventions:
- Response types use explicit namespace prefixes: `tds:`, `trt:`, `timg:`, `tt:`
- Request parsing uses namespace-agnostic `unmarshalAnyNS()` (ONVIF clients vary prefixes)
- Discovery ProbeMatches XML built as **raw string bytes** (not `encoding/xml`) — NVR depends on exact element local names
- Auth: `isAuthRequired()` checks action prefix (`Set*`, `Remove*`, `Create*`, `Go*`). Read operations are open.
- Per-request IP: ONVIF responses echo the NVR's source IP as XAddr host (not the device's own IP), so URLs are reachable from whichever interface was queried

**NEVER** change `GetStreamUriResponse` element names (`MediaUri`, `Uri`) or ProbeMatches structure — MiBee NVR uses raw SOAP parsing with local-name matching.

## NVR Integration

**Hard constraint**: MiBee NVR (`/home/mickey/Projects/iot/mibee-nvr`) is **READ-ONLY**. Never modify any file there.

NVR calls these ONVIF operations:
- WS-Discovery Probe → ProbeMatches with scopes (`/name/`, `/hardware/`)
- `GetDeviceInformation` → manufacturer, model, firmware, serial
- `GetCapabilities` → Device: true, Media: true, Imaging: true, **PTZ: false**
- `GetProfiles` → at least 1 profile with VideoEncoder (H264, 1280x720)
- `GetStreamUri` → `rtsp://<ip>:8554/stream`

NVR auto-selects first profile, probes RTSP DESCRIBE for actual encoding, has raw SOAP fallback for GetStreamUri. NVR test servers: Docker VM `192.168.63.197`, RPi `192.168.63.31`.

## Config

YAML at `~/mibee-eye/config.yaml`. Env vars override with `MIBEE_EYE_<SECTION>_<FIELD>` pattern. Sections: `camera`, `rtsp`, `onvif`, `device`, `web`, `metrics`, `snapshot`, `rtmp`, `hls`, `logging`.

ONVIF password is **required** — service refuses to start if empty. Set via `onvif.password` in config or `MIBEE_EYE_ONVIF_PASSWORD` env var.

## Conventions

- **Error wrapping**: `fmt.Errorf("context: %w", err)` everywhere
- **Thread safety**: `sync.RWMutex` on all shared state (camera params, AUHub, snapshot buffer)
- **Non-blocking drops**: AUHub.Write and camera readLoop drop frames if consumers are slow — never blocks the producer
- **ONVIF naming**: PascalCase in ONVIF handlers → lowercase camera param via `onvifToCam` map
- **Logging**: `log/slog` (JSON to stderr), level configurable via `logging.level`
- **Commit style**: Semantic — `feat(scope):`, `fix(scope):`, `refactor(scope):`, `chore(scope):`, `docs:`
- **Sub-AGENTS.md**: `internal/onvif/AGENTS.md` and `internal/camera/AGENTS.md` exist (gitignored) with package-specific deep-dive details

## Anti-Patterns

- **NEVER** modify files in `/home/mickey/Projects/iot/mibee-nvr/` — READ-ONLY
- **NEVER** use `os.Pipe()` for mtxrpicam subprocess — use `syscall.Pipe()` and clear `FD_CLOEXEC`
- **NEVER** change mtxrpicam wire protocol without updating both Go code AND the C binary
- **NEVER** change ONVIF XML response element names without testing against NVR
- **NEVER** suppress types with `interface{}` bypass — use proper type assertions via `toFloat64()`
- **NEVER** hold camera mutex during I/O (pipe write) — current `SetParam` does hold lock during write (known issue, be aware)

## Where To Look

| Task | Location |
|------|----------|
| Add ONVIF SOAP action | `internal/onvif/` — response type in service file, register via `s.RegisterAction()` in `Register*Handlers()` |
| Add camera parameter | `camera/params.go` (struct field) + `camera.go:mapParamName()` (name mapping) + `manager.go:ParamRanges` (validation) |
| Change RTSP behavior | `internal/rtsp/server.go` (wraps gortsplib v5) |
| Change imaging mappings | `internal/onvif/imaging.go` → `camera.ParamManager` |
| Add config field | `internal/config/config.go` — struct field + YAML tag + `DefaultConfig()` + `applyEnvOverrides()` |
| Fix discovery | `internal/onvif/discovery.go` (UDP multicast + HTTP POST probe) |
| Fix snapshot | `internal/onvif/snapshot.go` (SnapshotBuffer stores latest IDR; dual-tier: rpicam-still + H.264 IDR fallback) |
| Fix auth | `internal/onvif/auth.go` (WS-UsernameToken: PasswordText + PasswordDigest SHA1) |
| Change web UI | `internal/web/static/` (app.js, index.html, style.css — embedded via `//go:embed`) |
| Fix SPS/PPS injection | `cmd/server/main.go` (goroutine in Step 2, caches SPS/PPS, injects before IDR) |
| Add GB28181 settings | `internal/gb28181/` (SIP server, RTP push, PS mux) + `internal/config/config.go` (GB28181Config struct) + `internal/web/static/app.js` (settings panel) |
