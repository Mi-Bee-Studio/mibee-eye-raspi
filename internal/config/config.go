package config

import (
	"errors"
	"fmt"
	"log"
	"os"
	"strconv"

	"gopkg.in/yaml.v3"
)

// CameraConfig holds camera capture settings.
type CameraConfig struct {
	Device      string  `yaml:"device"`       // Camera device path
	Mode        string  `yaml:"mode"`         // Capture mode: "mtxrpicam" (default) or "rtsp"
	RTSPURL     string  `yaml:"rtsp_url"`     // External RTSP URL when mode=rtsp
	Width       int     `yaml:"width"`        // Capture width in pixels
	Height      int     `yaml:"height"`       // Capture height in pixels
	FPS         int     `yaml:"fps"`          // Frames per second
	Codec       string  `yaml:"codec"`        // Video codec (h264)
	Bitrate     int     `yaml:"bitrate"`      // Target bitrate in bps
	Brightness  float64 `yaml:"brightness"`   // -1.0 to 1.0
	Contrast    float64 `yaml:"contrast"`     // 0.0 to 32.0
	Saturation  float64 `yaml:"saturation"`   // 0.0 to 32.0
	Sharpness   float64 `yaml:"sharpness"`    // 0.0 to 16.0
}

// RTSPConfig holds RTSP server settings.
type RTSPConfig struct {
	Port     int    `yaml:"port"`     // RTSP port
	Username string `yaml:"username"` // RTSP authentication username
	Password string `yaml:"password"` // RTSP authentication password
}

// ONVIFConfig holds ONVIF server settings.
type ONVIFConfig struct {
	Port     int    `yaml:"port"`     // ONVIF HTTP port
	Username string `yaml:"username"` // ONVIF WS-UsernameToken username
	Password string `yaml:"password"` // ONVIF WS-UsernameToken password
}

// WebConfig holds Web UI server settings.
// The web UI serves a single-page admin panel for ONVIF config and camera params.
// When Username/Password are empty, the web server reuses the ONVIF credentials.
type WebConfig struct {
	Enabled  bool   `yaml:"enabled"`  // Enable Web UI server
	Port     int    `yaml:"port"`     // Web UI HTTP port
	Username string `yaml:"username"` // HTTP Basic auth user (empty -> onvif.username)
	Password string `yaml:"password"` // HTTP Basic auth pass (empty -> onvif.password)
}


// DeviceConfig holds ONVIF device information.
type DeviceConfig struct {
	Name         string `yaml:"name"`         // Camera friendly name
	Manufacturer string `yaml:"manufacturer"` // Device manufacturer
	Model        string `yaml:"model"`        // Device model
	Firmware     string `yaml:"firmware"`     // Firmware version
	HardwareID   string `yaml:"hardware_id"`  // Hardware identifier
	SerialNumber string `yaml:"serial_number"` // Device serial number
}

// LoggingConfig holds logging settings.
type LoggingConfig struct {
	Level string `yaml:"level"` // Log level (debug, info, warn, error)
}

// Config is the top-level configuration for MiBee Eye.
type Config struct {
	Camera  CameraConfig  `yaml:"camera"`
	RTSP    RTSPConfig    `yaml:"rtsp"`
	ONVIF   ONVIFConfig   `yaml:"onvif"`
	Device  DeviceConfig  `yaml:"device"`
	Logging LoggingConfig `yaml:"logging"`
	Web     WebConfig    `yaml:"web"`
}

// DefaultConfig returns a Config with all default values.
func DefaultConfig() *Config {
	return &Config{
		Camera: CameraConfig{
			Device:     "/dev/video0",
			Mode:       "mtxrpicam",
			RTSPURL:    "",
			Width:      1280,
			Height:     720,
			FPS:        15,
			Codec:      "h264",
			Bitrate:    2_000_000,
			Brightness: 0.0,
			Contrast:   1.0,
			Saturation: 1.0,
			Sharpness:  1.0,
		},
		RTSP: RTSPConfig{
			Port:     8554,
			Username: "",
			Password: "",
		},
		ONVIF: ONVIFConfig{
			Port:     8080,
			Username: "admin",
			Password: "",
		},
		Device: DeviceConfig{
			Name:         "Pi Camera V1",
			Manufacturer: "Raspberry Pi",
			Model:        "OV5647",
			Firmware:     "1.0.0",
			HardwareID:   "OV5647",
			SerialNumber: "",
		},
		Logging: LoggingConfig{
			Level: "info",
		},
		Web: WebConfig{
			Enabled: true,
			Port:    8088,
		},
	}
}

// Load reads a YAML configuration file at path and returns a Config.
// Values from the file are merged over DefaultConfig().
// Environment variables with the MIBEE_EYE_ prefix override both.
func Load(path string) (*Config, error) {
	cfg := DefaultConfig()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	if err := yaml.Unmarshal(data, cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	applyEnvOverrides(cfg)

	if err := cfg.Validate(); err != nil {
		return nil, err
	}

	if cfg.ONVIF.Password == "" {
		log.Printf("WARNING: ONVIF password is empty. Set onvif.password in config or MIBEE_EYE_ONVIF_PASSWORD env var")
	}

	return cfg, nil
}

// applyEnvOverrides applies MIBEE_EYE_ prefixed environment variables to config fields.
// Environment variable names follow the pattern MIBEE_EYE_<SECTION>_<FIELD>.
func applyEnvOverrides(cfg *Config) {
	// Camera section
	overrideString("MIBEE_EYE_CAMERA_DEVICE", &cfg.Camera.Device)
	overrideInt("MIBEE_EYE_CAMERA_WIDTH", &cfg.Camera.Width)
	overrideInt("MIBEE_EYE_CAMERA_HEIGHT", &cfg.Camera.Height)
	overrideInt("MIBEE_EYE_CAMERA_FPS", &cfg.Camera.FPS)
	overrideString("MIBEE_EYE_CAMERA_CODEC", &cfg.Camera.Codec)
	overrideInt("MIBEE_EYE_CAMERA_BITRATE", &cfg.Camera.Bitrate)
	overrideFloat("MIBEE_EYE_CAMERA_BRIGHTNESS", &cfg.Camera.Brightness)
	overrideFloat("MIBEE_EYE_CAMERA_CONTRAST", &cfg.Camera.Contrast)
	overrideFloat("MIBEE_EYE_CAMERA_SATURATION", &cfg.Camera.Saturation)
	overrideFloat("MIBEE_EYE_CAMERA_SHARPNESS", &cfg.Camera.Sharpness)

	// RTSP section
	overrideInt("MIBEE_EYE_RTSP_PORT", &cfg.RTSP.Port)
	overrideString("MIBEE_EYE_RTSP_USERNAME", &cfg.RTSP.Username)
	overrideString("MIBEE_EYE_RTSP_PASSWORD", &cfg.RTSP.Password)

	// ONVIF section
	overrideInt("MIBEE_EYE_ONVIF_PORT", &cfg.ONVIF.Port)
	overrideString("MIBEE_EYE_ONVIF_USERNAME", &cfg.ONVIF.Username)
	overrideString("MIBEE_EYE_ONVIF_PASSWORD", &cfg.ONVIF.Password)
	// Web section
	overrideBool("MIBEE_EYE_WEB_ENABLED", &cfg.Web.Enabled)
	overrideInt("MIBEE_EYE_WEB_PORT", &cfg.Web.Port)
	overrideString("MIBEE_EYE_WEB_USERNAME", &cfg.Web.Username)
	overrideString("MIBEE_EYE_WEB_PASSWORD", &cfg.Web.Password)

	// Device section
	overrideString("MIBEE_EYE_DEVICE_NAME", &cfg.Device.Name)
	overrideString("MIBEE_EYE_DEVICE_MANUFACTURER", &cfg.Device.Manufacturer)
	overrideString("MIBEE_EYE_DEVICE_MODEL", &cfg.Device.Model)
	overrideString("MIBEE_EYE_DEVICE_FIRMWARE", &cfg.Device.Firmware)
	overrideString("MIBEE_EYE_DEVICE_HARDWAREID", &cfg.Device.HardwareID)
	overrideString("MIBEE_EYE_DEVICE_SERIALNUMBER", &cfg.Device.SerialNumber)

	// Logging section
	overrideString("MIBEE_EYE_LOGGING_LEVEL", &cfg.Logging.Level)
}

// Sentinel errors for config validation.
var (
	errMustBePositive  = errors.New("must be positive")
	errInvalidCodec    = errors.New("codec must be h264 or h265")
	errInvalidLogLevel = errors.New("level must be debug, info, warn, or error")
)

// Validate checks all config fields and returns a detailed error if any are invalid.
// It uses fmt.Errorf with %%w wrapping for the inner validation error.
func (c *Config) Validate() error {
	if c.Camera.FPS <= 0 {
		return fmt.Errorf("config.camera.fps: %w", errMustBePositive)
	}
	if c.Camera.Width <= 0 {
		return fmt.Errorf("config.camera.width: %w", errMustBePositive)
	}
	if c.Camera.Height <= 0 {
		return fmt.Errorf("config.camera.height: %w", errMustBePositive)
	}
	if c.Camera.Bitrate <= 0 {
		return fmt.Errorf("config.camera.bitrate: %w", errMustBePositive)
	}
	if c.Camera.Brightness < -1.0 || c.Camera.Brightness > 1.0 {
		return fmt.Errorf("config.camera.brightness: %w", fmt.Errorf("out of range [-1.0, 1.0]"))
	}
	if c.Camera.Contrast < 0.0 || c.Camera.Contrast > 32.0 {
		return fmt.Errorf("config.camera.contrast: %w", fmt.Errorf("out of range [0.0, 32.0]"))
	}
	if c.Camera.Saturation < 0.0 || c.Camera.Saturation > 32.0 {
		return fmt.Errorf("config.camera.saturation: %w", fmt.Errorf("out of range [0.0, 32.0]"))
	}
	if c.Camera.Sharpness < 0.0 || c.Camera.Sharpness > 16.0 {
		return fmt.Errorf("config.camera.sharpness: %w", fmt.Errorf("out of range [0.0, 16.0]"))
	}
	if c.Camera.Codec != "h264" && c.Camera.Codec != "h265" {
		return fmt.Errorf("config.camera.codec: %w", errInvalidCodec)
	}
	if c.RTSP.Port <= 0 {
		return fmt.Errorf("config.rtsp.port: %w", errMustBePositive)
	}
	if c.ONVIF.Port <= 0 {
		return fmt.Errorf("config.onvif.port: %w", errMustBePositive)
	}
	if c.Web.Enabled && c.Web.Port <= 0 {
		return fmt.Errorf("config.web.port: %w", errMustBePositive)
	}
	switch c.Logging.Level {
	case "debug", "info", "warn", "error":
		// valid
	default:
		return fmt.Errorf("config.logging.level: %w", errInvalidLogLevel)
	}
	return nil
}

func overrideString(envName string, dest *string) {
	if v, ok := os.LookupEnv(envName); ok {
		*dest = v
	}
}

func overrideInt(envName string, dest *int) {
	if v, ok := os.LookupEnv(envName); ok {
		if n, err := strconv.Atoi(v); err == nil {
			*dest = n
		}
	}
}

func overrideFloat(envName string, dest *float64) {
	if v, ok := os.LookupEnv(envName); ok {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			*dest = f
		}
	}
}

func overrideBool(envName string, dest *bool) {
	if v, ok := os.LookupEnv(envName); ok {
		if b, err := strconv.ParseBool(v); err == nil {
			*dest = b
		}
	}
}
