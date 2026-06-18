// Portions derived from MediaMTX (https://github.com/bluenviron/mediamtx)
// Original code Copyright (c) bluenviron, MIT License
//
// camera.go defines the Camera interface and the RPiCamera implementation
// that communicates with the mtxrpicam subprocess via the binary pipe protocol.

package camera

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"golang.org/x/sys/unix"
)

// Frame represents a single captured video frame.
type Frame struct {
	Data      []byte    // H.264 Annex-B NALU data (may contain multiple NALUs)
	Timestamp time.Time // Frame capture time (NTP-adjusted)
	PTS       int64     // Presentation timestamp in 90kHz clock
}

// CameraInfo provides metadata about the camera device.
type CameraInfo struct {
	Name         string
	Manufacturer string
	Model        string
	Width        uint32
	Height       uint32
	FPS          float32
	Codec        string
	SerialNumber string
}

// Camera is the interface for camera capture backends.
type Camera interface {
	// Start begins capturing frames from the camera device.
	Start(ctx context.Context) error

	// Stop gracefully stops capture and releases resources.
	Stop() error

	// Frames returns a read-only channel that receives captured frames.
	// The channel is closed when Stop() is called.
	Frames() <-chan Frame

	// SetParam changes a camera parameter at runtime.
	// Supported names: brightness, contrast, saturation, sharpness,
	// width, height, fps, exposure, gain, awbMode, hFlip, vFlip,
	// shutter, denoise, ev, bitrate, idrPeriod.
	SetParam(name string, value interface{}) error

	// GetParam returns the current value of a camera parameter.
	GetParam(name string) (interface{}, error)

	// Info returns camera device information.
	Info() CameraInfo
}

// RPiCamera implements Camera by spawning the mtxrpicam subprocess
// and communicating via the binary pipe protocol.
// If the subprocess dies, it is automatically restarted with exponential
// backoff (1s → 30s max).
type RPiCamera struct {
	mu     sync.RWMutex
	params Params
	info   CameraInfo

	// subprocess management
	cmd       *exec.Cmd
	confPipe  *pipe // config: Go -> mtxrpicam
	videoPipe *pipe // video: mtxrpicam -> Go

	// frame delivery (created once in Start, closed only in Stop)
	framesCh chan Frame
	stopOnce sync.Once

	// lifecycle state
	ctx     context.Context
	started bool
	stopped bool
	wg      sync.WaitGroup // tracks run() goroutine

	// shutdown coordination
	cancel context.CancelFunc

	// binary path
	binPath string

	// frame drop tracking
	droppedFrames atomic.Uint64
}

// RPiCameraOption configures RPiCamera behavior.
type RPiCameraOption func(*RPiCamera)

// WithBinPath sets the path to the mtxrpicam binary.
func WithBinPath(path string) RPiCameraOption {
	return func(c *RPiCamera) {
		c.binPath = path
	}
}

// WithParams sets initial camera parameters.
func WithParams(p Params) RPiCameraOption {
	return func(c *RPiCamera) {
		c.params = p
	}
}

// WithInfo sets camera info metadata.
func WithInfo(info CameraInfo) RPiCameraOption {
	return func(c *RPiCamera) {
		c.info = info
	}
}

// NewRPiCamera creates a new RPiCamera with the given options.
func NewRPiCamera(opts ...RPiCameraOption) *RPiCamera {
	c := &RPiCamera{
		params:  DefaultParams(),
		binPath: filepath.Join("deploy", "bin", "mtxrpicam"),
		info: CameraInfo{
			Name:         "RPi Camera",
			Manufacturer: "Raspberry Pi",
			Model:        "OV5647",
			Width:        1280,
			Height:       720,
			FPS:          15,
			Codec:        "H264",
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	c.info.Width = c.params.Width
	c.info.Height = c.params.Height
	c.info.FPS = c.params.FPS

	return c
}

// Start begins capturing frames. The mtxrpicam subprocess is automatically
// restarted if it dies. Call Stop() to shut down.
func (c *RPiCamera) Start(ctx context.Context) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.started {
		return fmt.Errorf("camera already started")
	}

	// Validate binary exists
	if _, err := os.Stat(c.binPath); err != nil {
		return fmt.Errorf("mtxrpicam binary not found at %s: %w", c.binPath, err)
	}

	// Wrap context with cancel so Stop() can wake up run() from backoff selects
	c.ctx, c.cancel = context.WithCancel(ctx)
	c.started = true
	c.framesCh = make(chan Frame, 30) // buffer ~2 seconds at 15fps

	c.wg.Add(1)
	go c.run()

	return nil
}

// run is the main loop that spawns and monitors the mtxrpicam subprocess.
// It runs in its own goroutine and handles automatic restart on subprocess
// death with exponential backoff.
func (c *RPiCamera) run() {
	defer c.wg.Done()

	backoff := time.Second
	const maxBackoff = 30 * time.Second

	for {
		// Check if we should stop
		c.mu.RLock()
		stopped := c.stopped
		c.mu.RUnlock()
		if stopped {
			return
		}

		// Spawn subprocess
		err := c.spawnSubprocess()
		if err != nil {
			log.Printf("camera: spawn failed: %v, retrying in %v", err, backoff)
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(backoff):
				backoff = min(backoff*2, maxBackoff)
				continue
			}
		}

		// Reset backoff on successful spawn
		backoff = time.Second

		// readLoop blocks until subprocess dies or pipe error
		c.readLoop()

		// Clean up dead subprocess (keep framesCh alive)
		c.cleanupSubprocess()

		// Check if we should stop
		c.mu.RLock()
		stopped = c.stopped
		c.mu.RUnlock()
		if stopped {
			return
		}

		log.Printf("camera: subprocess died, restarting in %v", backoff)
		select {
		case <-c.ctx.Done():
			return
		case <-time.After(backoff):
			backoff = min(backoff*2, maxBackoff)
		}
	}
}

// spawnSubprocess creates pipes, spawns mtxrpicam, and sends initial config.
func (c *RPiCamera) spawnSubprocess() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var confFds, videoFds [2]int
	if err := syscall.Pipe(confFds[:]); err != nil {
		return fmt.Errorf("create conf pipe: %w", err)
	}
	if err := syscall.Pipe(videoFds[:]); err != nil {
		syscall.Close(confFds[0])
		syscall.Close(confFds[1])
		return fmt.Errorf("create video pipe: %w", err)
	}

	// Clear close-on-exec flag so FDs survive exec to child process.
	for _, fd := range []int{confFds[0], confFds[1], videoFds[0], videoFds[1]} {
		flags, _ := unix.FcntlInt(uintptr(fd), unix.F_GETFD, 0)
		unix.FcntlInt(uintptr(fd), unix.F_SETFD, flags&^unix.FD_CLOEXEC)
	}

	binPath, err := filepath.Abs(c.binPath)
	if err != nil {
		syscall.Close(confFds[0])
		syscall.Close(confFds[1])
		syscall.Close(videoFds[0])
		syscall.Close(videoFds[1])
		return fmt.Errorf("resolve binary path: %w", err)
	}

	binDir := filepath.Dir(binPath)

	// Resolve binDir to absolute path so LD_LIBRARY_PATH works
	// even after cmd.Dir changes the subprocess working directory.
	if absBinDir, err := filepath.Abs(binDir); err == nil {
		binDir = absBinDir
	}
	env := []string{
		"PIPE_CONF_FD=" + strconv.Itoa(confFds[0]),
		"PIPE_VIDEO_FD=" + strconv.Itoa(videoFds[1]),
		"LD_LIBRARY_PATH=" + binDir,
		"HOME=" + os.Getenv("HOME"),
		"PATH=" + os.Getenv("PATH"),
	}

	// Pass through libcamera-related environment variables so the subprocess
	// can find IPA tuning files, module paths, etc. Critical for cameras that
	// need system-installed IPA config (e.g. IMX219 on RPi with bundled libcamera).
	for _, key := range []string{"LIBCAMERA_IPA_CONFIG_PATH", "LIBCAMERA_IPA_MODULE_PATH", "LIBCAMERA_LOG_LEVELS"} {
		if v := os.Getenv(key); v != "" {
			env = append(env, key+"="+v)
		}
	}

	c.cmd = exec.CommandContext(c.ctx, binPath)
	c.cmd.Env = env
	c.cmd.Dir = binDir
	c.cmd.Stdout = os.Stdout
	c.cmd.Stderr = os.Stderr
	c.cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}

	if err := c.cmd.Start(); err != nil {
		syscall.Close(confFds[0])
		syscall.Close(confFds[1])
		syscall.Close(videoFds[0])
		syscall.Close(videoFds[1])
		c.cmd = nil
		return fmt.Errorf("start mtxrpicam: %w", err)
	}

	// Close the ends that belong to the child process
	syscall.Close(confFds[0])  // child reads config
	syscall.Close(videoFds[1]) // child writes video

	// Initialize pipe wrappers
	confWriteFile := os.NewFile(uintptr(confFds[1]), "conf-write")
	videoReadFile := os.NewFile(uintptr(videoFds[0]), "video-read")
	c.confPipe = newPipe(nil, confWriteFile)
	c.videoPipe = newPipe(videoReadFile, nil)

	// Send initial config
	cmd, err := c.params.SerializeCommand()
	if err != nil {
		c.cleanupSubprocessLocked()
		return fmt.Errorf("serialize initial config: %w", err)
	}
	if err := c.confPipe.write(cmd); err != nil {
		// Config send failed — subprocess won't produce frames
		c.cleanupSubprocessLocked()
		return fmt.Errorf("send initial config: %w", err)
	}

	log.Printf("camera: mtxrpicam subprocess started (pid %d)", c.cmd.Process.Pid)
	return nil
}

// Stop gracefully stops the camera and its subprocess.
func (c *RPiCamera) Stop() error {
	c.stopOnce.Do(func() {
		// Signal run() to stop accepting restarts
		c.mu.Lock()
		c.stopped = true
		if c.cancel != nil {
			c.cancel()
		}
		c.mu.Unlock()

		// Kill subprocess and close pipes (causes readLoop to exit)
		c.cleanupSubprocess()

		// Close frames channel so downstream consumers exit their range loops
		c.mu.Lock()
		if c.framesCh != nil {
			close(c.framesCh)
			c.framesCh = nil
		}
		c.mu.Unlock()

		// Wait for run() goroutine to finish (with timeout)
		c.waitForShutdown()
	})

	return nil
}

// waitForShutdown waits for the run() goroutine with a timeout.
func (c *RPiCamera) waitForShutdown() {
	done := make(chan struct{})
	go func() {
		c.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// run() exited normally
	case <-time.After(10 * time.Second):
		log.Printf("camera: Stop timed out waiting for run() goroutine after 10s")
	}
}

// cleanupSubprocess cleans up the mtxrpicam subprocess and pipes.
// Does NOT close framesCh — that's only done by Stop().
func (c *RPiCamera) cleanupSubprocess() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cleanupSubprocessLocked()
}

// cleanupSubprocessLocked cleans up subprocess resources with mu already held.
func (c *RPiCamera) cleanupSubprocessLocked() {
	if c.confPipe != nil {
		if closer, ok := c.confPipe.writer.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		c.confPipe = nil
	}

	if c.videoPipe != nil {
		if closer, ok := c.videoPipe.reader.(interface{ Close() error }); ok {
			_ = closer.Close()
		}
		c.videoPipe = nil
	}

	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_ = c.cmd.Wait()
		c.cmd = nil
	}
}

// Frames returns the read-only channel of captured frames.
func (c *RPiCamera) Frames() <-chan Frame {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.framesCh
}

// SetParam modifies a camera parameter and sends the update to the subprocess.
// If the subprocess is between restarts, the param is saved in memory and
// applied when the next subprocess spawns.
func (c *RPiCamera) SetParam(name string, value interface{}) error {
	c.mu.Lock()

	if !c.started || c.stopped {
		c.mu.Unlock()
		return fmt.Errorf("camera not started")
	}

	paramName, ok := mapParamName(name)
	if !ok {
		c.mu.Unlock()
		return fmt.Errorf("unknown parameter: %s", name)
	}

	if err := setParamValue(&c.params, paramName, value); err != nil {
		c.mu.Unlock()
		return fmt.Errorf("set %s: %w", name, err)
	}

	cmd, serializeErr := c.params.SerializeCommand()
	if serializeErr != nil {
		c.mu.Unlock()
		return fmt.Errorf("serialize params for %s: %w", name, serializeErr)
	}
	confPipe := c.confPipe

	// Update info if resolution/FPS changed
	c.info.Width = c.params.Width
	c.info.Height = c.params.Height
	c.info.FPS = c.params.FPS

	c.mu.Unlock()

	// Write to pipe OUTSIDE the lock to avoid blocking other operations.
	// If the pipe write fails, the param is saved in memory and will be
	// applied on the next subprocess restart. Error is propagated to the
	// caller so they know it may not take effect immediately.
	if confPipe != nil {
		if err := confPipe.write(cmd); err != nil {
			return fmt.Errorf("param %s saved but not applied (subprocess pipe write failed): %w", name, err)
		}
	}

	return nil
}

// GetParam returns the current value of a camera parameter.
func (c *RPiCamera) GetParam(name string) (interface{}, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	paramName, ok := mapParamName(name)
	if !ok {
		return nil, fmt.Errorf("unknown parameter: %s", name)
	}

	return getParamValue(c.params, paramName)
}

// Info returns the camera device information.
func (c *RPiCamera) Info() CameraInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.info
}

// DroppedFrames returns the count of frames dropped due to slow consumers.
func (c *RPiCamera) DroppedFrames() uint64 {
	return c.droppedFrames.Load()
}

// readLoop reads frames from the video pipe and sends them to the frames channel.
// Blocks until the pipe is closed or an error occurs.
// Called synchronously from run() — does NOT clean up subprocess on exit.
func (c *RPiCamera) readLoop() {
	c.mu.RLock()
	videoPipe := c.videoPipe
	framesCh := c.framesCh
	c.mu.RUnlock()

	if videoPipe == nil || framesCh == nil {
		return
	}

	for {
		buf, err := videoPipe.read()
		if err != nil {
			log.Printf("camera: pipe read error: %v", err)
			return
		}

		if len(buf) == 0 {
			continue
		}

		switch buf[0] {
		case 'e':
			// Error from subprocess
			errMsg := string(buf[1:])
			log.Printf("camera: mtxrpicam error: %s", errMsg)
			return

		case 'r':
			// Ready signal — subprocess is ready to capture
			continue

		case 'b': // v1.11.3 binary uses 'b' (master renamed to 'd')
			// Video frame data
			if len(buf) < 9 {
				continue
			}

			// Parse DTS (8 bytes, little-endian)
			dts := int64(buf[8])<<56 | int64(buf[7])<<48 | int64(buf[6])<<40 |
				int64(buf[5])<<32 | int64(buf[4])<<24 | int64(buf[3])<<16 |
				int64(buf[2])<<8 | int64(buf[1])

			// Convert DTS (microseconds) to PTS (90kHz)
			pts := multiplyAndDivide(dts, 90000, 1e6)

			// Calculate NTP timestamp
			ntp := time.Now()

			// Extract NALU data (everything after the 9-byte header)
			naluData := make([]byte, len(buf)-9)
			copy(naluData, buf[9:])

			frame := Frame{
				Data:      naluData,
				Timestamp: ntp,
				PTS:       pts,
			}

			// Non-blocking send — drop frame if channel full
			select {
			case framesCh <- frame:
			default:
				c.droppedFrames.Add(1)
				// Frame dropped — consumer too slow
			}

		default:
			// Unknown message type — ignore
			continue
		}
	}
}

// multiplyAndDivide performs (v * m / d) without overflow.
// Portions derived from MediaMTX.
func multiplyAndDivide(v, m, d int64) int64 {
	secs := v / d
	dec := v % d
	return secs*m + dec*m/d
}

// isIDRFrame checks if the Annex-B NALU data contains an IDR frame.
// IDR NALU types in H.264: 5 (IDR slice) or SPS (type 7) followed by IDR.
func isIDRFrame(data []byte) bool {
	if len(data) < 4 {
		return false
	}

	// Annex-B start code: 00 00 00 01 or 00 00 01
	// After start code, NALU header: first byte, lower 5 bits = NALU type
	// Type 5 = IDR slice
	i := 0
	for i < len(data)-3 {
		if data[i] == 0 && data[i+1] == 0 {
			// 4-byte start code: 00 00 00 01
			if i+3 < len(data) && data[i+2] == 0 && data[i+3] == 1 {
				if i+4 < len(data) {
					naluType := data[i+4] & 0x1F
					if naluType == 5 {
						return true
					}
				}
				i += 5
				continue
			}
			// 3-byte start code: 00 00 01
			if data[i+2] == 1 {
				naluType := data[i+3] & 0x1F
				if naluType == 5 {
					return true
				}
				i += 4
				continue
			}
		}
		i++
	}
	return false
}

// mapParamName maps user-facing parameter names to internal param field names.
func mapParamName(name string) (string, bool) {
	mapping := map[string]string{
		"brightness":   "Brightness",
		"contrast":     "Contrast",
		"saturation":   "Saturation",
		"sharpness":    "Sharpness",
		"width":        "Width",
		"height":       "Height",
		"fps":          "FPS",
		"exposure":     "Exposure",
		"exposureMode": "Exposure",
		"gain":         "Gain",
		"awbMode":      "AWB",
		"hFlip":        "HFlip",
		"vFlip":        "VFlip",
		"shutter":      "Shutter",
		"denoise":      "Denoise",
		"ev":           "EV",
		"bitrate":      "Bitrate",
		"idrPeriod":    "IDRPeriod",
		"metering":     "Metering",
		"mode":         "Mode",
		"hdr":          "HDR",
		"awbGainRed":   "AWBGainRed",
		"awbGainBlue":  "AWBGainBlue",
		"codec":        "Codec",
		"cameraID":     "CameraID",
	}

	internal, ok := mapping[name]
	return internal, ok
}
