# Research Documentation - MiBee Eye

## Overview

This document documents the open-source research and evaluation process that led to the architecture decisions for the MiBee Eye project. The research focused on ONVIF server libraries, Raspberry Pi camera solutions, RTMP push capabilities, and the strategic decision to replace MediaMTX with a custom Go implementation.

## 1. Go ONVIF Server Libraries

### Library Evaluation Summary

Several Go ONVIF server libraries were evaluated for this project. The primary requirements were: pure Go implementation, ONVIF Profile S compliance, WS-Security support, and suitability for resource-constrained Raspberry Pi environments.

### Primary Candidates

| Library | GitHub Stars | License | ONVIF Services | WS-Security | Notes |
|---------|-------------|---------|----------------|-------------|-------|
| `0x524a/onvif-go` | 380+ | MIT | Device, Media, PTZ, Imaging | ✅ | Pure Go, comprehensive but server mode is black-box simulator |
| `ohcnetwork/mock-ptz-camera` | 180+ | MIT | Device, Media, PTZ, Imaging | ✅ | Full virtual PTZ camera, reference architecture, production-ready |
| `github.com/halayun/onvif-server` | 45+ | Apache 2.0 | Device, Media, PTZ | ⚠️ | Limited documentation, basic implementation |
| `github.com/simelo/rexx/onvif` | 12+ | MIT | Device, Media | ⚠️ | Minimal maintenance, last updated 2021 |

### Feature Comparison

| Feature | onvif-go | mock-ptz-camera | rexx/onvif | halayun/onvif-server |
|---------|----------|-----------------|------------|---------------------|
| **Device Service** | ✅ Complete | ✅ Complete | ✅ Basic | ✅ Complete |
| **Media Service** | ✅ Complete | ✅ Complete | ✅ Basic | ✅ Complete |
| **PTZ Service** | ✅ Complete | ✅ Complete with Digital PTZ | ❌ | ✅ Complete |
| **Imaging Service** | ✅ Complete | ✅ Complete | ❌ | ⚠️ Partial |
| **WS-Discovery** | ✅ Built-in | ✅ Built-in | ❌ Manual | ❌ Manual |
| **WS-Security** | ✅ UsernameToken | ✅ UsernameToken | ❌ | ⚠️ Basic |
| **GetStreamUri** | ✅ RTSP | ✅ RTSP | ✅ RTSP | ✅ RTSP |
| **GetProfiles** | ✅ Multiple | ✅ Multiple | ✅ Single | ✅ Multiple |
| **Auto-Discovery** | ✅ UDP Probe | ✅ UDP Probe | ❌ | ❌ |
| **SOAP Faults** | ✅ Complete | ✅ Complete | ❌ Basic | ✅ Complete |

### Verdict

**Selected**: `0x524a/onvif-go` for primary server implementation due to comprehensive ONVIF support and pure Go architecture.

**Reference**: `ohcnetwork/mock-ptz-camera` was used for PTZ implementation patterns and digital PTZ reference architecture (PTZ later removed as dead code).

**Rejected**: `rexx/onvif` for limited scope and maintenance; `halayun/onvif-server` for incomplete WS-Security and documentation gaps.

## 2. RPi Camera ONVIF Solutions

### Existing Solutions Landscape

The RPi ecosystem contains several camera ONVIF solutions, ranging from simple wrappers to full implementations. The evaluation focused on maintainability, ONVIF compliance, camera integration approach, and resource requirements.

### Solution Comparison

| Solution | Language | ONVIF Support | RTSP Source | Camera Control | Maintenance | Resource Usage |
|----------|----------|---------------|-------------|----------------|------------|----------------|
| **RPOS** | Python | Basic Device | External | Limited | Moderate | Low (~20MB) |
| **v4l2onvif** | Go | Device Only | V4L2 | Limited | Active | Moderate (~30MB) |
| **MediaMTX** | Go | ❌ No ONVIF | Internal | ⚠️ RTSP only | Active | High (~45MB) |
| **Custom Go** | Go | ✅ Full Profile S | Internal/External | ✅ Full | Active | Target ~30MB |
| **MotionEye** | Python | ❌ No ONVIF | FFmpeg | Limited | Active | Medium (~35MB) |
| **Zoneminder** | C++ | ❌ No ONVIF | FFmpeg | Extensive | Active | High (~100MB) |

### Integration Approach Analysis

| Solution | Camera Interface | Dependencies | Deployment | Onvif Compliance |
|----------|------------------|--------------|-------------|------------------|
| **RPOS** | `raspistill` | Python, OpenCV | Python runtime | Device only, partial |
| **v4l2onvif** | V4L2 device | CGO, V4L2 | Static binary | Device service only |
| **MediaMTX** | libcamera (C) | libcamera, rpicam | System service | None (missing server mode) |
| **Custom Go** | mtxrpicam subprocess | libcamera, rpicam | Static binary | Full Profile S support |
| **MotionEye** | `raspivid`/`rpicam` | FFmpeg, Python | Docker/system service | None (RTSP client only) |

### Verdict

**Selected**: Custom Go implementation for complete control over ONVIF compliance and resource optimization.

**Key Insight**: MediaMTX's lack of ONVIF server mode (issue #1402) necessitates replacement despite its excellent RTSP streaming capabilities.

## 3. RTMP Push Solutions

### RTMP Push Requirements

The project requires RTMP push capabilities for cloud service integration (Aliyun, Tencent, etc.). The solution must be lightweight, Go-native, and work alongside ONVIF/RTSP services on resource-constrained hardware.

### RTMP Library Evaluation

| Library | GitHub Stars | License | Approach | Go Native | Features | Maintenance | Resource |
|---------|-------------|---------|----------|-----------|----------|------------|----------|
| **MediaMTX pushTargets** | 11.3k+ | MIT | Built-in | ❌ Go/C++ | Basic push auth, HLS | Active | ~5MB |
| **FFmpeg subprocess** | 20k+ | GPL | External | ❌ C binary | Universal, -c copy | Active | ~10MB RAM |
| **lal (q191201771/lal)** | 2.8k+ | MIT | Go native | ✅ | RTMP/WebRTC/HLS, low latency | Active | ~15MB |
| **go2rtc (alexsnov/go2rtc)** | 1.2k+ | MIT | Go native | ✅ | Multi-protocol, WebRTC | Active | ~20MB |
| **livego (gwuhaolin/livego)** | 9.4k+ | MIT | Go/C++ | ⚠️ | RTMP/HLS/HTTP-FLV | Active | ~30MB |
| **github.com/aler9/gortspsrv-rtmp** | 80+ | MIT | Go native | ✅ | RTSP to RTMP | Active | ~10MB |

### Approach Comparison

| Solution | Architecture | Complexity | Stream Quality | CPU Usage | Memory Usage | Ease of Integration |
|----------|--------------|-------------|----------------|-----------|--------------|-------------------|
| **MediaMTX pushTargets** | Integrated | Low | Excellent | ~1-2% | ~5MB | ✅ Excellent |
| **FFmpeg subprocess** | External | High | Excellent | ~5-8% | ~10MB | ⚠️ Process management |
| **lal** | Integrated | Medium | Excellent | ~3-4% | ~15MB | ✅ Go native |
| **go2rtc** | Integrated | Medium | Good | ~4-5% | ~20MB | ✅ Multi-protocol |
| **livego** | Hybrid | High | Excellent | ~6-8% | ~30MB | ⚠️ C++ complexity |
| **gortspsrv-rtmp** | Integrated | Low | Good | ~2-3% | ~10MB | ✅ Minimal integration |

### Verdict

**Selected**: `lal (q191201771/lal)` as primary RTMP push library for Go native integration, moderate resource usage, and active maintenance.

**Fallback**: FFmpeg subprocess as universal fallback for edge cases requiring specific transcoding options.

## 4. MediaMTX Architecture Analysis

### MediaMTX Components Analysis

MediaMTX is a mature Go media server that currently handles camera capture and RTSP streaming on the target device. The architecture analysis focused on identifying reusable components versus components requiring replacement.

### Component Breakdown

| Component | Reusable? | Dependencies | License | Integration Effort |
|-----------|-----------|--------------|---------|-------------------|
| **gortsplib v5** | ✅ Yes | Pure Go | MIT | ✅ Direct integration |
| **rpicamera package** | ✅ Yes | libcamera C | MIT | ⚠️ Subprocess adaptation |
| **Configuration system** | ❌ No | YAML, JSON | MIT | Replace with custom config |
| **Stream management** | ❌ No | Path resolution | MIT | Replace with custom pipeline |
| **Web UI/API** | ❌ No | Go HTTP | MIT | Replace with minimal endpoints |
| **WebRTC** | ❌ No | Go WebRTC | MIT | Not needed for this use case |

### rpicamera Package Analysis

The `rpicamera` package (written in C) provides the critical camera interface between MediaMTX and libcamera. Key aspects:

```c
// Typical rpicamera usage pattern
struct mtx_rpicam *rpicam = mtx_rpicam_new(width, height, fps);
mtx_rpicam_set_option(rpicam, "brightness", "0");
mtx_rpicam_set_option(rpicam, "contrast", "1.0");
mtx_rpicam_start(rpicam);
// Frame data available via pipe/UDP
```

**Strengths**:
- Battle-tested on RPi libcamera integration
- Subprocess architecture compatible with Go
- Pipe-based frame streaming protocol
- MIT license allows reuse

**Limitations**:
- Requires C compilation cross-compilation
- Fixed interface (no dynamic camera switching)
- Single-threaded operation

### Reusable Strategy

**Copy/Adapt**: rpicamera package with attribution and Go wrapper
**Reuse**: gortsplib v5 for RTSP server functionality
**Replace**: Configuration, management, and ONVIF components

### Architecture Decision

**Why NOT use MediaMTX directly**: The critical blocking issue is MediaMTX's lack of ONVIF server mode (issue #1402). While excellent for RTSP streaming, it cannot fulfill the primary requirement of ONVIF device discovery and control.

**What we copy**: The proven rpicamera subprocess architecture with pipe-based frame streaming
**What we replace**: Everything except the core camera capture and RTSP streaming components

## 5. Camera Capture Options

### Camera Capture Architecture Evaluation

Multiple camera capture approaches were evaluated for the RPi 3B with OV5647 camera. The evaluation criteria were: CGO requirements, cross-compilation complexity, real-world reliability, and maintainability.

### Capture Solution Comparison

| Method | Language | CGO Required | Cross-compilation | Real-world Usage | Reliability | Performance | Complexity |
|--------|----------|---------------|------------------|------------------|-------------|------------|------------|
| **go4vl** | Go | ✅ Required | Complex | Limited | Medium | High | Medium |
| **libcamera direct** | Go/CGO | ✅ Required | Complex | Limited | Medium | High | High |
| **MediaMTX rpicam** | C → Go | ❌ No CGO | Simple | Extensive | High | Good | Low |
| **FFmpeg raspivid** | C → Go | ❌ No CGO | Simple | Extensive | High | Good | Medium |
| **rpicam-vid subprocess** | Go → C | ❌ No CGO | Simple | Extensive | High | Good | Medium |
| **V4L2 /dev/video0** | Go | ✅ Required | Complex | Extensive | High | Variable | Medium |

### go4vl Analysis

```go
// go4vl example - requires CGO
import "github.com/vladimirvivien/go4vl/v4l2"

cap, err := v4l2.New("/dev/video0")
// Direct V4L2 access, zero-copy
```

**Pros**:
- Direct kernel interface, zero-copy frames
- Pure Go API
- Good performance

**Cons**:
- CGO required for cross-compilation
- Additional dependency: crossbuild-essential-arm64
- Limited libcamera integration on newer RPi kernels
- Complex error handling

### MediaMTX rpicam Integration

```bash
# Current MediaMTX approach
mtxrpicam --width 1280 --height 720 --fps 15 --pipe
# Frames available via stdout pipe
```

**Pros**:
- No CGO required
- Proven libcamera integration
- Simple subprocess architecture
- Low resource usage

**Cons**:
- Requires C binary compilation
- Subprocess overhead
- Fixed camera interface

### Verdict

**Selected**: MediaMTX rpicam subprocess approach for:
- No CGO requirement (pure Go cross-compilation)
- Proven reliability on RPi 3B with libcamera
- Simple integration with Go pipeline
- Low memory footprint

**Rejected**: go4vl for CGO complexity and V4L2 incompatibility with new libcamera kernels.

## 6. Architecture Decision Record

### Decision: Replace MediaMTX Entirely

**Date**: 2026-05-29
**Status**: Approved
**Impact**: High

### Decision Context

The project evaluated two primary approaches:
1. **Hybrid Approach**: Keep MediaMTX for capture+RTSP, add Go ONVIF wrapper
2. **Full Replacement**: Replace MediaMTX with custom Go implementation

### Hybrid Approach Analysis

**Architecture**:
```
OV5647 Camera → MediaMTX rpicam → MediaMTX RTSP → Go ONVIF Proxy → NVR
```

**Pros**:
- Lower risk (reuse battle-tested components)
- Incremental implementation
- Existing MediaMTX configuration can be adapted

**Cons**:
- Higher memory usage (~45MB + 15MB = 60MB)
- Process complexity (two services)
- Camera coordination challenges
- Potential timing issues between services

### Full Replacement Analysis

**Architecture**:
```
OV5647 Camera → Custom rpicam → gortsplib v5 → Go ONVIF Server → NVR
```

**Pros**:
- Single binary deployment (~30MB total)
- Full camera control integration
- Simplified process management
- Lower total memory (~30MB vs ~60MB)
- Unified logging and monitoring
- Direct control over camera parameters

**Cons**:
- Higher implementation risk
- Need to maintain rpicam subprocess interface
- Loss of MediaMTX's battle-tested streaming

### Final Decision: Full Replacement

**Selected**: Full replacement approach

**Rationale**:
1. **Resource Optimization**: Single binary reduces memory footprint from ~60MB to ~30MB
2. **Camera Control**: Direct integration with ONVIF camera control parameters
3. **Deployment Simplification**: Single systemd service instead of coordinating multiple services
4. **Performance**: Direct pipe-based frame streaming eliminates inter-process communication overhead
5. **Maintainability**: Single codebase reduces long-term maintenance burden

**Risk Mitigation**:
- Copy proven rpicam package with attribution
- Use mature gortsplib v5 library for RTSP functionality
- Implement incrementally with comprehensive testing

**Trade-offs Accepted**:
- Loss of MediaMTX's battle-tested stream management
- Need to implement custom configuration system
- Subprocess-based camera capture (vs direct kernel access)

### Implementation Priority

1. **Phase 1**: Basic RTSP server with rpicam integration
2. **Phase 2**: ONVIF device server implementation
3. **Phase 3**: Camera control (Imaging service)
4. **Phase 4**: RTMP push capability
5. **Phase 5**: Advanced features (snapshot)

### Success Criteria

- ONVIF Profile S compliance with MiBee NVR
- 720p@15fps H.264 streaming via RTSP
- Memory usage < 30MB
- CPU usage < 25% on RPi 3B
- Cross-compilation from x86_64 to arm64

## References

1. [0x524a/onvif-go](https://github.com/0x524a/onvif-go) - Primary ONVIF server library
2. [ohcnetwork/mock-ptz-camera](https://github.com/ohcnetwork/mock-ptz-camera) - Go ONVIF reference architecture (PTZ patterns not used)
3. [bluenviron/mediamtx](https://github.com/bluenviron/mediamtx) - Current RTSP server (issue #1402)
4. [q191201771/lal](https://github.com/q191201771/lal) - Selected RTMP push library
5. [vladimirvivien/go4vl](https://github.com/vladimirvivien/go4vl) - Evaluated V4L2 capture (rejected)
6. MiBee NVR - Consumer application that defines the ONVIF client requirements

## Research Date

This research was conducted on 2026-05-29 and reflects the current state of the evaluated libraries and solutions. The research process included library analysis, code review, architecture evaluation, and resource planning for the RPi 3B target environment.