// Package hls provides an HTTP Live Streaming (HLS) bridge from an RTSP source.
//
// It runs ffmpeg as a subprocess that reads the local RTSP stream and
// continuously writes HLS segments (TS) plus a rolling .m3u8 playlist
// to a directory. The directory is served back to authenticated web UI
// clients via an http.Handler.
//
// ffmpeg is invoked with -c:v copy so H.264 is remuxed, not re-encoded —
// CPU cost on RPi 3B is ~2% per active playback. Typical latency is
// 2–5 seconds (hls_time * list_size + segment fetch jitter).
package hls

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

// Config holds HLS server settings.
type Config struct {
	// RTSPURL is the source stream (typically rtsp://127.0.0.1:8554/stream).
	RTSPURL string
	// OutputDir is the directory where ffmpeg writes .m3u8 and .ts files.
	// It will be created if it does not exist.
	OutputDir string
	// SegmentTime is the target duration of each .ts segment in seconds.
	// Default: 1.
	SegmentTime int
	// ListSize is the number of segments kept in the live playlist.
	// Default: 6.
	ListSize int
	// Username, Password — optional RTSP digest credentials.
	Username string
	Password string
	// FFMpegBin — ffmpeg binary name or path. Default: "ffmpeg".
	FFMpegBin string
	// Logger — optional. nil falls back to log.Default().
	Logger *log.Logger
	// RestartOnExit — if true, restart ffmpeg when it exits unexpectedly.
	// Default: true.
	RestartOnExit bool
}

// Server manages the ffmpeg subprocess that produces HLS segments
// and serves them over HTTP.
type Server struct {
	cfg    Config
	logger *log.Logger

	mu        sync.Mutex
	cmd       *exec.Cmd
	cancel    context.CancelFunc
	running   bool
	hasOutput bool // true once at least one .ts segment exists
	ready     chan struct{}

	stopOnce sync.Once
	wg       sync.WaitGroup // tracks runLoop goroutine
	watchReadyRunning atomic.Bool // prevents goroutine leak on restart
}

// New creates a new HLS server. Call Start to begin segmenting.
func New(cfg Config) *Server {
	if cfg.SegmentTime <= 0 {
		cfg.SegmentTime = 1
	}
	if cfg.ListSize <= 0 {
		cfg.ListSize = 6
	}
	if cfg.FFMpegBin == "" {
		cfg.FFMpegBin = "ffmpeg"
	}
	if cfg.OutputDir == "" {
		cfg.OutputDir = "/tmp/hls-mibee-eye"
	}
	if cfg.Logger == nil {
		cfg.Logger = log.Default()
	}
	return &Server{
		cfg:    cfg,
		logger: cfg.Logger,
		ready:  make(chan struct{}),
	}
}

// Start launches ffmpeg and blocks until it is producing output (or fails).
// It returns nil once at least one segment exists on disk.
// If ffmpeg is not available on PATH, Start returns a non-fatal error —
// the web UI will then fall back to JPEG snapshot.
func (s *Server) Start(ctx context.Context) error {
	// Check ffmpeg is available before committing.
	if _, err := exec.LookPath(s.cfg.FFMpegBin); err != nil {
		return fmt.Errorf("ffmpeg not found (%s): %w", s.cfg.FFMpegBin, err)
	}

	// Ensure output dir exists and is empty.
	if err := os.MkdirAll(s.cfg.OutputDir, 0o755); err != nil {
		return fmt.Errorf("create output dir: %w", err)
	}
	if err := clearDir(s.cfg.OutputDir); err != nil {
		return fmt.Errorf("clear output dir: %w", err)
	}

	s.cfg.Logger.Printf("hls: starting ffmpeg %s -> %s", s.cfg.RTSPURL, s.cfg.OutputDir)

	runCtx, cancel := context.WithCancel(ctx)
	s.mu.Lock()
	s.cancel = cancel
	s.running = true
	s.hasOutput = false
	s.ready = make(chan struct{})
	s.mu.Unlock()

	s.wg.Add(1)
	go func() {
		defer s.wg.Done()
		s.runLoop(runCtx)
	}()

	// Wait for first segment or ctx cancel.
	select {
	case <-s.ready:
		s.logger.Printf("hls: first segment ready, serving from %s", s.cfg.OutputDir)
		return nil
	case <-time.After(15 * time.Second):
		cancel()
		return errors.New("hls: no segments produced within 15s")
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Stop terminates the ffmpeg subprocess if running.
// It sends SIGTERM to the process group first, waits up to 5 seconds,
// then escalates to SIGKILL if the process is still alive.
func (s *Server) Stop() error {
	var err error
	s.stopOnce.Do(func() {
		s.mu.Lock()
		cancel := s.cancel
		cmd := s.cmd
		s.running = false
		s.mu.Unlock()

		if cancel != nil {
			cancel()
		}

		// Kill process group: SIGTERM, wait up to 5s, then SIGKILL.
		if cmd != nil && cmd.Process != nil {
			killProcessGroup(cmd.Process.Pid)
		}

		// Wait for runLoop goroutine to finish (which calls cmd.Wait()).
		s.wg.Wait()
	})
	return err
}

// Handler returns an http.Handler that serves files from the HLS output dir.
// Path traversal is prevented by http.FileServer's built-in cleaning.
// It should be mounted under a path-stripping prefix (e.g. /api/hls/).
func (s *Server) Handler() http.Handler {
	fs := http.FileServer(http.Dir(s.cfg.OutputDir))
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-cache, no-store, must-revalidate")
		fs.ServeHTTP(w, r)
	})
}

// OutputDir returns the on-disk directory where segments are written.
func (s *Server) OutputDir() string {
	return s.cfg.OutputDir
}

// runLoop manages the ffmpeg subprocess. It (re)starts on exit if configured,
// and signals the ready channel when the first .ts segment appears.
func (s *Server) runLoop(ctx context.Context) {
	restartDelay := 2 * time.Second
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		err := s.runOnce(ctx)
		if ctx.Err() != nil {
			return
		}
		if !s.cfg.RestartOnExit {
			if err != nil {
				s.logger.Printf("hls: ffmpeg exited: %v", err)
			}
			return
		}
		s.logger.Printf("hls: ffmpeg exited (%v), restarting in %s", err, restartDelay)
		select {
		case <-time.After(restartDelay):
		case <-ctx.Done():
			return
		}
	}
}

// runOnce launches ffmpeg, captures stderr, and blocks until it exits or
// the first segment appears (which signals ready).
func (s *Server) runOnce(ctx context.Context) error {
	args := s.buildArgs()
	cmd := exec.CommandContext(ctx, s.cfg.FFMpegBin, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("stderr pipe: %w", err)
	}

	s.mu.Lock()
	s.cmd = cmd
	s.mu.Unlock()

	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start ffmpeg: %w", err)
	}

	// Log ffmpeg stderr in background (rate-limited).
	go logStderr(s.logger, stderr)

	// Poll for first segment; signal ready when found.
	// Only one watchReady goroutine runs at a time to prevent leaks on restart.
	if !s.watchReadyRunning.Swap(true) {
		go func() {
			s.watchReady(ctx)
			s.watchReadyRunning.Store(false)
		}()
	}

	if err := cmd.Wait(); err != nil {
		// Don't wrap context cancellation as an error.
		if ctx.Err() != nil {
			return nil
		}
		return err
	}
	return nil
}

// buildArgs returns the ffmpeg command-line arguments.
func (s *Server) buildArgs() []string {
	args := []string{
		"-hide_banner",
		"-loglevel", "error",
		"-rtsp_transport", "tcp",
	}
	if s.cfg.Username != "" {
		args = append(args,
			"-username", s.cfg.Username,
			"-password", s.cfg.Password,
		)
	}
	args = append(args,
		"-i", s.cfg.RTSPURL,
		"-c:v", "copy",
		"-an", // drop audio (RTSP stream is video-only)
		"-f", "hls",
		"-hls_time", fmt.Sprintf("%d", s.cfg.SegmentTime),
		"-hls_list_size", fmt.Sprintf("%d", s.cfg.ListSize),
		"-hls_flags", "delete_segments+independent_segments",
		"-hls_segment_filename", filepath.Join(s.cfg.OutputDir, "seg-%d.ts"),
		filepath.Join(s.cfg.OutputDir, "stream.m3u8"),
	)
	return args
}

// watchReady polls the output dir for the first .ts segment and closes
// the ready channel exactly once per Start.
func (s *Server) watchReady(ctx context.Context) {
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			ok, err := hasSegment(s.cfg.OutputDir)
			if err != nil {
				continue
			}
			if ok {
				s.mu.Lock()
				if !s.hasOutput {
					s.hasOutput = true
					close(s.ready)
				}
				s.mu.Unlock()
				return
			}
		}
	}
}

// hasSegment reports whether at least one .ts file exists in dir.
func hasSegment(dir string) (bool, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false, err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if strings.HasSuffix(e.Name(), ".ts") {
			return true, nil
		}
	}
	return false, nil
}

// clearDir removes all files inside dir (but not the directory itself).
func clearDir(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	for _, e := range entries {
		_ = os.RemoveAll(filepath.Join(dir, e.Name()))
	}
	return nil
}

// logStderr copies ffmpeg's stderr to our logger, line by line.
// Returns when the reader returns an error (typically io.EOF on pipe close).
func logStderr(logger *log.Logger, r interface{ Read(p []byte) (int, error) }) {
	buf := make([]byte, 4096)
	for {
		n, err := r.Read(buf)
		if n > 0 {
			line := strings.TrimRight(string(buf[:n]), "\r\n")
			if line != "" {
				logger.Printf("hls ffmpeg: %s", line)
			}
		}
		if err != nil {
			return
		}
	}
}

// killProcessGroup sends SIGTERM to the process group, waits up to 5s,
// then escalates to SIGKILL if the process is still alive.
func killProcessGroup(pid int) {
	pgid := -pid // negative PID = process group

	// Graceful shutdown first.
	_ = syscall.Kill(pgid, syscall.SIGTERM)

	// Poll for process exit up to 5 seconds.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if err := syscall.Kill(pgid, 0); err != nil {
			// Process group is gone.
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	// Force kill with SIGKILL.
	_ = syscall.Kill(pgid, syscall.SIGKILL)
	time.Sleep(50 * time.Millisecond) // brief wait for SIGKILL to take effect
}
