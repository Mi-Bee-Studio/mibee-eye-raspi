package ptz

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestContinuousMove(t *testing.T) {
	s := NewState()

	s.ContinuousMove(Velocity{Pan: 0.5, Tilt: 0, Zoom: 0})
	time.Sleep(200 * time.Millisecond)
	s.Stop()

	pos := s.GetPosition()
	if pos.Pan <= 0 {
		t.Errorf("expected Pan > 0 after 200ms continuous move at 0.5, got %f", pos.Pan)
	}
	if pos.Tilt != 0 {
		t.Errorf("expected Tilt == 0, got %f", pos.Tilt)
	}
	if pos.Zoom != 0 {
		t.Errorf("expected Zoom == 0, got %f", pos.Zoom)
	}
}

func TestContinuousMoveStop(t *testing.T) {
	s := NewState()

	s.ContinuousMove(Velocity{Pan: 1.0, Tilt: 1.0, Zoom: 1.0})
	// Stop almost immediately
	time.Sleep(10 * time.Millisecond)
	s.Stop()

	pos := s.GetPosition()
	// After 10ms, position should have barely changed from origin
	if pos.Pan > 0.02 {
		t.Errorf("expected Pan barely changed after immediate stop, got %f", pos.Pan)
	}
	if pos.Tilt > 0.02 {
		t.Errorf("expected Tilt barely changed after immediate stop, got %f", pos.Tilt)
	}
	if pos.Zoom > 0.02 {
		t.Errorf("expected Zoom barely changed after immediate stop, got %f", pos.Zoom)
	}
}

func TestContinuousMoveClamp(t *testing.T) {
	s := NewState()

	// Move at max speed toward upper boundary
	s.ContinuousMove(Velocity{Pan: 1.0, Tilt: 1.0, Zoom: 1.0})
	time.Sleep(3 * time.Second) // enough to hit boundaries
	s.Stop()

	pos := s.GetPosition()
	if pos.Pan > 1.0 {
		t.Errorf("expected Pan <= 1.0, got %f", pos.Pan)
	}
	if pos.Tilt > 1.0 {
		t.Errorf("expected Tilt <= 1.0, got %f", pos.Tilt)
	}
	if pos.Zoom > 1.0 {
		t.Errorf("expected Zoom <= 1.0, got %f", pos.Zoom)
	}
	// 3s at 50ms tick = 60 ticks × 0.01/step = 0.6 total movement
	if pos.Pan < 0.5 {
		t.Errorf("expected Pan significantly moved after 3s, got %f", pos.Pan)
	}
}

func TestAbsoluteMove(t *testing.T) {
	s := NewState()

	// First set a known starting position via immediate move
	s.mu.Lock()
	s.position = Position{Pan: -0.5, Tilt: 0.5, Zoom: 0.3}
	s.mu.Unlock()

	s.AbsoluteMove(Position{Pan: 0.5, Tilt: -0.3, Zoom: 0.8})

	// Wait for the easing animation to complete (~1 second)
	time.Sleep(2 * time.Second)

	pos := s.GetPosition()
	if pos.Pan != 0.5 {
		t.Errorf("expected Pan 0.5, got %f", pos.Pan)
	}
	if pos.Tilt != -0.3 {
		t.Errorf("expected Tilt -0.3, got %f", pos.Tilt)
	}
	if pos.Zoom != 0.8 {
		t.Errorf("expected Zoom 0.8, got %f", pos.Zoom)
	}
}

func TestAbsoluteMoveClamp(t *testing.T) {
	s := NewState()

	s.AbsoluteMove(Position{Pan: 5.0, Tilt: 5.0, Zoom: 5.0})
	time.Sleep(2 * time.Second)

	pos := s.GetPosition()
	if pos.Pan != 1.0 {
		t.Errorf("expected Pan clamped to 1.0, got %f", pos.Pan)
	}
	if pos.Tilt != 1.0 {
		t.Errorf("expected Tilt clamped to 1.0, got %f", pos.Tilt)
	}
	if pos.Zoom != 1.0 {
		t.Errorf("expected Zoom clamped to 1.0, got %f", pos.Zoom)
	}
}

func TestAbsoluteMoveStopsPreviousMovement(t *testing.T) {
	s := NewState()

	// Start continuous move
	s.ContinuousMove(Velocity{Pan: 1.0, Tilt: 0, Zoom: 0})
	time.Sleep(100 * time.Millisecond)

	// AbsoluteMove should stop continuous movement
	s.AbsoluteMove(Position{Pan: 0, Tilt: 0, Zoom: 0})
	time.Sleep(2 * time.Second)

	pos := s.GetPosition()
	// After animation, should be at (0,0,0) — not at continuous move position
	if pos.Pan > 0.01 {
		t.Errorf("expected Pan near 0 after AbsoluteMove, got %f", pos.Pan)
	}
}

func TestRelativeMove(t *testing.T) {
	s := NewState()

	// Start at origin
	s.RelativeMove(Velocity{Pan: 0.3, Tilt: 0.2, Zoom: 0.1})

	pos := s.GetPosition()
	if pos.Pan != 0.3 {
		t.Errorf("expected Pan 0.3, got %f", pos.Pan)
	}
	if pos.Tilt != 0.2 {
		t.Errorf("expected Tilt 0.2, got %f", pos.Tilt)
	}
	if pos.Zoom != 0.1 {
		t.Errorf("expected Zoom 0.1, got %f", pos.Zoom)
	}
}

func TestRelativeMoveClamp(t *testing.T) {
	s := NewState()

	// Set position near boundary
	s.mu.Lock()
	s.position = Position{Pan: 0.9, Tilt: -0.9, Zoom: 0.9}
	s.mu.Unlock()

	// Relative move that would exceed boundary
	s.RelativeMove(Velocity{Pan: 0.5, Tilt: -0.5, Zoom: 0.5})

	pos := s.GetPosition()
	if pos.Pan != 1.0 {
		t.Errorf("expected Pan clamped to 1.0, got %f", pos.Pan)
	}
	if pos.Tilt != -1.0 {
		t.Errorf("expected Tilt clamped to -1.0, got %f", pos.Tilt)
	}
	if pos.Zoom != 1.0 {
		t.Errorf("expected Zoom clamped to 1.0, got %f", pos.Zoom)
	}
}

func TestStop(t *testing.T) {
	s := NewState()

	s.ContinuousMove(Velocity{Pan: 1.0, Tilt: 0, Zoom: 0})
	time.Sleep(50 * time.Millisecond)

	status := s.GetStatus()
	if status != "MOVING" {
		t.Errorf("expected MOVING, got %s", status)
	}

	s.Stop()
	time.Sleep(20 * time.Millisecond) // let goroutine drain

	status = s.GetStatus()
	if status != "IDLE" {
		t.Errorf("expected IDLE after Stop, got %s", status)
	}
}

func TestStopTwice(t *testing.T) {
	s := NewState()

	s.ContinuousMove(Velocity{Pan: 1.0, Tilt: 0, Zoom: 0})
	s.Stop()
	s.Stop() // second stop should not panic

	if s.GetStatus() != "IDLE" {
		t.Errorf("expected IDLE, got %s", s.GetStatus())
	}
}

func TestSetGetPreset(t *testing.T) {
	s := NewState()

	// Set position
	s.mu.Lock()
	s.position = Position{Pan: 0.5, Tilt: -0.3, Zoom: 0.8}
	s.mu.Unlock()

	err := s.SetPreset("home", "Home Position")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify preset position
	presetPos, err := s.GetPresetPosition("home")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if presetPos.Pan != 0.5 || presetPos.Tilt != -0.3 || presetPos.Zoom != 0.8 {
		t.Errorf("preset position mismatch: got %+v", presetPos)
	}

	// GetPresets should contain "home"
	presets := s.GetPresets()
	if len(presets) != 1 || presets[0] != "home" {
		t.Errorf("expected presets [home], got %v", presets)
	}
}

func TestGotoPreset(t *testing.T) {
	s := NewState()

	// Set position and store preset
	s.mu.Lock()
	s.position = Position{Pan: 0.7, Tilt: -0.5, Zoom: 0.9}
	s.mu.Unlock()
	s.SetPreset("zoom1", "Zoom Preset 1")

	// Move away
	s.mu.Lock()
	s.position = Position{Pan: 0, Tilt: 0, Zoom: 0}
	s.mu.Unlock()

	// Go to preset
	err := s.GotoPreset("zoom1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Wait for easing to complete
	time.Sleep(2 * time.Second)

	pos := s.GetPosition()
	if pos.Pan != 0.7 {
		t.Errorf("expected Pan 0.7, got %f", pos.Pan)
	}
	if pos.Tilt != -0.5 {
		t.Errorf("expected Tilt -0.5, got %f", pos.Tilt)
	}
	if pos.Zoom != 0.9 {
		t.Errorf("expected Zoom 0.9, got %f", pos.Zoom)
	}
}

func TestGotoPresetNotFound(t *testing.T) {
	s := NewState()

	err := s.GotoPreset("nonexistent")
	if err != ErrPresetNotFound {
		t.Errorf("expected ErrPresetNotFound, got %v", err)
	}
}

func TestRemovePreset(t *testing.T) {
	s := NewState()

	s.SetPreset("temp", "Temporary")
	presets := s.GetPresets()
	if len(presets) != 1 {
		t.Fatalf("expected 1 preset, got %d", len(presets))
	}

	err := s.RemovePreset("temp")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	presets = s.GetPresets()
	if len(presets) != 0 {
		t.Errorf("expected 0 presets after removal, got %v", presets)
	}
}

func TestRemovePresetNotFound(t *testing.T) {
	s := NewState()

	err := s.RemovePreset("nonexistent")
	if err != ErrPresetNotFound {
		t.Errorf("expected ErrPresetNotFound, got %v", err)
	}
}

func TestGetPresets(t *testing.T) {
	s := NewState()

	s.SetPreset("home", "Home")
	s.SetPreset("zoom1", "Zoom 1")
	s.SetPreset("wide", "Wide Angle")

	presets := s.GetPresets()
	if len(presets) != 3 {
		t.Fatalf("expected 3 presets, got %d", len(presets))
	}

	// Check all tokens are present (order not guaranteed)
	tokenMap := make(map[string]bool)
	for _, p := range presets {
		tokenMap[p] = true
	}
	for _, want := range []string{"home", "zoom1", "wide"} {
		if !tokenMap[want] {
			t.Errorf("preset %q not found in %+v", want, presets)
		}
	}
}

func TestConcurrentAccess(t *testing.T) {
	s := NewState()
	s.SetPreset("home", "Home")

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			switch id % 5 {
			case 0:
				s.ContinuousMove(Velocity{Pan: 0.5, Tilt: 0, Zoom: 0})
			case 1:
				s.Stop()
			case 2:
				s.AbsoluteMove(Position{Pan: 0.3, Tilt: -0.2, Zoom: 0.5})
			case 3:
				_ = s.GetPosition()
			case 4:
				_ = s.GetStatus()
			}
		}(i)
	}
	wg.Wait()

	// Verify state is consistent (no panics, no races)
	_ = s.GetPosition()
	_ = s.GetStatus()
}

func TestContinuousMoveReplacesPrevious(t *testing.T) {
	s := NewState()

	// Start moving right
	s.ContinuousMove(Velocity{Pan: 1.0, Tilt: 0, Zoom: 0})
	time.Sleep(100 * time.Millisecond)

	// Replace with left movement
	s.ContinuousMove(Velocity{Pan: -1.0, Tilt: 0, Zoom: 0})
	time.Sleep(100 * time.Millisecond)

	s.Stop()
	pos := s.GetPosition()

	// Should have moved right, then started moving back left
	// Net position should be closer to 0 than if we only moved right
	if pos.Pan < -0.5 {
		t.Errorf("expected Pan to have moved right then left, got %f", pos.Pan)
	}
}

func TestNegativeVelocity(t *testing.T) {
	s := NewState()

	// Set position to center-right
	s.mu.Lock()
	s.position = Position{Pan: 0.5, Tilt: 0.5, Zoom: 0.5}
	s.mu.Unlock()

	s.ContinuousMove(Velocity{Pan: -0.5, Tilt: -0.5, Zoom: -0.5})
	time.Sleep(300 * time.Millisecond)
	s.Stop()

	pos := s.GetPosition()
	if pos.Pan >= 0.5 {
		t.Errorf("expected Pan to decrease, got %f", pos.Pan)
	}
	if pos.Tilt >= 0.5 {
		t.Errorf("expected Tilt to decrease, got %f", pos.Tilt)
	}
}

func TestNewStateDefaults(t *testing.T) {
	s := NewState()

	pos := s.GetPosition()
	if pos.Pan != 0 || pos.Tilt != 0 || pos.Zoom != 0 {
		t.Errorf("expected origin position, got %+v", pos)
	}

	if s.GetStatus() != "IDLE" {
		t.Errorf("expected IDLE, got %s", s.GetStatus())
	}

	if len(s.GetPresets()) != 0 {
		t.Errorf("expected no presets, got %v", s.GetPresets())
	}
}

func TestSetPresetOverwrite(t *testing.T) {
	s := NewState()

	// Set initial preset
	s.mu.Lock()
	s.position = Position{Pan: 0.5, Tilt: 0, Zoom: 0}
	s.mu.Unlock()
	s.SetPreset("home", "Home")

	// Overwrite with new position
	s.mu.Lock()
	s.position = Position{Pan: -0.5, Tilt: 0, Zoom: 0}
	s.mu.Unlock()
	s.SetPreset("home", "Home Updated")

	presetPos, _ := s.GetPresetPosition("home")
	if presetPos.Pan != -0.5 {
		t.Errorf("expected overwritten Pan -0.5, got %f", presetPos.Pan)
	}
}

func BenchmarkContinuousMove(b *testing.B) {
	s := NewState()
	for i := 0; i < b.N; i++ {
		s.ContinuousMove(Velocity{Pan: 0.5, Tilt: 0, Zoom: 0})
		s.Stop()
	}
}

func BenchmarkGetPosition(b *testing.B) {
	s := NewState()
	for i := 0; i < b.N; i++ {
		_ = s.GetPosition()
	}
}

func TestSetPresetConcurrency(t *testing.T) {
	s := NewState()
	var wg sync.WaitGroup

	// Concurrent writes
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			_ = s.SetPreset(fmt.Sprintf("preset-%d", id), fmt.Sprintf("Preset %d", id))
		}(i)
	}

	// Concurrent reads
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = s.GetPresets()
		}()
	}

	wg.Wait()

	if len(s.GetPresets()) != 100 {
		t.Errorf("expected 100 presets, got %d", len(s.GetPresets()))
	}
}

func TestPresetPersistence(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "presets.json")

	s := NewState()
	if err := s.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence: %v", err)
	}

	// Set presets
	s.mu.Lock()
	s.position = Position{Pan: 0.5, Tilt: -0.3, Zoom: 0.8}
	s.mu.Unlock()
	s.SetPreset("home", "Home Position")

	s.mu.Lock()
	s.position = Position{Pan: -0.5, Tilt: 0.3, Zoom: 0.2}
	s.mu.Unlock()
	s.SetPreset("wide", "Wide Angle")

	// Create new state and load from file
	s2 := NewState()
	if err := s2.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence (2): %v", err)
	}

	presets := s2.GetPresets()
	if len(presets) != 2 {
		t.Fatalf("expected 2 presets, got %d", len(presets))
	}

	homePos, err := s2.GetPresetPosition("home")
	if err != nil {
		t.Fatalf("GetPresetPosition(home): %v", err)
	}
	if homePos.Pan != 0.5 || homePos.Tilt != -0.3 || homePos.Zoom != 0.8 {
		t.Errorf("home position mismatch: got %+v", homePos)
	}

	widePos, err := s2.GetPresetPosition("wide")
	if err != nil {
		t.Fatalf("GetPresetPosition(wide): %v", err)
	}
	if widePos.Pan != -0.5 || widePos.Tilt != 0.3 || widePos.Zoom != 0.2 {
		t.Errorf("wide position mismatch: got %+v", widePos)
	}

	// Test remove also persists
	s2.RemovePreset("home")
	presets = s2.GetPresets()
	if len(presets) != 1 {
		t.Errorf("expected 1 preset after removal, got %d", len(presets))
	}

	// Load again to verify removal persisted
	s3 := NewState()
	if err := s3.EnablePersistence(path); err != nil {
		t.Fatalf("EnablePersistence (3): %v", err)
	}
	presets = s3.GetPresets()
	if len(presets) != 1 {
		t.Errorf("expected 1 preset on re-load, got %d", len(presets))
	}

	// Verify file content on disk
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if len(data) == 0 {
		t.Error("persistence file is empty")
	}
}

func TestPresetPersistenceNoFile(t *testing.T) {
	// EnablePersistence with non-existent file should not error
	dir := t.TempDir()
	path := filepath.Join(dir, "nonexistent.json")

	s := NewState()
	if err := s.EnablePersistence(path); err != nil {
		t.Errorf("expected no error for missing file, got %v", err)
	}

	// Setting a preset should create the file
	s.SetPreset("test", "Test")
	if _, err := os.Stat(path); os.IsNotExist(err) {
		t.Error("expected persistence file to be created after SetPreset")
	}
}
