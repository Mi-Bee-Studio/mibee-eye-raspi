[English](../onvif-compliance.md)
# ONVIF 合规性参考

本文档提供了 mibee-eye ONVIF 相机服务的详细合规性信息。该实现提供了 ONVIF Device/Media/Imaging 服务、WS-Discovery 支持和 WS-Security 身份验证，用于 NVR 集成。

## ONVIF Profile S 合规性

| 组件 | 状态 | 备注 |
|-----------|--------|-------|
| **ONVIF Device 服务** | ✅ 完整 | WS-Discovery、GetDeviceInformation、GetCapabilities |
| **ONVIF Media 服务** | ✅ 完整 | GetProfiles、GetStreamUri、VideoSource 支持 |
| **ONVIF PTZ 服务** | ❌ 未实现 | PTZ 已作为死代码移除（未连接到相机） |
| **ONVIF Imaging 服务** | ✅ 完整 | 亮度、对比度、饱和度、锐度、曝光、白平衡 |
| **WS-Discovery** | ✅ 完整 | UDP 组播 + HTTP POST 探测支持 |
| **WS-Security** | ✅ 完整 | UsernameToken Text 和 Digest 身份验证 |

## 支持的服务

### Device 服务

| 操作 | 已实现 | 备注 |
|-----------|-------------|-------|
| `GetSystemDateAndTime` | ✅ | 返回 UTC 时间，手动模式（无 NTP 同步） |
| `GetDeviceInformation` | ✅ | 制造商、型号、固件、序列号、硬件 ID 来自配置 |
| `GetCapabilities` | ✅ | 广告 Media、Device、Imaging 服务 |
| `GetServices` | ✅ | 列出所有 ONVIF 服务及 XAddr 端点 |
| `GetScopes` | ✅ | 名称、硬件和类型范围 |

**设备信息响应：**
```yaml
Manufacturer: "Raspberry Pi"
Model: "Camera V1" 
Firmware: "mibee-eye-v1.0.0"
SerialNumber: "OV5647-SERIAL"
HardwareId: "OV5647"
```

**功能响应：**
```yaml
Device: XAddr: "http://<相机IP>:8080/onvif/device_service"
Media: XAddr: "http://<相机IP>:8080/onvif/media_service"  

Imaging: XAddr: "http://<相机IP>:8080/onvif/device_service"
```

### Media 服务

| 操作 | 已实现 | 备注 |
|-----------|-------------|-------|
| `GetProfiles` | ✅ | 单个配置文件，包含 H.264 编码 |
| `GetStreamUri` | ✅ | 返回 RTSP URL（rtsp://host:port/stream） |
| `GetVideoSources` | ✅ | 单个视频源（Pi Camera） |

**配置文件配置：**
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

**流 URI 响应：**
```yaml
MediaUri:
  Uri: "rtsp://<相机IP>:8554/stream"
  InvalidAfterConnect: "false"
  InvalidAfterReboot: "false"
  Timeout: "PT0S"
```

### Imaging 服务

| 操作 | 已实现 | 备注 |
|-----------|-------------|-------|
| `GetImagingSettings` | ✅ | 当前相机参数 |
| `SetImagingSettings` | ✅ | 亮度、对比度、饱和度、锐度、曝光 |
| `GetOptions` | ✅ | 参数范围和可用模式 |

**支持的成像参数：**
- **亮度**：-1.0 到 1.0（实时调整）
- **对比度**：0.0 到 32.0（实时调整）
- **饱和度**：0.0 到 32.0（实时调整）
- **锐度**：0.0 到 16.0（实时调整）
- **曝光**：AUTO/MANUAL 模式，0.0 到 10000 μs 曝光时间
- **白平衡**：AUTO/MANUAL 模式，0.0 到 1.0 Cr/Cb 增益

**成像设置响应：**
```yaml
Settings:
  Brightness: { Value: 0.0 }
  Contrast: { Value: 1.0 }
  Saturation: { Value: 1.0 }
  Sharpness: { Value: 0.0 }
  Exposure: { Mode: "AUTO", Time: 16667 }
  WhiteBalance: { Mode: "AUTO" }
```

PTZ 服务已作为死代码移除（状态机跟踪位置但从未应用到相机 - 无 ScalerCrop 连接）。OV5647 硬件没有 PTZ 电机。
## WS-Discovery 支持
服务支持两种 WS-Discovery 探测方法：
### UDP 组播 (239.255.255.250:3702)
- 在组播地址上侦听 Probe 消息
- 发送包含设备元数据的 ProbeMatches 响应
- 自动检测本地 IP 用于 XAddr 生成
- 范围包括 /name/ 和 /hardware/ 用于 NVR 匹配
### HTTP POST 探测 (/onvif/device_service)
- 通过 HTTP POST 到设备服务端点处理 Probe 消息
- 支持通过防火墙/代理进行发现
- 与 UDP 组播相同的 XML 响应
- ProbeMatches XML 作为原始字符串字节构建（不是 encoding/xml）以保持精确的元素本地名称
- NVR 依赖精确的 ProbeMatch 结构进行设备匹配
**ProbeMatches 响应：**
```xml
<ProbeMatch>
  <EndpointReference>
    <Address>uuid:device-uuid-here</Address>
  </EndpointReference>
  <Scopes>onvif://www.onvif.org/name/Pi Camera V1 onvif://www.onvif.org/hardware/OV5647</Scopes>
  <XAddrs>http://<相机IP>:8080/onvif/device_service</XAddrs>
  <Types>tdn:NetworkVideoTransmitter tdn:Device</Types>
  <MetadataVersion>1</MetadataVersion>
</ProbeMatch>
```
  ## WS-Security 支持
  UsernameToken 身份验证实现了两种密码类型：
  ### PasswordText 模式
- 直接密码比较
- 在没有 Nonce 时使用
- 简单但安全性较低
### PasswordDigest 模式
- SHA1 摘要：`base64(SHA1(base64(Nonce) + Created + Password))`
- 更安全，需要 Nonce 和 Created 时间戳
- 推荐生产环境使用
**身份验证流程：**
1. 解析 SOAP Security 头部的 UsernameToken
2. 如果存在 Nonce，计算摘要并比较
3. 如果没有 Nonce，使用直接密码比较
4. 返回包含用户名和成功状态的 AuthResult

## 已知限制

### 功能限制
- **单一配置文件**：只支持一个媒体配置文件（主配置文件）
- **无音频**：音频流未实现
- **无事件**：事件服务不支持
- **无分析**：视频分析不可用
- **无扩展**：ONVIF 扩展（如 Analytics、Search）未实现

### 硬件约束  
- **OV5647 相机**：固定对焦，无自动对焦功能
- **无红外截止滤镜**：固定红外滤镜（不支持 NoIR 变体）
- **无 WDR**：OV5647 不支持宽动态范围


### 协议限制
- **手动系统时间**：无 NTP 同步，UTC 时间在启动时固定
- **单一视频源**：只支持一个 CSI 相机
- **无 HTTPS**：仅 HTTP（无 SSL/TLS 加密）
- **基本身份验证**：无高级身份验证方法（如证书）

## 已测试的 ONVIF 客户端

### MiBee NVR
- **协议**：ONVIF 客户端（0x524a/onvif-go 库）
- **发现**：WS-Discovery + HTTP 探测
- **身份验证**：UsernameToken Digest
- **操作**：GetDeviceInformation、GetCapabilities、GetProfiles、GetStreamUri
- **集成状态**：✅ 完全兼容

### 兼容性说明
- 使用与服务器相同的 `0x524a/onvif-go` 库，确保协议兼容性
- 支持 UDP 和 HTTP 发现方法
- 符合 Profile S 合规性要求
- RTSP DESCRIBE 用于流验证
- 支持 SOAP 回退处理边缘情况

## 服务端点

| 服务 | 端点 | 协议 | 描述 |
|---------|----------|----------|-------------|
| Device 服务 | `/onvif/device_service` | HTTP/SOAP | 设备管理 |
| Media 服务 | `/onvif/media_service` | HTTP/SOAP | 媒体配置文件/URI |
| 快照 | `/snapshot` | HTTP | JPEG 快照 |
| RTSP 流 | `/stream` | RTSP | H.264 视频流 |

## 错误处理

服务遵循 ONVIF SOAP 1.2 错误约定：

- **soap:Sender**：客户端请求错误（无效操作、身份验证失败）
- **soap:Receiver**：服务器处理错误（相机访问、参数无效）
- **SOAP 错误代码**：带有描述性文本的标准错误代码

**错误响应示例：**
```xml
<SOAP-ENV:Fault>
  <faultcode>soap:Sender</faultcode>
  <faulttext>authentication failed: password mismatch</faulttext>
</SOAP-ENV:Fault>
```