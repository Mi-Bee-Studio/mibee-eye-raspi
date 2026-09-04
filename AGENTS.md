# AGENTS.md — MiBee Eye (蜂眼)

Go 1.26+ ONVIF camera service for single-board computers. Primary purpose: **test camera for MiBee NVR** (`/home/mickey/Projects/iot/mibee-nvr`). Implements ONVIF Device/Media/Imaging server + RTSP streaming via `mtxrpicam` subprocess (no CGO, no MediaMTX dependency at runtime).

## Device & Credentials

| Item | Value |
|------|-------|
| Host | `ssh mickey@192.168.63.118` (rpi3b-cam, sudo NOPASSWD). Previous host `.162` is offline. |
| Board | RPi 3B, aarch64, 4 cores @1.2GHz, 905MB RAM (~565MB available) |
| Camera | IMX219 CSI module via system `rpicam-vid` (camera mode `rpicamvid`) |
| Network | WiFi only — **drops under sustained transfer**, 270Mbps theoretical |
| Binary | `/home/mickey/rpi3b-cam/rpi3b-cam-arm64` (device-local, NOT in repo) |
| Config | `/home/mickey/rpi3b-cam/configs/config.yaml` (device-local, NOT in repo; `-config` default is relative to WorkingDirectory) |
| Recordings | `/home/mickey/rpi3b-cam/recordings/` |
| Service | `systemctl {status,start,stop,restart} mibee-eye` — unit from `deploy/mibee-eye-118.service` (installed as `mibee-eye.service`, enabled, Restart=always) |

## Build & Deploy

```bash
# Cross-compile WITHOUT AI detection (pure Go, no CGO — any host)
GOOS=linux GOARCH=arm64 go build -o build/mibee-eye ./cmd/server

# Cross-compile WITH AI detection (SPEC §4.6; the deploy variant)
# CGO is required by onnxruntime_go (dlopen). Do NOT use `zig cc` here —
# zig-built CGO binaries crash with SIGILL inside dlopen on the Pi
# (verified 2026-09-04). Use the Arm GNU cross toolchain instead:
CGO_ENABLED=1 \
CC=$HOME/tools/arm-gnu-toolchain-13.2.Rel1-x86_64-aarch64-none-linux-gnu/bin/aarch64-none-linux-gnu-gcc \
GOOS=linux GOARCH=arm64 go build -tags ai -o build/rpi3b-cam-arm64 ./cmd/server

# NOTE: onnxruntime_go is pinned to v1.31.0 (requests ORT API 26). The
# deployed libonnxruntime.so is 1.28.0 (supports API ≤28); binding
# versions ≥v1.33 request API 29 and fail with "Error setting ORT API
# base: 2" — bump the pin only together with the runtime library.

# Test (default build) / with the AI tag
go test ./...
go vet -tags ai ./...
```

**AI runtime prerequisites on the device** (fail-open when missing): the
`ffmpeg` binary (Debian package, keyframe-only decode), the NanoDet model
at `[ai] model_path`, and `libonnxruntime.so` at `[ai] onnx_lib_path`
(aarch64 build; see `~/mibee-eye-raspi-rs/tmp/ort/`). Enable via the `[ai]`
config section. Detection cadence follows `camera.idr_period` (15 = 1s
keyframes at 15fps).

**Deploy gotchas** (learned the hard way):

1. `make deploy` is **broken** — tries to `scp configs/config.yaml` which doesn't exist (only `config.example.yaml` in repo). Deploy binary manually instead.
2. **WiFi drops under load** — upload the new binary as `.new`, then stop/swap/start:
   ```bash
   GOOS=linux GOARCH=arm64 go build -o build/rpi3b-cam-arm64 ./cmd/server
   gzip -c build/rpi3b-cam-arm64 | ssh mickey@192.168.63.118 'gunzip > ~/rpi3b-cam/rpi3b-cam-arm64.new && chmod +x ~/rpi3b-cam/rpi3b-cam-arm64.new'
   ssh mickey@192.168.63.118 'sudo systemctl stop mibee-eye && mv ~/rpi3b-cam/rpi3b-cam-arm64{.new,} && sudo systemctl start mibee-eye'
   ```
3. Camera mode on .118 is `rpicamvid` (system `rpicam-vid` + system libcamera) — no `mtxrpicam` binary or `LD_LIBRARY_PATH` needed. The legacy `mtxrpicam` mode notes below still apply to hosts that use it.
4. Camera is **exclusive** — only one process can hold `/dev/video0`. The systemd unit has `Conflicts=mediamtx.service` to prevent conflicts.
5. **Port 9100 conflict**: metrics defaults to :9100 but Prometheus node_exporter is already there. Set `metrics.enabled: false` or change port.

## Architecture

```
mtxrpicam subprocess (CSI capture, H.264 encode)
    ↓ binary pipe (4-byte LE framed)
camera.RPiCamera → h264.Parser → [SPS/PPS cache + IDR injection]
    ↓
h264.AUHub (fan-out, non-blocking drop)
    ↓ ↓ ↓ ↓ ↓ ↓ ↓
RTSP   Web/MSE   Snapshot   HLS*   RTMP*   GB28181   Recording*
(gortsplib v5)   (rpicam-still + IDR fallback)   (*disabled by default)   (SIP + RTP push, live/playback/download)   (H.264 segments + index.jsonl)

ONVIF SOAP server (:8080) — Device, Media, Imaging services
WS-Discovery UDP multicast (239.255.255.250:3702)
GB28181 SIP server (optional) — SIP signaling (UDP/TCP), RTP PS live/playback/download push, RecordInfo, SIP INFO playback control
Local recording (optional) — H.264 segments recordings/YYYY-MM-DD/HH/MMSS.h264 + append-only index.jsonl, retention + storage cap
Web Admin UI (:8088) — MSE live preview, imaging controls, config editor
```

### Unified Web API (SPEC v1, since 2026-08-31)

The web layer implements the MiBee camera unified SPEC (`../mibee-webui/SPEC.md`):
session-cookie auth + CSRF (`/api/auth/*`), `{"ok","data"}/{"ok","error","message"}` envelope,
`/api/cameras` resource model (fixed id "0"), server-side fMP4 over chunked HTTP
(`/api/cameras/0/stream.mse`), SSE `/api/events`, `PUT /api/config` deep-merge (restarts via
SIGTERM), capability superset. The embedded frontend is the shared mibee-webui build.
**`/snapshot` (open, for NVRs), HLS and :9100 metrics are unchanged device dialects.**

### Startup wiring order (`cmd/server/main.go`)

1. Camera (mtxrpicam subprocess or external RTSP source)
2. H264 Parser + AUHub + SnapshotBuffer + SPS/PPS injection goroutine
3. RTSP server (gortsplib v5, skipped when consuming external RTSP)
4. ParamManager (validates ONVIF imaging params, maps to camera)
5. ONVIF server (Device + Media + Imaging + Snapshot handlers)
6. WS-Discovery (UDP multicast responder)
6.4. Local recording (optional, disabled by default) — Writer subscribes AUHub, retention goroutine prunes by age/cap
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

YAML at `~/mibee-eye/config.yaml`. Env vars override with `MIBEE_EYE_<SECTION>_<FIELD>` pattern. Sections: `camera`, `rtsp`, `onvif`, `device`, `web`, `metrics`, `snapshot`, `rtmp`, `hls`, `gb28181`, `recording`, `logging`.

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
| Change web UI | `../mibee-webui/` (shared frontend source of truth) → `make sync-go` there copies into `internal/web/static/` (embedded via `//go:embed all:static`) |
| Fix SPS/PPS injection | `cmd/server/main.go` (goroutine in Step 2, caches SPS/PPS, injects before IDR) |
| Add recording settings | `internal/recording/` (Writer/Index/Retention) + `internal/config/config.go` (RecordingConfig) |
| Add GB28181 settings | `internal/gb28181/` (SIP server, RTP push, PS mux) + `internal/config/config.go` (GB28181Config struct) + `internal/web/static/app.js` (settings panel) |

## GB28181 Interop Notes (NVR-tested)

Real-world interop bugs discovered while testing against MiBee NVR
(192.168.63.197, SIP platform on port 5060). These are protocol-level
issues that unit tests don't catch — only surface with a real SIP stack
on the other end.

### Missing qop="auth" support
- **Symptom**: NVR rejects digest auth response with 403 Forbidden
  when it sends `qop="auth"` in the 401 challenge.
- **Root cause**: Without qop support, the response hash formula is wrong:
  should be `MD5(HA1:nonce:nc:cnonce:qop:HA2)` not `MD5(HA1:nonce:HA2)`.
- **Fix**: Pass qop through, generate cnonce, use nc=00000001.
- **Files**: `internal/gb28181/auth.go`

### Via branch not unique for authed REGISTER
- **Symptom**: NVR treats the authed REGISTER as a retransmission and
  replays the cached 401; registration never completes.
- **Root cause**: The Go code computed `via` once and reused it for both
  REGISTER messages. Same branch parameter means same request to NVR.
- **Fix**: Generate a new Via with different branch for the authed REGISTER.
- **Files**: `internal/gb28181/client.go`

### Local IP detection returns 0.0.0.0
- **Symptom**: NVR can't route SIP responses; 200 OK never arrives.
- **Root cause**: SIP socket binds to `0.0.0.0:5060`, so `socket.local_addr()`
  returns `0.0.0.0`. The NVR can't route responses to `0.0.0.0`.
- **Fix**: Create a temporary UDP socket, connect to platform address,
  read its local_addr() to get the real source IP.
- **Files**: `internal/gb28181/network.go`

### MANSCDP XML format: CmdType/SN must be attributes
- **Symptom**: NVR rejects MANSCDP XML messages with parse error.
- **Root cause**: NVR's `manscdp.Decode()` expects
  `<Notify CmdType="Keepalive" SN="1">` (attributes), not
  `<Notify><CmdType>Keepalive</CmdType><SN>1</SN>...` (child elements).
- **Fix**: Go adds `,attr` to struct tags (e.g., `xml:"CmdType,attr"`).
- **Files**: `internal/gb28181/manscdp.go`

### mtxrpicam libcamera ABI incompatibility
- **Symptom**: The mtxrpicam binary from mediamtx-rpicamera v2.8.0
  bundles libcamera.so.9.9 (custom soname). On Debian 13 trixie with
  system libcamera 0.7.1, the binary segfaults after camera init.
  Building from source against system libcamera fails due to meson wrap
  dependencies.
- **Root cause**: mtxrpicam uses a vendored libcamera with custom
  soname that conflicts with system libcamera on newer distros.
- **Fix**: New `rpicamvid` camera mode uses system `rpicam-vid` binary
  instead. Config: `camera.mode: "rpicamvid"`. No custom dependencies,
  uses stock libcamera stack.
- **Files**: `internal/camera/params.go` (mode enum), `internal/camera/rpicamvid.go` (new implementation), `cmd/server/main.go` (mode switch)

## Issue tracking

This repo's GitHub (https://github.com/Mi-Bee-Studio/mibee-eye-raspi/issues) is the **central issue tracker for BOTH Pi camera projects** — Go (`rpi3b-cam`) and Rust (`mibee-eye-raspi-rs`). NVR team files all discovered interop bugs here. Do not split issues across repos.

## 协议优先 + 全面 TDD（2026-09-01 起生效，HARD）

完整规范见工作区根 `AGENTS.md`（唯一真源）。要点：

- 本项目消费 `gb28181-go/device` + `onvif-go/v2`（git 依赖）。凡新增/改进功能涉及**协议层代码**（线格式、SOAP/SIP/MANSCDP 报文、摘要认证、SDP、RTP/PS、WS-Discovery 语义、协议级超时重传），**必须先在对应协议库 TDD 实现**（失败测试→实现→golden 契约→CI 绿→PR 合并），本项目只升 git pin + 写调用胶水。禁止在产品仓库复制或 patch 协议逻辑。
- 线上急修唯一例外：产品侧临时绕过须标注 `// HOTFIX(protocol-debt)` + 库仓库开 issue + 两个迭代内在库内正式实现并移除。
- 全面 TDD：测试与代码同提交、禁止事后补测；bug 修复先写复现失败测试；`go test`（含 `-race`）不过不提交。
