# MiBee Eye Troubleshooting Guide

[中文](zh/troubleshooting.md)

This guide covers common issues and solutions for MiBee Eye, the single-board computer ONVIF camera service.

## Quick Health Check

```bash
# Check if mibee-eye is running
systemctl status mibee-eye

# Check camera device
ls -la /dev/video0

# Check network connectivity
netstat -tlnp | grep -E '8554|8080|3702|8088'

# Check memory usage
free -h

# Check CPU usage
top -bn1 | head -20

# Check web UI
curl -s -o /dev/null -w "%{http_code}" http://localhost:8088/
# Expected: 200 or 302 (login redirect)

# Check HLS stream
curl -s http://localhost:8088/hls/stream.m3u8 | head -5
```

# Check camera encoder is working (look for x264 in logs)
journalctl -u mibee-eye --since "1 minute ago" | grep -i "x264\|encoder\|h264"

## Camera Detection Issues

### Symptoms
- Camera not found in NVR discovery
- mibee-eye logs show "camera not detected"
- Stream shows black screen

### Diagnosis
```bash
# Check if camera device exists
ls -la /dev/video0
# Should show /dev/video0 character device

# Test camera with libcamera directly
rpicam-hello

# Check device tree overlay
cat /boot/firmware/config.txt | grep dtoverlay
# Should show: dtoverlay=ov5647

# Check kernel camera support (ignore if device works via libcamera)
vcgencmd get_camera
```

### Solutions
1. **MediaMTX conflicts**: Stop MediaMTX first
   ```bash
   sudo systemctl stop mediamtx
   sudo systemctl disable mediamtx
   ```

2. **Missing DT overlay**: Add to config.txt
   ```bash
   sudo nano /boot/firmware/config.txt
   # Add: dtoverlay=ov5647
   sudo reboot
   ```

3. **Camera module not connected**: Check CSI cable

4. **Wrong device path**: Update config.yaml
   ```yaml
   camera:
     device: "/dev/video0"  # or your camera device
   ```

## Web UI Login Issues

### Symptoms
- Login page shows but credentials don't work
- "Invalid credentials" error on login
- Cannot access web admin panel

### Diagnosis
```bash
# Check web UI is running
curl -s -o /dev/null -w "%{http_code}" http://localhost:8088/

# Check auth endpoint
curl -s -X POST http://localhost:8088/api/login \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"test"}'

# Check credentials in config
grep -A2 'web:' config.yaml
```

### Solutions
1. **Default credentials**: Web UI defaults to ONVIF credentials when web.username/web.password are empty. Set explicit web credentials:
   ```yaml
   web:
     username: "admin"
     password: "your-password"
   ```
2. **Clear browser cache**: Token-based auth stores session in localStorage. Clear browser data for the site.
3. **Check config**: Ensure web.enabled: true in config.yaml
sudo systemctl restart mibee-eye
## Camera Encoder Issues

### Symptoms
- Service starts but logs show "encoder_create(): unable to activate output stream"
- RTSP stream connects but no video data
- Snapshot endpoint returns 503 Service Unavailable

### Diagnosis
```bash
# Check if mtxrpicam can find libcamera
LD_LIBRARY_PATH=/home/pi/mibee-eye/deploy/bin ldd ~/mibee-eye/deploy/bin/mtxrpicam
# If "libcamera.so.9.9 => not found", bundled libs are missing

# Check if bundled libcamera files exist
ls -la ~/mibee-eye/deploy/bin/libcamera*.so*

# Check LD_LIBRARY_PATH in systemd service
grep LD_LIBRARY_PATH /etc/systemd/system/mibee-eye.service

# Test camera directly with rpicam-vid
rpicam-vid -t 1000 --width 1280 --height 720 -o /dev/null

# Check system libcamera version (may differ from bundled version)
dpkg -l | grep libcamera
```

### Root Cause
The `mtxrpicam` binary is dynamically linked against `libcamera.so.9.9`, which is NOT the same as the system-installed libcamera (Debian 13 provides `libcamera.so.0.7`). The bundled version must be present in `deploy/bin/` and `LD_LIBRARY_PATH` must point there.

### Solutions
1. **Missing bundled libs**: Re-deploy from mediamtx-rpicamera release
   ```bash
   # Download and extract on workstation
   gh release download v2.6.0 --repo bluenviron/mediamtx-rpicamera \
     --pattern "mtxrpicam_64.tar.gz"
   tar xzf mtxrpicam_64.tar.gz
   
   # Copy bundled libs to device
   scp mtxrpicam_64/libcamera*.so* <your-rpi-user>@<your-rpi-ip>:~/mibee-eye/deploy/bin/
   scp mtxrpicam_64/mtxrpicam <your-rpi-user>@<your-rpi-ip>:~/mibee-eye/deploy/bin/
   
   # Restart service
   sudo systemctl restart mibee-eye
   ```

2. **LD_LIBRARY_PATH not set**: Verify systemd service configuration
   ```bash
   # Should contain: Environment=LD_LIBRARY_PATH=/path/to/deploy/bin
   systemctl cat mibee-eye
   
   # If missing, edit the service file
   sudo systemctl edit mibee-eye --force
   # Add: Environment=LD_LIBRARY_PATH=/home/pi/mibee-eye/deploy/bin
   sudo systemctl daemon-reload
   sudo systemctl restart mibee-eye
   ```

3. **Camera held by another process**: Stop MediaMTX
   ```bash
   sudo systemctl stop mediamtx
   sudo systemctl disable mediamtx
   # Verify camera is free
   lsof /dev/video0
   ```
## RTSP Streaming Issues

### Symptoms
- RTSP stream not accessible
- Connection timeouts
- NVR can't connect to stream

### Diagnosis
```bash
# Check RTSP port is listening
netstat -tlnp | grep 8554

# Test RTSP connection locally
ffplay rtsp://localhost:8554/stream

# Check firewall rules
sudo ufw status

# Check camera exclusivity
lsof /dev/video0
```

### Solutions
1. **Port conflict**: Change RTSP port in config.yaml
   ```yaml
   rtsp:
     port: 8555  # Change if needed
   ```

2. **Firewall block**: Allow RTSP port
   ```bash
   sudo ufw allow 8554/tcp
   ```

3. **Camera access conflict**: Ensure only one process uses /dev/video0
   ```bash
   sudo systemctl stop mediamtx  # if running
   ```

4. **Network issues**: Check from client system
   ```bash
   telnet <camera-ip> 8554
   ```

## ONVIF Discovery Issues

### Symptoms
- NVR can't discover camera
- WS-Discovery probe fails
- ONVIF device service not found

### Diagnosis
```bash
# Check ONVIF HTTP port
netstat -tlnp | grep 8080

# Test UDP multicast port
nc -ul 3702

# Check ONVIF service logs
journalctl -u mibee-eye -f

# Test ONVIF endpoint manually
curl -X POST http://localhost:8080/onvif/device_service
```

### Solutions
1. **Network issues**: Check multicast routing
   ```bash
   # Enable multicast if needed
   sudo sysctl -w net.ipv4.conf.all.mc_forwarding=1
   ```

2. **Port conflict**: Change ONVIF port
   ```yaml
   onvif:
     port: 8081  # Change if needed
   ```

3. **Firewall blocks**: Allow ONVIF ports
   ```bash
   sudo ufw allow 8080/tcp
   sudo ufw allow 3702/udp
   ```

4. **Discovery timeout**: Increase in NVR config if needed

## NVR Integration Issues

### Symptoms
- NVR shows camera but can't add
- GetStreamUri fails
- Authentication issues

### Diagnosis
```bash
# Check ONVIF credentials in config.yaml
# Test ONVIF client manually
curl -X POST -H "Content-Type: application/soap+xml" \
  -d "<soap:Envelope>...</soap:Envelope>" \
  http://localhost:8080/onvif/device_service

# Check RTSP URL format
echo "rtsp://localhost:8554/stream"

# Check device info response
curl -s http://localhost:8080/onvif/device_service | grep -i device
```

### Solutions
1. **Authentication**: Set ONVIF credentials
   ```yaml
   onvif:
     username: "admin"
     password: "your-password"
   ```

2. **Invalid RTSP URL**: Ensure URL matches config
   ```yaml
   rtsp:
     username: ""  # Leave empty if no auth
     password: ""
   ```

3. **Profile issues**: Check video encoder config
   ```yaml
   camera:
     width: 1280
     height: 720
     codec: "h264"
   ```

4. **Device info**: Update device metadata
   ```yaml
   device:
     name: "My Camera"
     manufacturer: "Raspberry Pi"
     model: "OV5647"
   ```

## Snapshot Issues

### Symptoms
- Snapshot endpoint returns error
- NVR can't capture images

### Diagnosis
```bash
# Check snapshot endpoint
curl -I http://localhost:8080/snapshot

# Test snapshot endpoint and check response
curl -s -w "\nHTTP Status: %{http_code}\n" http://localhost:8080/snapshot -o /dev/null
# HTTP 200 + "image/jpeg" = working
# HTTP 503 = camera not providing frames (check encoder)
```

### Solutions
1. **Camera not running**: Ensure mibee-eye is active
   ```bash
   sudo systemctl restart mibee-eye
   ```

2. **Resolution issues**: Adjust camera resolution in config.yaml
   ```yaml
   camera:
     width: 1280
     height: 720
   ```

3. **Encoder not producing frames**: Check logs and verify mtxrpicam is working
   ```bash
   journalctl -u mibee-eye --since "5 minutes ago" | grep -i "encoder\|h264"
   ```
## HLS Live Preview Issues

### Symptoms
- Web UI shows black video player
- "HLS not available" message in web UI
- Browser console shows hls.js errors

### Diagnosis
```bash
# Check HLS HTTP endpoint
curl -s http://localhost:8088/hls/stream.m3u8

# Verify HLS is enabled in config
grep -A2 'hls:' config.yaml

# Check mibee-eye logs for HLS errors
journalctl -u mibee-eye --grep "HLS"

# Check RTSP stream is feeding HLS (must be running)
journalctl -u mibee-eye --grep "AUHub"
```

### Solutions
1. **HLS not enabled**: Enable HLS in config.yaml
   ```yaml
   hls:
     enabled: true
     segment_duration: 2s
   ```

   2. **RTSP source unavailable**: Ensure RTSP stream is working first:
   ```bash
   ffprobe rtsp://localhost:8554/stream
   ```

3. **Browser compatibility**: HLS uses pure Go MPEG-TS segmenter served via HTTP. Ensure browser supports MSE or use hls.js library.

   4. **Restart mibee-eye**: sudo systemctl restart mibee-eye

5. **Debug logging**: Set `logging.level: debug` to see segmenter errors


## Port 9100 Conflict

### Symptoms

- Metrics endpoint fails to start
- "Address already in use" error in logs
- Service starts but metrics not accessible

### Diagnosis

```bash
# Check if port 9100 is in use
netstat -tlnp | grep 9100

# Check for prometheus-node-exporter
ps aux | grep node_exporter
```

### Root Cause

Port 9100 is used by Prometheus node_exporter by default, conflicting with mibee-eye metrics endpoint.

### Solutions

1. **Disable metrics**: Set `metrics.enabled: false` in config.yaml

2. **Change metrics port**: Use a different port:
   ```yaml
   metrics:
     enabled: true
     port: 9101  # or any available port
   ```


## WiFi Stability Issues

### Symptoms

- RTSP/HLS streams drop intermittently
- Client disconnects randomly
- Network throughput drops under load

### Root Cause

RPi 3B WiFi drops under sustained transfer load due to hardware limitations (270Mbps theoretical).

### Solutions

1. **Use Ethernet if possible**: Wired connection is more stable

2. **Lower bitrate/fps**: Reduce camera settings for WiFi:
   ```yaml
   camera:
     fps: 10
     bitrate: 1000000  # 1 Mbps
   ```

3. **Restart service to recover**: If WiFi drops:
   ```bash
   sudo systemctl restart mibee-eye
   ```

### Symptoms
- High memory usage
- Lag in video stream
- CPU overload

### Diagnosis
```bash
# Check memory usage
free -h

# Check CPU usage
top -bn1 | grep -E 'mibee-eye|mediamtx'

# Check RTSP connections
lsof -i :8554

# Check network bandwidth
ip -s link show wlan0
```

### Solutions
1. **Reduce resolution**: Lower capture settings
   ```yaml
   camera:
     width: 640
     height: 480
     fps: 10
     bitrate: 1000000  # 1 Mbps
   ```

2. **Reduce RTSP buffer sizes** (advanced): Lower subscriber_buffer_size in config.yaml
3. **Monitor resources**: Add monitoring
   ```bash
   # Monitor memory every 5 seconds
   watch -n 5 "free -h && ps aux | grep mibee-eye"
   ```

## Out of Memory (OOM) Issues

### Symptoms
- mibee-eye process killed unexpectedly
- "Killed" in journalctl logs
- dmesg shows "Out of memory" or "oom-killer"
- System becomes unresponsive

### Diagnosis
```bash
# Check for OOM kills
dmesg | grep -i "oom\|killed"

# Check memory usage history
free -h

# Check for memory-hungry processes
ps aux --sort=-%mem | head -10
```

### Root Cause
The single-board computer has limited RAM. If another process consumes excessive memory (e.g. prometheus-node-exporter-collectors' apt_info.py using 124MB), the OOM killer will terminate the largest process.
### Solutions
1. **Check for cron/periodic jobs**: Disable unnecessary timers:
   ```bash
   systemctl list-timers | grep -E "apt|collect"
   sudo systemctl disable --now prometheus-node-exporter-apt.timer
   ```
2. **Reduce camera bitrate**: Lower in config.yaml
3. **Monitor with less memory-intensive tools**: Remove prometheus-node-exporter-collectors if not needed
4. **Add swap** (last resort): 512MB swap file provides OOM cushion

## Debug Mode and Logging

### Enable Debug Logging
```bash
# Set debug level via environment
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye

# Or in config.yaml
logging:
  level: "debug"
```

### Verbose Mode Options
```bash
# Enable ONVIF debug logging
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye -onvif-debug

# Enable RTSP debug logging
MIBEE_EYE_LOGGING_LEVEL=debug ./mibee-eye -rtsp-debug
```

### Log Analysis Tips
```bash
# Follow logs in real-time
journalctl -u mibee-eye -f

# Filter error messages
journalctl -u mibee-eye | grep ERROR

# Look for timeout patterns
journalctl -u mibee-eye | grep -i timeout

# Check for resource warnings
journalctl -u mibee-eye | grep -i "memory\|cpu"
```

## Common Error Messages

### Camera Access Issues
```
ERROR: camera device not available
```
- Solution: Stop MediaMTX, check device path

### Port Already in Use
```
ERROR: address already in use
```
- Solution: Change port in config or stop conflicting service

### Network Issues
```
ERROR: connection refused
```
- Solution: Check firewall, network connectivity

### Memory Issues
```
WARNING: high memory usage detected
```
- Solution: Reduce resolution, FPS, or bitrate

### Encoder Create Error
```
camera: mtxrpicam error: encoder_create(): unable to activate output stream
```
- Cause: Bundled libcamera shared libraries missing or LD_LIBRARY_PATH not set
- Solution: See "Camera Encoder Issues" section

### Shared Library Not Found
```
error while loading shared libraries: libcamera.so.9.9: cannot open shared object file
```
- Cause: LD_LIBRARY_PATH does not include the deploy/bin/ directory
- Solution: Set `Environment=LD_LIBRARY_PATH=<deploy-path>/bin` in systemd service
### HLS Error
```
WARNING: HLS bridge not started
```
- Solution: Ensure `hls.enabled: true` and RTSP server is running to feed AUHub subscribers
## System Status Commands

```bash
# Complete system health check
echo "=== System Status ==="
echo "Camera Device:"
ls -la /dev/video0 2>/dev/null || echo "No camera found"

echo "Services:"
systemctl is-active mibee-eye mediamtx

echo "Network:"
netstat -tlnp | grep -E '8554|8080|3702'

echo "Web UI:"
curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8088/
echo "HLS Status:"
curl -s http://localhost:8088/hls/stream.m3u8 > /dev/null 2>&1 && echo "HLS active" || echo "HLS inactive"

echo "Memory:"
free -h

echo "Camera Process:"
pgrep -f mibee-eye || echo "mibee-eye not running"

echo "Conflicting Processes:"
lsof /dev/video0 2>/dev/null | grep -v mibee-eye || echo "No conflicts"
```

echo "Encoder Status:"
journalctl -u mibee-eye --since "5 minutes ago" | grep -i "x264\|encoder" | tail -3

echo "Library Resolution:"
LD_LIBRARY_PATH=~/mibee-eye/deploy/bin ldd ~/mibee-eye/deploy/bin/mtxrpicam 2>&1 | grep -E "found|libcamera"

## Contact Support

If issues persist:
1. Check logs: `journalctl -u mibee-eye`
2. Include system info: `uname -a`, `dpkg -l | grep mibee-eye`
3. Provide exact error messages and reproduction steps
4. Include configuration file (redact sensitive data)