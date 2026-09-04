package ai

// Service wires the frame decoder, the detector and the API surface
// together: it runs at most one inference per interval, keeps the latest
// snapshot for GET /api/detections, and publishes ai_detection events for
// the SSE hub. Fail-open everywhere: a missing model or ONNX Runtime
// library leaves the service inactive (ai:false), never fake data.

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/h264"
)

// Service runs the detection loop for one camera.
type Service struct {
	detector  Detector
	decoder   *FrameDecoder
	opts      Options
	mu        sync.RWMutex
	snap      Snapshot
	events    chan Event
	inference atomic.Uint64
	frame     atomic.Uint64
	active    bool
}

// NewService builds the AI service from options. It returns (nil, nil)
// when disabled or unavailable (fail-open) — callers treat nil as
// ai:false. The detector factory is injected so tests can stub inference.
func NewService(opts Options, hub *h264.AUHub, newDetector func(Options) (Detector, error)) *Service {
	opts = opts.withDefaults()
	if !opts.Enabled {
		slog.Info("ai: disabled by configuration")
		return nil
	}
	detector, err := newDetector(opts)
	if err != nil {
		slog.Warn("ai: detector unavailable, AI stays disabled (fail-open)", "error", err)
		return nil
	}
	return &Service{
		detector: detector,
		decoder:  NewFrameDecoder(hub, opts.DecoderBin),
		opts:     opts,
		events:   make(chan Event, 16),
		active:   true,
		snap:     Snapshot{Detections: []Detection{}, Model: detector.ModelName()},
	}
}

// Active reports whether a real detector is loaded.
func (s *Service) Active() bool { return s != nil && s.active }

// ModelName identifies the active model ("" when inactive).
func (s *Service) ModelName() string {
	if !s.Active() {
		return ""
	}
	return s.detector.ModelName()
}

// Snapshot returns the latest detections (SPEC v1 §4.6 shape).
func (s *Service) Snapshot() Snapshot {
	if !s.Active() {
		return Snapshot{}
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.snap
}

// Events exposes the ai_detection SSE stream (SPEC v1 §6). The channel is
// never closed; consumers must stop with the context.
func (s *Service) Events() <-chan Event { return s.events }

// Inferences returns the completed-inference counter (for /metrics).
func (s *Service) Inferences() uint64 {
	if !s.Active() {
		return 0
	}
	return s.inference.Load()
}

// Start runs the decode + inference loop until ctx is cancelled.
func (s *Service) Start(ctx context.Context) {
	if !s.Active() {
		return
	}
	slog.Info("ai: service started", "model", s.detector.ModelName(),
		"interval_ms", s.opts.IntervalMs, "threshold", s.opts.ConfidenceThreshold,
		"decoder", s.opts.DecoderBin,
		"frame", fmt.Sprintf("%dx%d", decoderFrameW, decoderFrameH))

	s.decoder.Start(ctx)
	go s.runLoop(ctx, s.decoder.Frames())
}

// runLoop consumes decoded frames and runs at most one inference per
// interval. Exposed for tests, which feed synthetic frame channels.
func (s *Service) runLoop(ctx context.Context, frames <-chan Frame) {
	interval := time.Duration(s.opts.IntervalMs) * time.Millisecond
	lastRun := time.Now().Add(-interval)
	for frame := range frames {
		if ctx.Err() != nil {
			return
		}
		if time.Since(lastRun) < interval {
			continue
		}
		lastRun = time.Now()
		frameNo := s.frame.Add(1)

		detections, err := s.detector.Detect(&frame, s.opts.VideoW, s.opts.VideoH)
		if err != nil {
			slog.Warn("ai: inference error", "error", err)
			s.storeSnapshot(nil)
			continue
		}
		kept := make([]Detection, 0, len(detections))
		for _, d := range detections {
			if d.Confidence >= s.opts.ConfidenceThreshold {
				kept = append(kept, d)
			}
		}
		s.inference.Add(1)
		s.storeSnapshot(kept)

		select {
		case s.events <- Event{CameraID: "0", Detections: kept, FrameNumber: frameNo}:
		default: // no SSE consumers / slow — drop, snapshots remain fresh
		}
	}
	slog.Info("ai: decoder stopped, service loop exiting")
}

func (s *Service) storeSnapshot(detections []Detection) {
	if detections == nil {
		detections = []Detection{}
	}
	s.mu.Lock()
	s.snap = Snapshot{
		Detections: detections,
		Model:      s.detector.ModelName(),
		Timestamp:  time.Now().Unix(),
	}
	s.mu.Unlock()
}
