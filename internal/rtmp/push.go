
// Package rtmp manages FFmpeg subprocess for RTSP→RTMP stream push.
package rtmp

import (
	"context"
	"os/exec"
	"sync"
	"time"
)

// Status represents the RTMP push connection state.
type Status string

const (
	StatusDisconnected Status = "disconnected"
	StatusConnecting   Status = "connecting"
	StatusConnected    Status = "connected"
	StatusError        Status = "error"
)

const maxRestarts = 5
const restartWindow = 1 * time.Minute
const cooldownDuration = 5 * time.Minute
const restartDelay = 5 * time.Second


// Config holds RTMP push configuration.
type Config struct {
	Enabled bool
	URL     string
	RTSPURL string // Local RTSP URL (e.g., rtsp://localhost:8554/stream)
}

// Push manages FFmpeg subprocess for RTSP→RTMP push.
type Push struct {
	mu        sync.RWMutex
	status    Status
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	url       string // RTMP target URL
	rtspURL   string // Local RTSP URL to pull from
	enabled   bool
	ffmpegBin string // Path to ffmpeg binary (default: "ffmpeg")
	restartCount      int
	restartWindowStart time.Time
	cooldownUntil     time.Time
}

// New creates a new RTMP push manager.
func New(cfg Config) *Push {
	return &Push{
		status:    StatusDisconnected,
		url:       cfg.URL,
		rtspURL:   cfg.RTSPURL,
		enabled:   cfg.Enabled,
		ffmpegBin: "ffmpeg",
	}
}

// Start begins pushing RTSP to RTMP via FFmpeg subprocess.
// The FFmpeg command: ffmpeg -rtsp_transport tcp -i {rtspURL} -c copy -f flv {rtmpURL}
// On crash, auto-restarts up to maxRestarts times per restartWindow,
// then enters cooldownDuration cooldown before retrying.
func (p *Push) Start(ctx context.Context) error {
	p.mu.Lock()
	if !p.enabled {
		p.mu.Unlock()
		return nil
	}
	p.restartCount = 0
	p.restartWindowStart = time.Time{}
	p.cooldownUntil = time.Time{}
	p.mu.Unlock()

	ctx, cancel := context.WithCancel(ctx)

	p.mu.Lock()
	p.cancel = cancel
	p.mu.Unlock()

	go p.run(ctx)
	return nil
}

// Stop stops the FFmpeg subprocess and cancels auto-restart.
func (p *Push) Stop() error {
	p.mu.Lock()
	cancel := p.cancel
	p.cancel = nil
	cmd := p.cmd
	p.mu.Unlock()

	if cancel != nil {
		cancel()
	}

	if cmd != nil && cmd.Process != nil {
		_ = cmd.Process.Kill()
		_ = cmd.Wait() // REAP the zombie — prevents defunct child process
	}

	p.mu.Lock()
	p.status = StatusDisconnected
	p.restartCount = 0
	p.cooldownUntil = time.Time{}
	p.restartWindowStart = time.Time{}
	p.mu.Unlock()

	return nil
}

// Status returns current connection status.
func (p *Push) Status() Status {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.status
}

// SetURL changes the RTMP target URL (requires restart to take effect).
func (p *Push) SetURL(url string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.url = url
}

// URL returns the current RTMP target URL.
func (p *Push) URL() string {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.url
}

// Enabled returns whether RTMP push is enabled.
func (p *Push) Enabled() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.enabled
}

// buildCommand constructs the FFmpeg command.
func (p *Push) buildCommand() *exec.Cmd {
	return p.buildCommandWithBin(p.ffmpegBin)
}

func (p *Push) run(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		p.mu.Lock()

		// If in cooldown, wait until it ends, then reset
		if !p.cooldownUntil.IsZero() {
			if time.Now().Before(p.cooldownUntil) {
				remaining := time.Until(p.cooldownUntil)
				p.mu.Unlock()
				p.setStatus(StatusError)
				select {
				case <-ctx.Done():
					return
				case <-time.After(remaining):
				}
				p.mu.Lock()
			}
			p.restartCount = 0
			p.cooldownUntil = time.Time{}
			p.restartWindowStart = time.Time{}
		}

		// Reset count if restart window has expired
		if !p.restartWindowStart.IsZero() && time.Since(p.restartWindowStart) > restartWindow {
			p.restartCount = 0
			p.restartWindowStart = time.Now()
		}

		// Initialize window start on first restart
		if p.restartWindowStart.IsZero() {
			p.restartWindowStart = time.Now()
		}

		// Max restarts reached in this window — enter cooldown
		if p.restartCount >= maxRestarts {
			p.cooldownUntil = time.Now().Add(cooldownDuration)
			p.mu.Unlock()
			p.setStatus(StatusError)
			select {
			case <-ctx.Done():
				return
			case <-time.After(cooldownDuration):
			}
			p.mu.Lock()
			p.restartCount = 0
			p.cooldownUntil = time.Time{}
			p.restartWindowStart = time.Time{}
			p.mu.Unlock()
			continue
		}

		p.restartCount++
		p.mu.Unlock()

		p.setStatus(StatusConnecting)

		cmd := p.buildCommand()

		p.mu.Lock()
		p.cmd = cmd
		p.mu.Unlock()

		err := cmd.Start()
		if err != nil {
			p.setStatus(StatusError)
			select {
			case <-ctx.Done():
				return
			case <-time.After(restartDelay):
				continue
			}
		}

		p.setStatus(StatusConnected)

		err = cmd.Wait()

		p.mu.Lock()
		p.cmd = nil
		p.mu.Unlock()

		if ctx.Err() != nil {
			return
		}

		if err != nil {
			p.setStatus(StatusError)
		} else {
			p.setStatus(StatusDisconnected)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(restartDelay):
			continue
		}
	}
}

// CooldownUntil returns the cooldown end time (zero if not in cooldown).
// Exported for testing.
func (p *Push) CooldownUntil() time.Time {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.cooldownUntil
}

func (p *Push) setStatus(s Status) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.status = s
}

// buildCommandWithBin is like buildCommand but allows overriding the ffmpeg binary path.
// Exported for testing.
func (p *Push) buildCommandWithBin(bin string) *exec.Cmd {
	return exec.Command(
		bin,
		"-rtsp_transport", "tcp",
		"-i", p.rtspURL,
		"-c", "copy",
		"-f", "flv",
		p.url,
	)
}

// RestartCount returns the number of restart attempts.
// Exported for testing.
func (p *Push) RestartCount() int {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.restartCount
}

// SetStatus directly sets the status. Used only in tests.
func (p *Push) SetStatus(s Status) {
	p.setStatus(s)
}
func (p *Push) SetCancel(f context.CancelFunc) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cancel = f
}

// SetCmd sets the command. Used only in tests.
func (p *Push) SetCmd(cmd *exec.Cmd) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cmd = cmd
}
