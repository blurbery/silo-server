// internal/playback/throttle.go
package playback

import (
	"io"
	"log"
	"sync"
	"time"
)

const (
	// throttleCheckInterval is how often the throttler checks the gap.
	throttleCheckInterval = 5 * time.Second

	// minThresholdSeconds is the minimum allowed throttle threshold.
	minThresholdSeconds = 60
)

// TranscodeThrottler pauses and resumes an FFmpeg process by sending
// interactive commands to its stdin. It monitors the gap
// between the transcode position (segments produced) and the client's
// download position (highest segment fetched).
type TranscodeThrottler struct {
	session          *TranscodeSession
	stdinPipe        io.WriteCloser
	thresholdSeconds int
	segmentDuration  int
	paused           bool
	stopCh           chan struct{}
	mu               sync.Mutex
}

// NewTranscodeThrottler creates a throttler. thresholdSeconds is clamped
// to a minimum of 60.
func NewTranscodeThrottler(session *TranscodeSession, stdinPipe io.WriteCloser, thresholdSeconds, segmentDuration int) *TranscodeThrottler {
	if thresholdSeconds < minThresholdSeconds {
		thresholdSeconds = minThresholdSeconds
	}
	return &TranscodeThrottler{
		session:          session,
		stdinPipe:        stdinPipe,
		thresholdSeconds: thresholdSeconds,
		segmentDuration:  segmentDuration,
		stopCh:           make(chan struct{}),
	}
}

// Start launches the background check goroutine.
func (t *TranscodeThrottler) Start() {
	go t.run()
}

// Stop signals the check goroutine to exit. If FFmpeg is currently paused,
// it sends a resume command before stopping.
func (t *TranscodeThrottler) Stop() {
	t.mu.Lock()
	defer t.mu.Unlock()

	select {
	case <-t.stopCh:
		return // already stopped
	default:
	}

	if t.paused {
		t.sendResume()
	}
	close(t.stopCh)
}

func (t *TranscodeThrottler) run() {
	ticker := time.NewTicker(throttleCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-t.stopCh:
			return
		case <-ticker.C:
			if !t.session.IsRunning() {
				return
			}
			t.CheckOnce()
		}
	}
}

// CheckOnce performs a single throttle check. Exported for testing.
func (t *TranscodeThrottler) CheckOnce() {
	progress := t.session.SegmentProgress(time.Now())
	if progress.ProducedHead < progress.StartSegmentNumber {
		return
	}

	gapSegments := progress.ProducedHead - progress.LastRequestedSegment
	segmentDuration := progress.SegmentDuration
	if segmentDuration <= 0 {
		segmentDuration = t.segmentDuration
	}
	gap := gapSegments * segmentDuration

	t.mu.Lock()
	defer t.mu.Unlock()

	if gap >= t.thresholdSeconds && !t.paused {
		log.Printf("playback: throttler pausing ffmpeg (gap=%ds, threshold=%ds)", gap, t.thresholdSeconds)
		t.sendPause()
		t.paused = true
	} else if gap < t.thresholdSeconds && t.paused {
		log.Printf("playback: throttler resuming ffmpeg (gap=%ds, threshold=%ds)", gap, t.thresholdSeconds)
		t.sendResume()
		t.paused = false
	}
}

func (t *TranscodeThrottler) sendPause() {
	t.sendCommand("p")
}

func (t *TranscodeThrottler) sendResume() {
	t.sendCommand("u")
}

// sendCommand writes an interactive FFmpeg command to stdin. Errors are logged
// but not returned, which handles dead pipes from externally killed FFmpeg.
func (t *TranscodeThrottler) sendCommand(command string) {
	if _, err := t.stdinPipe.Write([]byte(command)); err != nil {
		log.Printf("playback: throttler stdin write error (ffmpeg may have exited): %v", err)
	}
}
