# 配置文档

[English](../configuration.md)

MiBee Eye 配置采用 YAML 格式，控制摄像头服务的所有方面，包括捕获设置、流媒体协议和设备标识。

## 配置文件

### 文件位置
配置默认从 `configs/config.yaml` 加载。通过复制 `configs/config.example.yaml` 并根据您的设置进行修改来创建此文件。

### 文件格式
```yaml
# 注释使用 # 符号
# 顶级部分定义功能区域
camera:        # 摄像头捕获设置
rtsp:          # RTSP 流媒体服务器
onvif:         # ONVIF 设备服务
device:        # 设备标识
logging:       # 日志配置
web:           # Web UI 配置
metrics:       # Prometheus 指标导出器
snapshot:      # JPEG 快照端点
rtmp:          # RTMP 推送流媒体
hls:           # HLS 实时流媒体
```

## 配置部分

### 摄像头配置

摄像头捕获设置控制如何从摄像头设备捕获视频帧。

```yaml
camera:
  # 捕获模式："mtxrpicam"（默认，使用子进程）或 "rtsp"（消费外部 RTSP URL）
  mode: mtxrpicam

  # mtxrpicam 二进制文件路径（摄像头捕获子进程）
  # 此二进制文件及其捆绑的 libcamera 库必须存在于该路径下
  bin_path: deploy/bin/mtxrpicam

  # 摄像头设备路径（V4L2 或 libcamera）
  device: /dev/video0
  
  # 捕获分辨率（宽度 x 高度）
  # 支持的分辨率：640x480, 1296x972, 1920x1080, 2592x1944
  width: 1280
  height: 720
  
  # 每秒帧数（OV5647 传感器最大 30）
  fps: 15
  
  # 视频编解码器（h264 或 h265）
  codec: h264
  
  # 目标比特率（每秒位数）
  # 示例：2000000 = 2 Mbps
  bitrate: 2000000
  
  # 图像控制（硬件特定范围适用）
  # 亮度：-1.0 到 1.0（0.0 = 默认值，负值 = 更暗，正值 = 更亮）
  brightness: 0.0
  
  # 对比度：0.0 到 32.0（1.0 = 默认值）
  contrast: 1.0
  
  # 饱和度：0.0 到 32.0（1.0 = 默认值）
  saturation: 1.0
  
  # 锐度：0.0 到 16.0（1.0 = 默认值）
  sharpness: 1.0
  
  # mode=rtsp 时的外部 RTSP URL（mode=mtxrpicam 时忽略）
  rtsp_url: ""
  
  # 关键帧间隔（1=每帧，15=每15帧）
  idr_period: 15
  
  # 帧通道缓冲容量（帧数）
  frame_buffer_size: 30
  
  # 最大子进程重启退避持续时间
  max_backoff: 30s
```

### RTSP 配置

RTSP 服务器设置用于视频流客户端。

```yaml
rtsp:
  # RTSP 服务器端口（默认：8554）
  port: 8554
  
  # 可选的 RTSP 身份验证
  # 留空字符串表示无身份验证
  username: ""
  password: ""
  
  # AUHub 订阅者通道缓冲大小
  subscriber_buffer_size: 64
  
  # gortsplib 写队列大小（256默认对WiFi来说太小）
  write_queue_size: 2048
  
  # 启用 UDP 传输（NVR客户端需要）
  enable_udp: true
  
  # UDP RTP 端口（默认：8000）
  udp_rtp_port: 8000
  
  # UDP RTCP 端口（默认：8001）
  udp_rtcp_port: 8001
```

### ONVIF 配置

ONVIF 服务器设置，用于通过 NVR 系统进行设备发现和控制。

```yaml
onvif:
  # ONVIF HTTP/SOAP 端口（默认：8080）
  port: 8080
  
  # ONVIF WS-UsernameToken 身份验证
  # MiBee NVR 集成必需
  username: "admin"
  
  # ONVIF 密码（生产环境必须设置）
  password: ""
```

### Web UI 配置

Web UI 设置用于内置的浏览器管理面板，提供实时预览和摄像头配置功能。

```yaml
web:
  # 启用 Web 管理界面（默认：true）
  enabled: true

  # Web UI HTTP 端口（默认：8088）
  port: 8088

  # Web UI 身份验证
  # 当用户名/密码为空时使用 ONVIF 凭据
  username: "admin"
  password: ""
  
  # CORS 允许的来源（生产环境使用特定来源）
  allowed_origins:
    - "*"
  
  # HTTP 服务器超时
  read_header_timeout: 5s
  read_timeout: 10s
  write_timeout: 30s
  idle_timeout: 120s
```


### RTMP 配置

RTMP 推送设置，用于流式传输到云服务。

```yaml
rtmp:
  # 启用 RTMP 推送流媒体
  enabled: false
  
  # 云服务的 RTMP 推送 URL
  # 示例：
  # - rtmp://push-server/app/stream
  # - rtmp://live.twitch.tv/app/channel-key
  url: "rtmp://push-server/app/stream"
  
  # 最大重连尝试次数（0 = 无限制）
  max_retries: 10
```

### 设备配置

通过 ONVIF GetDeviceInformation 服务公开的设备信息。

```yaml
device:
  # NVR 中显示的友好摄像头名称
  name: "Pi Camera V1"
  
  # 设备制造商
  manufacturer: "Raspberry Pi"
  
  # 摄像头传感器型号
  model: "OV5647"
  
  # 固件版本字符串
  firmware: "1.0.0"
  
  # 硬件标识符
  hardware_id: "OV5647"
  
  # 序列号（如果不可用则为空）
  serial_number: ""
```

### 日志配置

用于调试和监控的日志设置。

```yaml
logging:
  # 日志级别（debug, info, warn, error）
  # debug：最详细，包含所有调试消息
  # info：标准操作日志
  # warn：仅警告和错误
  # error：仅错误
  level: "info"
```

### Snapshot 配置

Snapshot 端点设置，用于通过 HTTP 进行 JPEG/H.264 捕获。

```yaml
snapshot:
  # 启用 snapshot 端点（默认：true）
  enabled: true

  # JPEG 质量 1-100（仅用于 rpicam-still 子进程；H.264 IDR 回退忽略此设置）
  quality: 85
```

Snapshot 端点使用双层策略：
1. **第一层**：当摄像头空闲时，`rpicam-still` 子进程捕获真实 JPEG
2. **第二层**：当摄像头管道忙碌时，回退到存储的 H.264 IDR 帧（返回为 `video/H264`）

### Metrics 配置

Prometheus 指标导出器设置。

```yaml
metrics:
  # 启用指标 HTTP 端点（默认：true）
  enabled: true

  # 指标 HTTP 服务器端口（默认：9100）
  # 注意：9100 与 Prometheus node_exporter 冲突 — 如果两者在同一主机上运行，请更改端口或禁用
  port: 9100
```

### HLS 配置

HLS 实时流设置，用于浏览器播放。

```yaml
hls:
  # 启用 HLS 服务器（默认：false）
  enabled: false

  # 目标分段持续时间（默认：2s）
  segment_duration: 2s
```

HLS 服务器使用纯 Go MPEG-TS 分段器 — 无 ffmpeg 子进程。分段保存在内存中。


## 默认值参考

| 部分 | 字段 | 默认值 | 类型 | 描述 |
|------|------|--------|------|------|
| **camera** | mode | `"mtxrpicam"` | string | 捕获模式（mtxrpicam 或 rtsp） |
| | bin_path | `"deploy/bin/mtxrpicam"` | string | mtxrpicam 二进制文件路径 |
| | device | `/dev/video0` | string | 摄像头设备路径 |
| | width | `1280` | int | 捕获宽度（像素） |
| | height | `720` | int | 捕获高度（像素） |
| | fps | `15` | int | 每秒帧数 |
| | codec | `"h264"` | string | 视频编解码器 |
| | bitrate | `2000000` | int | 比特率（每秒位数） |
| | brightness | `0.0` | float | 亮度控制 |
| | contrast | `1.0` | float | 对比度控制 |
| | saturation | `1.0` | float | 饱和度控制 |
| | sharpness | `1.0` | float | 锐度控制 |
| | rtsp_url | `""` | string | 外部 RTSP URL（mode=rtsp 时使用） |
| | idr_period | `15` | int | 关键帧间隔 |
| | frame_buffer_size | `30` | int | 帧通道缓冲容量 |
| | max_backoff | `"30s"` | string | 最大子进程重启退避 |
| **rtsp** | port | `8554` | int | RTSP 服务器端口 |
| | username | `""` | string | RTSP 用户名 |
| | password | `""` | string | RTSP 密码 |
| | subscriber_buffer_size | `64` | int | AUHub 订阅者通道缓冲大小 |
| | write_queue_size | `2048` | int | gortsplib 写队列大小 |
| | enable_udp | `true` | bool | 启用 UDP 传输 |
| | udp_rtp_port | `8000` | int | UDP RTP 端口 |
| | udp_rtcp_port | `8001` | int | UDP RTCP 端口 |
| **onvif** | port | `8080` | int | ONVIF HTTP 端口 |
| | username | `"admin"` | string | ONVIF 用户名 |
| | password | `""` | string | ONVIF 密码 |
| **device** | name | `"Pi Camera V1"` | string | 友好摄像头名称 |
| | manufacturer | `"Raspberry Pi"` | string | 设备制造商 |
| | model | `"OV5647"` | string | 摄像头传感器型号 |
| | firmware | `"1.0.0"` | string | 固件版本 |
| | hardware_id | `"OV5647"` | string | 硬件标识符 |
| | serial_number | `""` | string | 设备序列号 |
| **logging** | level | `"info"` | string | 日志级别 |
| **web** | enabled | `true` | bool | 启用 Web UI |
| | port | `8088` | int | Web UI HTTP 端口 |
| | username | `""` | string | Web UI 用户名（默认使用 onvif.username） |
| | password | `""` | string | Web UI 密码（默认使用 onvif.password） |
| | allowed_origins | `["*"]` | []string | CORS 允许的来源 |
| | read_header_timeout | `"5s"` | string | HTTP 读取头超时 |
| | read_timeout | `"10s"` | string | HTTP 读取超时 |
| | write_timeout | `"30s"` | string | HTTP 写入超时 |
| | idle_timeout | `"120s"` | string | HTTP 空闲超时 |
| **metrics** | enabled | `true` | bool | 启用指标端点 |
| | port | `9100` | int | 指标 HTTP 服务器端口 |
| **snapshot** | enabled | `true` | bool | 启用 snapshot 端点 |
| | quality | `85` | int | JPEG 质量 1-100 |
| **rtmp** | enabled | `false` | bool | 启用 RTMP 推送 |
| | url | `"rtmp://push-server/app/stream"` | string | RTMP 推送 URL |
| | max_retries | `10` | int | 最大重连尝试次数 |
| **hls** | enabled | `false` | bool | 启用 HLS 服务器 |
| | segment_duration | `"2s"` | string | 目标分段持续时间 |
## 环境变量覆盖

所有配置值都可以使用 `MIBEE_EYE_` 前缀的环境变量覆盖。这对于部署、测试和容器化环境很有用。

### 格式
环境变量遵循模式：`MIBEE_EYE_<部分>_<字段>`

### 示例
```bash
# 覆盖摄像头分辨率
MIBEE_EYE_CAMERA_WIDTH=1920 MIBEE_EYE_CAMERA_HEIGHT=1080 ./mibee-eye

# 为生产环境设置 ONVIF 密码
MIBEE_EYE_ONVIF_PASSWORD=securepassword123 ./mibee-eye

# 更改 RTSP 端口
MIBEE_EYE_RTSP_PORT=554 ./mibee-eye

# 启用调试日志

MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye

# Web UI 访问和凭据
MIBEE_EYE_WEB_ENABLED=true ./mibee-eye

# 设置 Web UI 凭据（独立于 ONVIF）
MIBEE_EYE_WEB_USERNAME=admin MIBEE_EYE_WEB_PASSWORD=webpass ./mibee-eye
# 为生产环境设置 ONVIF 密码
MIBEE_EYE_ONVIF_PASSWORD=securepassword123 ./mibee-eye
# 设置设备信息
MIBEE_EYE_DEVICE_NAME="Office Camera" ./mibee-eye
```
### 所有环境变量

| 部分 | 字段 | 环境变量 |
|------|------|----------|
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
## 示例配置

### 基本配置（默认设置）
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

### 高分辨率配置
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

### 云流媒体配置
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


### 低带宽配置
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


web:
  enabled: true
  port: 8088
  username: "admin"
  password: ""

## 配置提示

1. **摄像头兼容性**：并非所有分辨率和设置都与所有摄像头模块兼容。请使用您的特定摄像头硬件测试配置。

2. **性能**：更高的分辨率和比特率需要更多的 CPU 和带宽。在树莓派 3B 上，720p @ 15fps 是推荐的平衡点。

3. **安全性**：在生产环境中，始终为 ONVIF 身份验证设置强密码。

4. **网络**：RTSP 流媒体可能消耗大量带宽。确保您的网络基础设施能够处理所选的比特率。

5. **调试**：使用 `MIBEE_EYE_LOGGING_LEVEL=debug` 来解决配置问题。

6. **环境变量**：使用环境变量存储像密码这样的敏感数据，避免将它们存储在配置文件中。

7. **验证**：服务将根据硬件约束验证配置值。无效的设置将被记录或设置为默认值。

8. **Web UI 访问**：Web 管理面板可通过 http://<设备-ip>:8088/ 访问。使用 ONVIF 凭据（如果配置了特定的 Web 凭据则使用 Web 凭据）登录。

9. **摄像头二进制文件**：`bin_path` 必须指向有效的 mtxrpicam 二进制文件。该文件所在目录还必须包含捆绑的 libcamera 共享库（libcamera.so.9.9、libcamera-base.so.9.9）和 IPA 模块。详见部署文档。