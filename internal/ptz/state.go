// Package ptz implements a digital PTZ state machine for virtual pan/tilt/zoom.
// It manages continuous movement, absolute/relative positioning, and preset storage.
// Position ranges: Pan [-1,1] (left to right), Tilt [-1,1] (down to up), Zoom [0,1] (full frame to max zoom).
package ptz

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sync"
	"time"
)

// Constants for movement timing, easing, and threshold values.
const (
	moveTickInterval   = 50 * time.Millisecond
	velocityStepSize   = 0.01
	absoluteMoveSteps  = 20
	absoluteEaseFactor = 0.15
	snapThreshold      = 0.001
)

// Position represents current PTZ coordinates.
type Position struct {
	Pan  float64 // -1.0 to 1.0 (left to right)
	Tilt float64 // -1.0 to 1.0 (down to up)
	Zoom float64 // 0.0 to 1.0 (full frame to max zoom)
}

// Velocity represents PTZ movement speed.
type Velocity struct {
	Pan  float64 // -1.0 to 1.0
	Tilt float64 // -1.0 to 1.0
	Zoom float64 // 0.0 to 1.0
}

// Preset stores a named PTZ position.
type Preset struct {
	Token    string
	Name     string
	Position Position
}

// State manages digital PTZ state with background movement.
type State struct {
	mu             sync.RWMutex
	position       Position
	velocity       Velocity
	presets        map[string]Preset
	moving         bool
	cancel         context.CancelFunc
	onPosChange    func(pos Position)
	onPresetChange func()
	persistPath    string // non-empty enables file persistence
}

// NewState creates a new PTZ state initialized at center position.
func NewState() *State {
	return &State{
		position: Position{Pan: 0, Tilt: 0, Zoom: 0},
		presets:  make(map[string]Preset),
	}
}

// clampf restricts v to [lo, hi].
func clampf(v, lo, hi float64) float64 {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

// clampPosition restricts all axes to valid ranges.
func clampPosition(p Position) Position {
	return Position{
		Pan:  clampf(p.Pan, -1, 1),
		Tilt: clampf(p.Tilt, -1, 1),
		Zoom: clampf(p.Zoom, 0, 1),
	}
}

// ContinuousMove starts velocity-based movement.
// The position is updated at 50ms tick intervals using: pos += velocity * 0.01
// Any previous movement is stopped before starting the new one.
func (s *State) ContinuousMove(vel Velocity) {
	s.stopInternal()

	s.mu.Lock()
	s.velocity = vel
	s.moving = true
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())

	s.mu.Lock()
	s.cancel = cancel
	s.mu.Unlock()

	go s.runMovement(ctx)
}

// runMovement updates position at 50ms intervals until context is cancelled.
func (s *State) runMovement(ctx context.Context) {
	ticker := time.NewTicker(moveTickInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.position.Pan = clampf(s.position.Pan+s.velocity.Pan*velocityStepSize, -1, 1)
			s.position.Tilt = clampf(s.position.Tilt+s.velocity.Tilt*velocityStepSize, -1, 1)
			s.position.Zoom = clampf(s.position.Zoom+s.velocity.Zoom*velocityStepSize, 0, 1)

			// Stop if we've hit a boundary and velocity would push further
			stopped := (s.velocity.Pan != 0 && s.position.Pan == clampf(s.velocity.Pan, -1, 1)) ||
				(s.velocity.Tilt != 0 && s.position.Tilt == clampf(s.velocity.Tilt, -1, 1)) ||
				(s.velocity.Zoom != 0 && s.position.Zoom == clampf(s.velocity.Zoom, 0, 1))
			// Actually let it continue — ContinuousMove stays active until Stop() is called,
			// even if at boundary. The position just won't change past the boundary.
			_ = stopped
			pos := s.position
			cb := s.onPosChange
			s.mu.Unlock()

			if cb != nil {
				cb(pos)
			}
		}
	}
}

// AbsoluteMove moves to exact position with exponential easing.
// Any previous movement is stopped.
func (s *State) AbsoluteMove(pos Position) {
	s.stopInternal()

	target := clampPosition(pos)

	s.mu.Lock()
	start := s.position
	s.mu.Unlock()

	ctx, cancel := context.WithCancel(context.Background())
	s.mu.Lock()
	s.cancel = cancel
	s.moving = true
	s.mu.Unlock()

	go s.runAbsoluteMove(ctx, start, target)
}

// runAbsoluteMove animates from start to target with exponential easing.
func (s *State) runAbsoluteMove(ctx context.Context, start, target Position) {
	ticker := time.NewTicker(moveTickInterval)
	defer ticker.Stop()

	for i := 0; i < absoluteMoveSteps; i++ {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.mu.Lock()
			s.position.Pan += (target.Pan - s.position.Pan) * absoluteEaseFactor
			s.position.Tilt += (target.Tilt - s.position.Tilt) * absoluteEaseFactor
			s.position.Zoom += (target.Zoom - s.position.Zoom) * absoluteEaseFactor

			// Snap to target when close enough
			if math.Abs(s.position.Pan-target.Pan) < snapThreshold {
				s.position.Pan = target.Pan
			}
			if math.Abs(s.position.Tilt-target.Tilt) < snapThreshold {
				s.position.Tilt = target.Tilt
			}
			if math.Abs(s.position.Zoom-target.Zoom) < snapThreshold {
				s.position.Zoom = target.Zoom
			}

			done := s.position == target
			pos := s.position
			cb := s.onPosChange
			s.mu.Unlock()

			if cb != nil {
				cb(pos)
			}

			if done {
				s.mu.Lock()
				s.moving = false
				s.velocity = Velocity{}
				s.mu.Unlock()
				return
			}
		}
	}

	// Ensure we reach exact target after all steps
	s.mu.Lock()
	s.position = target
	s.moving = false
	s.velocity = Velocity{}
	pos := s.position
	cb := s.onPosChange
	s.mu.Unlock()

	if cb != nil {
		cb(pos)
	}
}

// RelativeMove applies relative movement from current position.
// The delta is applied immediately (not animated).
func (s *State) RelativeMove(delta Velocity) {
	s.mu.Lock()
	s.position.Pan = clampf(s.position.Pan+delta.Pan, -1, 1)
	s.position.Tilt = clampf(s.position.Tilt+delta.Tilt, -1, 1)
	s.position.Zoom = clampf(s.position.Zoom+delta.Zoom, 0, 1)
	pos := s.position
	cb := s.onPosChange
	s.mu.Unlock()

	if cb != nil {
		cb(pos)
	}
}

// Stop halts all movement immediately.
func (s *State) Stop() {
	s.stopInternal()
}

// stopInternal stops movement without lock contention (caller handles locking).
func (s *State) stopInternal() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
		s.cancel = nil
	}
	s.velocity = Velocity{}
	s.moving = false
	s.mu.Unlock()
}

// GetPosition returns current position (thread-safe).
func (s *State) GetPosition() Position {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.position
}

// GetStatus returns movement status: "IDLE" or "MOVING".
func (s *State) GetStatus() string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.moving {
		return "MOVING"
	}
	return "IDLE"
}

// SetPreset stores current position as a named preset and persists if enabled.
func (s *State) SetPreset(token string, presetName string) error {
	s.mu.Lock()
	preset := Preset{
		Token:    token,
		Name:     presetName,
		Position: s.position,
	}
	if s.presets == nil {
		s.presets = make(map[string]Preset)
	}
	s.presets[token] = preset
	cb := s.onPresetChange
	path := s.persistPath
	var saveSnapshot []Preset
	if path != "" {
		saveSnapshot = make([]Preset, 0, len(s.presets))
		for _, p := range s.presets {
			saveSnapshot = append(saveSnapshot, p)
		}
	}
	s.mu.Unlock()

	if path != "" {
		savePresetsToFile(path, saveSnapshot)
	}

	if cb != nil {
		cb()
	}
	return nil
}

// RenamePreset changes a preset name without modifying its position and persists.
func (s *State) RenamePreset(token string, newName string) error {
	s.mu.Lock()
	preset, ok := s.presets[token]
	if !ok {
		s.mu.Unlock()
		return ErrPresetNotFound
	}
	preset.Name = newName
	s.presets[token] = preset
	cb := s.onPresetChange
	path := s.persistPath
	var saveSnapshot []Preset
	if path != "" {
		saveSnapshot = make([]Preset, 0, len(s.presets))
		for _, p := range s.presets {
			saveSnapshot = append(saveSnapshot, p)
		}
	}
	s.mu.Unlock()

	if path != "" {
		savePresetsToFile(path, saveSnapshot)
	}

	if cb != nil {
		cb()
	}
	return nil
}

// GetPresets returns list of preset tokens.
func (s *State) GetPresets() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	tokens := make([]string, 0, len(s.presets))
	for token := range s.presets {
		tokens = append(tokens, token)
	}
	return tokens
}

// GotoPreset moves to stored preset position using AbsoluteMove.
func (s *State) GotoPreset(token string) error {
	s.mu.RLock()
	preset, ok := s.presets[token]
	s.mu.RUnlock()

	if !ok {
		return ErrPresetNotFound
	}

	s.AbsoluteMove(preset.Position)
	return nil
}

// RemovePreset deletes a named preset and persists if enabled.
func (s *State) RemovePreset(token string) error {
	s.mu.Lock()
	if _, ok := s.presets[token]; !ok {
		s.mu.Unlock()
		return ErrPresetNotFound
	}

	delete(s.presets, token)
	cb := s.onPresetChange
	path := s.persistPath
	var saveSnapshot []Preset
	if path != "" {
		saveSnapshot = make([]Preset, 0, len(s.presets))
		for _, p := range s.presets {
			saveSnapshot = append(saveSnapshot, p)
		}
	}
	s.mu.Unlock()

	if path != "" {
		savePresetsToFile(path, saveSnapshot)
	}

	if cb != nil {
		cb()
	}
	return nil
}

// GetPresetPosition returns the position for a preset token, or error if not found.
func (s *State) GetPresetPosition(token string) (Position, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	preset, ok := s.presets[token]
	if !ok {
		return Position{}, ErrPresetNotFound
	}
	return preset.Position, nil
}

// ListPresets returns the full preset list (Name + Position).
// Unlike GetPresets which returns only tokens for ONVIF compat,
// this returns complete preset information for the web UI.
func (s *State) ListPresets() []Preset {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make([]Preset, 0, len(s.presets))
	for token, p := range s.presets {
		p.Token = token
		out = append(out, p)
	}
	return out
}

// SetOnPositionChange registers a callback invoked whenever the position
// changes (continuous move tick, absolute move step, relative move, preset goto).
// The callback is called outside the mutex lock to avoid deadlock.
func (s *State) SetOnPositionChange(fn func(pos Position)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPosChange = fn
}

// SetOnPresetListChange registers a callback invoked when a preset is added
// or removed. The callback is called outside the mutex lock.
func (s *State) SetOnPresetListChange(fn func()) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.onPresetChange = fn
}

// presetFileEntry is the JSON file format for a single saved preset.
type presetFileEntry struct {
	Token string  `json:"token"`
	Name  string  `json:"name"`
	Pan   float64 `json:"pan"`
	Tilt  float64 `json:"tilt"`
	Zoom  float64 `json:"zoom"`
}

// EnablePersistence enables file-based preset persistence.
// If the file exists, presets are loaded from it.
func (s *State) EnablePersistence(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.persistPath = path

	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("ptz: load presets: %w", err)
	}

	var entries []presetFileEntry
	if err := json.Unmarshal(data, &entries); err != nil {
		return fmt.Errorf("ptz: parse presets: %w", err)
	}

	s.presets = make(map[string]Preset, len(entries))
	for _, e := range entries {
		s.presets[e.Token] = Preset{
			Token: e.Token,
			Name:  e.Name,
			Position: Position{
				Pan:  e.Pan,
				Tilt: e.Tilt,
				Zoom: e.Zoom,
			},
		}
	}
	return nil
}

// savePresetsToFile writes presets atomically to path (JSON).
func savePresetsToFile(path string, presets []Preset) {
	entries := make([]presetFileEntry, 0, len(presets))
	for _, p := range presets {
		entries = append(entries, presetFileEntry{
			Token: p.Token,
			Name:  p.Name,
			Pan:   p.Position.Pan,
			Tilt:  p.Position.Tilt,
			Zoom:  p.Position.Zoom,
		})
	}

	data, err := json.Marshal(entries)
	if err != nil {
		return
	}

	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return
	}
	os.Rename(tmpPath, path)
}
