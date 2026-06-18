# MiBee Eye 故障排除指南

[English](../troubleshooting.md)

本文档涵盖了 mibee-eye（树莓 Pi ONVIF 相机服务）的常见问题和解决方案。

## 快速健康检查

```bash
# 检查 mibee-eye 是否运行
systemctl status mibee-eye

# 检查相机设备
ls -la /dev/video0

# 检查网络连接
# 检查网络连接
netstat -tlnp | grep -E '8554|8080|3702|8088'

# 检查内存使用
free -h

# 检查 CPU 使用
top -bn1 | head -20
```

# 检查 Web UI
curl -s -o /dev/null -w "%{http_code}" http://localhost:8088/
# 期望值：200 或 302（登录重定向）

# 检查 HLS 流
curl -s http://localhost:8088/hls/stream.m3u8 | head -5

# 检查摄像头编码器是否正常工作（日志中应有 x264）
journalctl -u mibee-eye --since "1 minute ago" | grep -i "x264\|encoder\|h264"
## 相机检测问题

### 症状
- NVR 中找不到相机
- MiBee Eye 日志显示 "camera not detected"
- 流显示黑屏

### 诊断
```bash
# 检查相机设备是否存在
ls -la /dev/video0
# 应显示 /dev/video0 字符设备

# 直接使用 libcamera 测试相机
rpicam-hello

# 检查设备树覆盖层
cat /boot/firmware/config.txt | grep dtoverlay
# 应显示：dtoverlay=ov5647

# 检查内核相机支持（如果设备通过 libcamera 工作，可忽略）
vcgencmd get_camera
```

### 解决方案
1. **MediaMTX 冲突**：首先停止 MediaMTX
   ```bash
   sudo systemctl stop mediamtx
   sudo systemctl disable mediamtx
   ```

2. **缺少 DT 覆盖层**：添加到 config.txt
   ```bash
   sudo nano /boot/firmware/config.txt
   # 添加：dtoverlay=ov5647
   sudo reboot
   ```

3. **相机模块未连接**：检查 CSI 电缆

4. **设备路径错误**：更新 config.yaml
   ```yaml
   camera:
     device: "/dev/video0"  # 或你的相机设备
   ```

## Web UI 登录问题

### 症状
- 显示登录页面但凭据无效
- 登录时显示 "Invalid credentials" 错误
- 无法访问 Web 管理面板

### 诊断
```bash
# 检查 Web UI 是否运行中
curl -s -o /dev/null -w "%{http_code}" http://localhost:8088/

# 检查身份验证端点
curl -s -X POST http://localhost:8088/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"test"}'

# 检查配置文件中的凭据
grep -A2 'web:' config.yaml
```

### 解决方案
1. **默认凭据**：当 web.username/web.password 为空时，Web UI 默认使用 ONVIF 凭据。设置显式的 Web 凭据：
   ```yaml
   web:
     username: "admin"
     password: "your-password"
   ```
2. **清除浏览器缓存**：基于令牌的身份验证在 localStorage 中存储会话。清除站点的浏览器数据。
3. **检查配置**：确保 config.yaml 中 web.enabled: true
4. **重启服务**：sudo systemctl restart mibee-eye
## 摄像头编码器问题

### 症状
- 服务启动但日志显示 "encoder_create(): unable to activate output stream"
- RTSP 流连接但无视频数据
- 快照端点返回 503 Service Unavailable

### 诊断
```bash
# 检查 mtxrpicam 是否能找到 libcamera
LD_LIBRARY_PATH=/home/pi/mibee-eye/deploy/bin ldd ~/mibee-eye/deploy/bin/mtxrpicam
# 如果显示 "libcamera.so.9.9 => not found"，说明捆绑库缺失

# 检查捆绑的 libcamera 文件是否存在
ls -la ~/mibee-eye/deploy/bin/libcamera*.so*

# 检查 systemd 服务中的 LD_LIBRARY_PATH
grep LD_LIBRARY_PATH /etc/systemd/system/mibee-eye.service

# 直接使用 rpicam-vid 测试摄像头
rpicam-vid -t 1000 --width 1280 --height 720 -o /dev/null

# 检查系统 libcamera 版本（可能与捆绑版本不同）
dpkg -l | grep libcamera
```

### 根因
mtxrpicam 二进制文件动态链接的是 `libcamera.so.9.9`，这与系统安装的 libcamera（Debian 13 提供的是 `libcamera.so.0.7`）不同。捆绑版本必须存在于 `deploy/bin/` 目录中，且 `LD_LIBRARY_PATH` 必须指向该目录。

### 解决方案
1. **缺少捆绑库**：从 mediamtx-rpicamera 发布版重新部署
   ```bash
   # 在工作站上下载并解压
   gh release download v2.6.0 --repo bluenviron/mediamtx-rpicamera \
     --pattern "mtxrpicam_64.tar.gz"
   tar xzf mtxrpicam_64.tar.gz
   
   # 复制捆绑库到设备
   scp mtxrpicam_64/libcamera*.so* <your-rpi-user>@<your-rpi-ip>:~/mibee-eye/deploy/bin/
   scp mtxrpicam_64/mtxrpicam <your-rpi-user>@<your-rpi-ip>:~/mibee-eye/deploy/bin/
   
   # 重启服务
   sudo systemctl restart mibee-eye
   ```

2. **LD_LIBRARY_PATH 未设置**：验证 systemd 服务配置
   ```bash
   # 应包含：Environment=LD_LIBRARY_PATH=/path/to/deploy/bin
   systemctl cat mibee-eye
   
   # 如果缺失，编辑服务文件
   sudo systemctl edit mibee-eye --force
   # 添加：Environment=LD_LIBRARY_PATH=/home/pi/mibee-eye/deploy/bin
   sudo systemctl daemon-reload
   sudo systemctl restart mibee-eye
   ```

3. **摄像头被其他进程占用**：停止 MediaMTX
   ```bash
   sudo systemctl stop mediamtx
   sudo systemctl disable mediamtx
   # 验证摄像头已释放
   lsof /dev/video0
   ```
## RTSP 流媒体问题

### 症状
- RTSP 流无法访问
- 连接超时
- NVR 无法连接到流

### 诊断
```bash
# 检查 RTSP 端口是否在监听
netstat -tlnp | grep 8554

# 本地测试 RTSP 连接
ffplay rtsp://localhost:8554/stream

# 检查防火墙规则
sudo ufw status

# 检查相机独占性
lsof /dev/video0
```

### 解决方案
1. **端口冲突**：在 config.yaml 中更改 RTSP 端口
   ```yaml
   rtsp:
     port: 8555  # 如需要则更改
   ```

2. **防火墙阻塞**：允许 RTSP 端口
   ```bash
   sudo ufw allow 8554/tcp
   ```

3. **相机访问冲突**：确保只有一个进程使用 /dev/video0
   ```bash
   sudo systemctl stop mediamtx  # 如果正在运行
   ```

4. **网络问题**：从客户端系统检查
   ```bash
   telnet <camera-ip> 8554
   ```

## ONVIF 发现问题

### 症状
- NVR 无法发现相机
- WS-Discovery 探测失败
- 找不到 ONVIF 设备服务

### 诊断
```bash
# 检查 ONVIF HTTP 端口
netstat -tlnp | grep 8080

# 测试 UDP 多播端口
nc -ul 3702

# 检查 ONVIF 服务日志
journalctl -u mibee-eye -f

# 手动测试 ONVIF 端点
curl -X POST http://localhost:8080/onvif/device_service
```

### 解决方案
1. **网络问题**：检查多播路由
   ```bash
   # 如果需要，启用多播
   sudo sysctl -w net.ipv4.conf.all.mc_forwarding=1
   ```

2. **端口冲突**：更改 ONVIF 端口
   ```yaml
   onvif:
     port: 8081  # 如需要则更改
   ```

3. **防火墙阻塞**：允许 ONVIF 端口
   ```bash
   sudo ufw allow 8080/tcp
   sudo ufw allow 3702/udp
   ```

4. **发现超时**：如需要，在 NVR 配置中增加超时时间

## NVR 集成问题

### 症状
- NVR 显示相机但无法添加
- GetStreamUri 失败
- 身份验证问题

### 诊断
```bash
# 检查 config.yaml 中的 ONVIF 凭据
# 手动测试 ONVIF 客户端
curl -X POST -H "Content-Type: application/soap+xml" \
  -d "<soap:Envelope>...</soap:Envelope>" \
  http://localhost:8080/onvif/device_service

# 检查 RTSP URL 格式
echo "rtsp://localhost:8554/stream"

# 检查设备信息响应
curl -s http://localhost:8080/onvif/device_service | grep -i device
```

### 解决方案
1. **身份验证**：设置 ONVIF 凭据
   ```yaml
   onvif:
     username: "admin"
     password: "your-password"
   ```

2. **无效的 RTSP URL**：确保 URL 与配置匹配
   ```yaml
   rtsp:
     username: ""  # 如果没有身份验证则留空
     password: ""
   ```

3. **配置文件问题**：检查视频编码器配置
   ```yaml
   camera:
     width: 1280
     height: 720
     codec: "h264"
   ```

4. **设备信息**：更新设备元数据
   ```yaml
   device:
     name: "我的相机"
     manufacturer: "树莓 Pi"
     model: "OV5647"
   ```

## 快照问题

### 症状
- 快照端点返回错误
- NVR 无法捕获图像

### 诊断
```bash
# 检查快照端点
curl -I http://localhost:8080/snapshot

# 测试快照端点并检查响应
curl -s -w "\nHTTP 状态: %{http_code}\n" http://localhost:8080/snapshot -o /dev/null
# HTTP 200 + "image/jpeg" = 正常工作
# HTTP 503 = 摄像头未提供帧（检查编码器）
```

### 解决方案
1. **相机未运行**：确保 MiBee Eye 处于活动状态
   ```bash
   sudo systemctl restart mibee-eye
   ```

2. **分辨率问题**：调整相机分辨率
   ```yaml
   camera:
     width: 1280
     height: 720
   ```

3. **编码器未产生帧**：检查日志并验证 mtxrpicam 正常工作
   ```bash
   journalctl -u mibee-eye --since "5 minutes ago" | grep -i "encoder\|h264"
   ```

## HLS 实时预览问题

### 症状
- Web UI 显示黑色视频播放器
- Web UI 中显示 "HLS not available" 消息
- 浏览器控制台显示 hls.js 错误

### 诊断
```bash
# 检查 HLS HTTP 端点
curl -s http://localhost:8088/hls/stream.m3u8

# 验证配置中启用了 HLS
grep -A2 'hls:' config.yaml

# 检查 MiBee Eye 日志中的 HLS 错误
journalctl -u mibee-eye --grep "HLS"

# 检查 RTSP 流是否在为 HLS 提供数据（必须运行）
journalctl -u mibee-eye --grep "AUHub"
```

### 解决方案
1. **HLS 未启用**：在 config.yaml 中启用 HLS
   ```yaml
   hls:
     enabled: true
     segment_duration: 2s
   ```

   2. **RTSP 源不可用**：首先确保 RTSP 流正常工作：
   ```bash
   ffprobe rtsp://localhost:8554/stream
   ```

3. **浏览器兼容性**：HLS 使用纯 Go MPEG-TS 分段器通过 HTTP 提供。确保浏览器支持 MSE 或使用 hls.js 库。

   4. **重启 MiBee Eye**：sudo systemctl restart mibee-eye

5. **调试日志**：设置 `logging.level: debug` 以查看分段器错误


## 端口 9100 冲突

### 症状

- 指标端点无法启动
- 日志中出现 "Address already in use" 错误
- 服务启动但指标不可访问

### 诊断

```bash
# 检查端口 9100 是否被使用
netstat -tlnp | grep 9100

# 检查 prometheus-node-exporter
ps aux | grep node_exporter
```

### 根因

Prometheus node_exporter 默认使用端口 9100，与 mibee-eye 指标端点冲突。

### 解决方案

1. **禁用指标**：在 config.yaml 中设置 `metrics.enabled: false`

2. **更改指标端口**：使用不同的端口：
   ```yaml
   metrics:
     enabled: true
     port: 9101  # 或任何可用端口
   ```


## WiFi 稳定性问题

### 症状

- RTSP/HLS 流间歇性中断
- 客户端随机断开
- 负载下网络吞吐量下降

### 根因

RPi 3B WiFi 由于硬件限制（理论 270Mbps），在持续传输负载下会掉线。

### 解决方案

1. **尽可能使用以太网**：有线连接更稳定

2. **降低比特率/帧率**：针对 WiFi 降低相机设置：
   ```yaml
   camera:
     fps: 10
     bitrate: 1000000  # 1 Mbps
   ```

3. **重启服务以恢复**：如果 WiFi 掉线：
   ```bash
   sudo systemctl restart mibee-eye
   ```

### 症状
- 高内存使用
- 视频流延迟
- CPU 过载

### 诊断
```bash
# 检查内存使用
free -h

# 检查 CPU 使用
top -bn1 | grep -E 'mibee-eye|mediamtx'

# 检查 RTSP 连接
lsof -i :8554

# 检查网络带宽
ip -s link show wlan0
```

### 解决方案
1. **降低分辨率**：降低捕获设置
   ```yaml
   camera:
     width: 640
     height: 480
     fps: 10
     bitrate: 1000000  # 1 Mbps
   ```

2. **降低 RTSP 缓冲区大小**（高级）：在 config.yaml 中降低 subscriber_buffer_size
3. **监控资源**：添加监控
   ```bash
   # 每 5 秒监控内存
   watch -n 5 "free -h && ps aux | grep mibee-eye"
   ```

## 内存不足 (OOM) 问题

### 症状
- mibee-eye 进程被意外终止
- journalctl 日志中显示 "Killed"
- dmesg 显示 "Out of memory" 或 "oom-killer"
- 系统变得无响应

### 诊断
```bash
# 检查 OOM 终止事件
dmesg | grep -i "oom\|killed"

# 检查内存使用历史
free -h

# 检查内存消耗大的进程
ps aux --sort=-%mem | head -10
```

### 根因
树莓 Pi 3B 只有 905MB RAM。如果另一个进程消耗过多内存（例如 prometheus-node-exporter-collectors 的 apt_info.py 使用 124MB），OOM 杀手会终止最大的进程。

### 解决方案
1. **检查 cron/周期性任务**：禁用不必要的定时器：
   ```bash
   systemctl list-timers | grep -E "apt|collect"
   sudo systemctl disable --now prometheus-node-exporter-apt.timer
   ```
2. **降低摄像头比特率**：在 config.yaml 中降低设置
3. **使用内存占用少的监控工具**：如果不需要，移除 prometheus-node-exporter-collectors
4. **添加交换空间**（最后手段）：512MB 交换文件提供 OOM 缓冲

## 调试模式和日志

### 启用调试日志
```bash
# 通过环境设置调试级别
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye

# 或者在 config.yaml 中
logging:
  level: "debug"
```

### 详细模式选项
```bash
# 启用 ONVIF 调试日志
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye -onvif-debug

# 启用 RTSP 调试日志
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye -rtsp-debug
```

### 日志分析技巧
```bash
# 实时查看日志
journalctl -u mibee-eye -f

# 过滤错误消息
journalctl -u mibee-eye | grep ERROR

# 查找超时模式
journalctl -u mibee-eye | grep -i timeout

# 检查资源警告
journalctl -u mibee-eye | grep -i "memory\|cpu"
```

## 常见错误消息

### 相机访问问题
```
ERROR: camera device not available
```
- 解决方案：停止 MediaMTX，检查设备路径

### 端口已使用
```
ERROR: address already in use
```
- 解决方案：在配置中更改端口或停止冲突服务

### 网络问题
```
ERROR: connection refused
```
- 解决方案：检查防火墙，网络连接

### 内存问题
```
WARNING: high memory usage detected
```
- 解决方案：降低分辨率、FPS 或比特率

### 编码器创建错误
```
camera: mtxrpicam error: encoder_create(): unable to activate output stream
```
- 原因：捆绑的 libcamera 共享库缺失或 LD_LIBRARY_PATH 未设置
- 解决方案：参见「摄像头编码器问题」部分

### 共享库未找到
```
error while loading shared libraries: libcamera.so.9.9: cannot open shared object file
```
- 原因：LD_LIBRARY_PATH 未包含 deploy/bin/ 目录
- 解决方案：在 systemd 服务中设置 `Environment=LD_LIBRARY_PATH=<deploy-path>/bin`
### HLS 错误
```
WARNING: HLS bridge not started
```
- 解决方案：确保 `hls.enabled: true` 且 RTSP 服务器正在运行以向 AUHub 订阅者提供数据

## 系统状态命令

```bash
# 完整系统健康检查
echo "=== 系统状态 ==="
echo "相机设备："
ls -la /dev/video0 2>/dev/null || echo "未找到相机"

echo "服务："
systemctl is-active mibee-eye mediamtx

echo "网络："
netstat -tlnp | grep -E '8554|8080|3702'

echo "Web UI:"
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8088/
echo "HLS 状态:"
curl -s http://localhost:8088/hls/stream.m3u8 > /dev/null 2>&1 && echo "HLS 活动" || echo "HLS 未活动"

echo "内存："
free -h

echo "相机进程："
pgrep -f mibee-eye || echo "MiBee Eye 未运行"

echo "冲突进程："
lsof /dev/video0 2>/dev/null | grep -v mibee-eye || echo "无冲突"
```

echo "编码器状态："
journalctl -u mibee-eye --since "5 minutes ago" | grep -i "x264\|encoder" | tail -3

echo "库解析："
LD_LIBRARY_PATH=~/mibee-eye/deploy/bin ldd ~/mibee-eye/deploy/bin/mtxrpicam 2>&1 | grep -E "found|libcamera"

## 联系支持

如果问题持续存在：
1. 检查日志：`journalctl -u mibee-eye`
2. 包含系统信息：`uname -a`，`dpkg -l | grep mibee-eye`
3. 提供确切的错误消息和重现步骤
4. 包含配置文件（删除敏感数据）