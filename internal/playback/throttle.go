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

	// throttleResumePercent keeps a meaningful buffer below the pause point
	// before restarting FFmpeg. Without this hysteresis, a fast codec-copy
	// producer can cross the same boundary again on the next five-second check
	// and repeatedly stop the playlist writer while a client is consuming it.
	throttleResumePercent = 75
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

	bufferedSeconds := progress.BufferedDurationSeconds
	if !progress.HasBufferedDuration {
		gapSegments := progress.ProducedHead - progress.LastRequestedSegment
		segmentDuration := progress.SegmentDuration
		if segmentDuration <= 0 {
			segmentDuration = t.segmentDuration
		}
		bufferedSeconds = float64(gapSegments * segmentDuration)
	}
	resumeThreshold := float64(t.thresholdSeconds*throttleResumePercent) / 100

	t.mu.Lock()
	defer t.mu.Unlock()

	if bufferedSeconds >= float64(t.thresholdSeconds) && !t.paused {
		log.Printf("playback: throttler pausing ffmpeg (buffered=%.1fs, pause_threshold=%ds, resume_threshold=%.1fs)", bufferedSeconds, t.thresholdSeconds, resumeThreshold)
		t.sendPause()
		t.paused = true
	} else if bufferedSeconds < resumeThreshold && t.paused {
		log.Printf("playback: throttler resuming ffmpeg (buffered=%.1fs, pause_threshold=%ds, resume_threshold=%.1fs)", bufferedSeconds, t.thresholdSeconds, resumeThreshold)
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
