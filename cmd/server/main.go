package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/camera"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/config"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/onvif"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/ptz"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/rtsp"
	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/web"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

// configAdapter wraps config.Config to implement onvif.ConfigProvider.
type configAdapter struct {
	cfg      *config.Config
	deviceIP string
}

func (a *configAdapter) ONVIFUsername() string      { return a.cfg.ONVIF.Username }
func (a *configAdapter) ONVIFPassword() string      { return a.cfg.ONVIF.Password }
func (a *configAdapter) ONVIFPort() int             { return a.cfg.ONVIF.Port }
func (a *configAdapter) RTSPPort() int              { return a.cfg.RTSP.Port }
func (a *configAdapter) DeviceIP() string           { return a.deviceIP }
func (a *configAdapter) CameraDevice() string       { return a.cfg.Camera.Device }
func (a *configAdapter) CameraCodec() string        { return a.cfg.Camera.Codec }
func (a *configAdapter) CameraBitrate() int         { return a.cfg.Camera.Bitrate }
func (a *configAdapter) CameraWidth() int           { return a.cfg.Camera.Width }
func (a *configAdapter) CameraHeight() int          { return a.cfg.Camera.Height }
func (a *configAdapter) CameraFPS() int             { return a.cfg.Camera.FPS }
func (a *configAdapter) DeviceName() string         { return a.cfg.Device.Name }
func (a *configAdapter) DeviceManufacturer() string { return a.cfg.Device.Manufacturer }
func (a *configAdapter) DeviceModel() string        { return a.cfg.Device.Model }
func (a *configAdapter) DeviceFirmware() string     { return a.cfg.Device.Firmware }
func (a *configAdapter) DeviceHardwareID() string   { return a.cfg.Device.HardwareID }
func (a *configAdapter) DeviceSerialNumber() string { return a.cfg.Device.SerialNumber }
func (a *configAdapter) LoggingLevel() string       { return a.cfg.Logging.Level }

// detectLocalIP finds the first non-loopback IPv4 address.
func detectLocalIP() string {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return "127.0.0.1"
	}
	for _, addr := range addrs {
		if ipnet, ok := addr.(*net.IPNet); ok && !ipnet.IP.IsLoopback() && ipnet.IP.To4() != nil {
			return ipnet.IP.String()
		}
	}
	return "127.0.0.1"
}

// noOpCamera is a stub camera.Camera used in tests.
// Imaging parameter changes are validated but not applied to hardware.
type noOpCamera struct{}

func (c *noOpCamera) Start(ctx context.Context) error          { return nil }
func (c *noOpCamera) Stop() error                        { return nil }
func (c *noOpCamera) Frames() <-chan camera.Frame        { return nil }
func (c *noOpCamera) SetParam(name string, value interface{}) error { return nil }
func (c *noOpCamera) GetParam(name string) (interface{}, error) {
	switch name {
	case "brightness":
		return 0.0, nil
	case "contrast":
		return 1.0, nil
	case "saturation":
		return 1.0, nil
	case "sharpness":
		return 1.0, nil
	case "exposure":
		return 0, nil
	case "gain":
		return 1.0, nil
	case "width":
		return 1280, nil
	case "height":
		return 720, nil
	case "fps":
		return 15, nil
	default:
		return nil, nil
	}
}
func (c *noOpCamera) Info() camera.CameraInfo {
	return camera.CameraInfo{}
}
func main() {
	configPath := flag.String("config", "configs/config.yaml", "path to config file")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Fprintf(os.Stderr, "mibee-eye version %s\n", version)
		os.Exit(0)
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config: %v", err)
	}

	if cfg.ONVIF.Password == "" {
		log.Printf("WARNING: ONVIF password is empty. Set onvif.password in config or MIBEE_EYE_ONVIF_PASSWORD env var")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	localIP := detectLocalIP()
	log.Printf("MiBee Eye %s starting (fallback IP %s)", version, localIP)
adapter := &configAdapter{cfg: cfg, deviceIP: localIP}

	// --- Step 1: Camera ---
	cameraParams := camera.DefaultParams()
	cameraParams.Width = uint32(cfg.Camera.Width)
	cameraParams.Height = uint32(cfg.Camera.Height)
	cameraParams.FPS = float32(cfg.Camera.FPS)
	cameraParams.Bitrate = uint32(cfg.Camera.Bitrate)
	cameraParams.Brightness = float32(cfg.Camera.Brightness)
	cameraParams.Contrast = float32(cfg.Camera.Contrast)
	cameraParams.Saturation = float32(cfg.Camera.Saturation)
	cameraParams.Sharpness = float32(cfg.Camera.Sharpness)
	cameraParams.IDRPeriod = 15
	cameraParams.Codec = "hardwareH264"
	cameraInfo := camera.CameraInfo{
		Name:         cfg.Device.Name,
		Manufacturer: cfg.Device.Manufacturer,
		Model:        cfg.Device.Model,
		SerialNumber: cfg.Device.SerialNumber,
		Width:        uint32(cfg.Camera.Width),
		Height:       uint32(cfg.Camera.Height),
		FPS:          float32(cfg.Camera.FPS),
		Codec:        "H264",
	}

	externalRTSPURL := ""
	var cam camera.Camera
	if cfg.Camera.Mode == "rtsp" {
		externalRTSPURL = cfg.Camera.RTSPURL
		if externalRTSPURL == "" {
			externalRTSPURL = fmt.Sprintf("rtsp://127.0.0.1:%d/stream", cfg.RTSP.Port)
		}
		log.Printf("camera: using external RTSP source: %s", externalRTSPURL)
		cam = camera.NewRTSPSource(externalRTSPURL, cameraParams, cameraInfo)
	} else {
		cam = camera.NewRPiCamera(
			camera.WithBinPath("deploy/bin/mtxrpicam"),
			camera.WithParams(cameraParams),
			camera.WithInfo(cameraInfo),
		)
	}

	if err := cam.Start(ctx); err != nil {
		log.Fatalf("camera start: %v", err)
	}

	// --- Step 2: H264 Parser + AUHub ---
	parser := h264.NewParser()
	auHub := h264.NewAUHub()

	go func() {
		for frame := range cam.Frames() {
			nalus := parser.Parse(frame.Data)
			if len(nalus) == 0 {
				continue
			}
			au := h264.AccessUnit{
				NALUs:     nalus,
				Timestamp: frame.Timestamp,
				KeyFrame:  false,
			}
			for _, n := range nalus {
				if n.IsIDR {
					au.KeyFrame = true
					break
				}
			}
			auHub.Write(au)
		}
	}()

	// --- Step 3: RTSP Server (skipped when consuming external RTSP) ---
	var rtspServer *rtsp.Server
	if externalRTSPURL != "" {
	} else {
		rtspSub := auHub.Subscribe(ctx)
		rtspServer = rtsp.New(rtsp.Config{
			Port:     cfg.RTSP.Port,
			Username: cfg.RTSP.Username,
			Password: cfg.RTSP.Password,
			Address:  localIP,
		})
		rtspServer.SetFrameSource(rtspSub.Channel)

		if err := rtspServer.Start(ctx); err != nil {
			log.Fatalf("rtsp server start: %v", err)
		}
	}


	// --- Step 4: ParamManager + PTZ ---
	paramManager := camera.NewParamManager(cam)
	ptzState := ptz.NewState()

	// --- Step 5: ONVIF Server ---
	onvifServer := onvif.New(adapter)

	// fallbackHost is used only when the per-request client IP can't be determined
	// (e.g. test callers). Real ONVIF responses echo the NVR's source IP back as
	// the XAddr/RTSP host so the URL is reachable from whichever interface was used.
	deviceHost := fmt.Sprintf("%s:%d", localIP, cfg.ONVIF.Port)
	onvif.RegisterDeviceHandlers(onvifServer, deviceHost, onvif.DeviceInfo{
		Name:         cfg.Device.Name,
		Manufacturer: cfg.Device.Manufacturer,
		Model:        cfg.Device.Model,
		Firmware:     cfg.Device.Firmware,
		HardwareID:   cfg.Device.HardwareID,
		SerialNumber: cfg.Device.SerialNumber,
	})
	onvif.RegisterMediaHandlers(onvifServer)
	onvif.RegisterImagingHandlers(onvifServer, paramManager)
	onvif.RegisterPTZHandlers(onvifServer, ptzState)

	var webServer *web.Server
	// --- Step 5.5: Web UI Server ---
	if cfg.Web.Enabled {
		webServer = web.New(web.Config{
			Port:        cfg.Web.Port,
			Username:    cfg.Web.Username,
			Password:    cfg.Web.Password,
			ConfigPath:  *configPath,
			OnvifConfig: adapter,
			Params:      paramManager,
			PTZ:         ptzState,
			AUHub:       auHub,
			Version:     version,
		})
		go func() {
			if err := webServer.Start(ctx); err != nil {
				log.Printf("web server exited: %v", err)
			}
		}()
	}

	// --- Step 6: WS-Discovery ---
	discovery := onvif.NewDiscovery(cfg, localIP)
	if err := discovery.StartUDP(ctx); err != nil {
		log.Printf("warning: failed to start WS-Discovery: %v", err)
	}
	onvifServer.SetDiscoveryHandler(http.HandlerFunc(discovery.HandleHTTPProbe))

	// Start ONVIF HTTP server in goroutine
	go func() {
		if err := onvifServer.Start(ctx); err != nil {
			log.Printf("onvif server exited: %v", err)
		}
	}()

	<-ctx.Done()
	log.Printf("MiBee Eye %s shutting down...", version)
	shutdownStep("discovery", 5*time.Second, func() error { return discovery.StopUDP() })
	shutdownStep("onvif", 5*time.Second, func() error { return onvifServer.Stop() })
	shutdownStep("web", 5*time.Second, func() error {
		if webServer != nil {
			return webServer.Stop()
		}
		return nil
	})
	shutdownStep("rtsp", 5*time.Second, func() error {
		if rtspServer != nil {
			return rtspServer.Stop()
		}
		return nil
	})
	shutdownStep("camera", 5*time.Second, func() error { return cam.Stop() })

	log.Printf("MiBee Eye %s stopped", version)
}

// shutdownStep runs a shutdown function with a timeout.
// If the function does not complete within the timeout, a warning is logged
// and execution continues to the next step.
func shutdownStep(name string, timeout time.Duration, fn func() error) {
	log.Printf("shutdown: stopping %s...", name)
	timer := time.NewTimer(timeout)
	defer timer.Stop()

	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()

	select {
	case err := <-done:
		if err != nil {
			log.Printf("shutdown: %s stopped with error: %v", name, err)
		} else {
			log.Printf("shutdown: %s stopped", name)
		}
	case <-timer.C:
		log.Printf("shutdown: %s stop timed out after %v", name, timeout)
	}
}
