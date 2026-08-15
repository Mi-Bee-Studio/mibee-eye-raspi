// Package gb28181 implements playback/download streaming of recorded
// segments for GB/T 28181 INVITE sessions (s=Playback / s=Download).
package gb28181

import (
	"context"
	"log/slog"
	"math"
	"net"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Mi-Bee-Studio/mibee-eye-raspi/internal/recording"
)

// PlaybackIndex extends RecordingIndex with the storage root needed to
// open segment files for playback. The concrete recording index satisfies
// it; RecordInfo-only fakes do not, and playback INVITEs against them are
// rejected with 488.
type PlaybackIndex interface {
	RecordingIndex
	Root() string
}

// parseSDPTimeRange extracts the t= line from an SDP body as a unix
// millisecond range. A missing or malformed t= line, or "t=0 0", means
// "all" and returns (0, math.MaxInt64).
func parseSDPTimeRange(body string) (startMs, endMs int64) {
	for _, line := range strings.Split(body, "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "t=") {
			continue
		}
		fields := strings.Fields(strings.TrimPrefix(line, "t="))
		if len(fields) < 2 {
			return 0, math.MaxInt64
		}
		start, err1 := strconv.ParseInt(fields[0], 10, 64)
		end, err2 := strconv.ParseInt(fields[1], 10, 64)
		if err1 != nil || err2 != nil {
			return 0, math.MaxInt64
		}
		if start == 0 && end == 0 {
			return 0, math.MaxInt64
		}
		return start * 1000, end * 1000
	}
	return 0, math.MaxInt64
}

// runPlayback streams recorded segments to the platform for a
// Playback/Download session. Playback paces frames to real time using the
// recorded PTS offsets relative to the first sent frame; Download sends as
// fast as possible. It stops when mediaCtx is cancelled (BYE or a replacing
// INVITE) or when the requested range is exhausted.
func (s *Server) runPlayback(mediaCtx context.Context, mediaConn *net.UDPConn, mediaTCPConn *net.TCPConn, rtpDest *net.UDPAddr, ssrc uint32, segments []recording.SegmentInfo, root string, startMs, endMs int64, sessionType string) {
	pusher := NewRtpPusher(mediaConn, rtpDest)
	if mediaTCPConn != nil {
		pusher.SetTCPConn(mediaTCPConn)
	}

	// The index returns segments in append order; sort by start time so
	// playback follows the recording chronology.
	sort.Slice(segments, func(i, j int) bool { return segments[i].StartMS < segments[j].StartMS })

	var baseWall time.Time
	var basePts int64 // unix ms of the first sent AU
	started := false

	for _, seg := range segments {
		reader, err := recording.OpenSegment(filepath.Join(root, seg.File))
		if err != nil {
			slog.Warn("gb28181: playback open segment failed", "file", seg.File, "error", err)
			continue
		}
		for {
			select {
			case <-mediaCtx.Done():
				slog.Info("gb28181: playback media goroutine stopped")
				return
			default:
			}
			nalus, pts, key, err := reader.Next()
			if err != nil {
				break // segment exhausted
			}
			auAbs := seg.StartMS + pts.Milliseconds()
			if auAbs < startMs {
				continue // skip to the requested start
			}
			if auAbs > endMs {
				return // past the requested end
			}
			if !started {
				if !key {
					// The decoder needs an IDR first — fast-forward to the
					// next keyframe within the requested range.
					for {
						nalus, pts, key, err = reader.Next()
						if err != nil {
							break
						}
						auAbs = seg.StartMS + pts.Milliseconds()
						if auAbs > endMs {
							return
						}
						if key {
							break
						}
					}
					if err != nil {
						break
					}
				}
				baseWall = time.Now()
				basePts = auAbs
				started = true
			}
			if sessionType == "Playback" {
				// Pace to real time: each AU goes out at baseWall +
				// (auAbs - basePts).
				wait := baseWall.Add(time.Duration(auAbs-basePts) * time.Millisecond)
				if d := time.Until(wait); d > 0 {
					select {
					case <-mediaCtx.Done():
						slog.Info("gb28181: playback media goroutine stopped")
						return
					case <-time.After(d):
					}
				}
			} else {
				// Download: no pacing; yield so other goroutines aren't starved.
				runtime.Gosched()
			}
			auTime := time.UnixMilli(auAbs)
			psData := MuxH264ToPS(nalus, key, auTime, auTime)
			if err := pusher.SendFrame(psData, key, auTime, ssrc); err != nil {
				slog.Warn("gb28181: playback send failed", "error", err)
				return
			}
		}
	}
	slog.Info("gb28181: playback stream complete", "session", sessionType, "segments", len(segments))
}
