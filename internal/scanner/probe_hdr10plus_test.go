package scanner

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// hdr10PlusStreamsJSON is a stream-level probe of an HDR10+ HEVC file: like
// real FFmpeg output, the video stream carries no HDR10+ side data. Stream 0
// is the audio track so the frame probe must select the video by its index.
const hdr10PlusStreamsJSON = `{"streams":[` +
	`{"index":0,"codec_name":"eac3","codec_type":"audio","channels":6,"disposition":{"default":1}},` +
	`{"index":1,"codec_name":"hevc","codec_type":"video","profile":"Main 10","width":3840,"height":1600,` +
	`"color_range":"tv","color_transfer":"smpte2084","color_primaries":"bt2020","pix_fmt":"yuv420p10le",` +
	`"disposition":{"default":1}}],` +
	`"format":{"filename":"movie.mkv","format_name":"matroska,webm","duration":"5400.0"}}`

// hdr10PlusFrameJSON is FFmpeg 8.1.3's answer to the scanner's frame probe
// for the first frame of the same file.
const hdr10PlusFrameJSON = `{"frames":[{"media_type":"video","side_data_list":[` +
	`{"side_data_type":"Mastering display metadata"},` +
	`{"side_data_type":"Content light level metadata"},` +
	`{"side_data_type":"HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}]}]}`

// writeHDR10PlusFFprobe writes a fake ffprobe that logs each invocation's
// arguments to the returned log file, answers the stream probe with
// streamsJSON, and runs frameScript for the frame probe.
func writeHDR10PlusFFprobe(t *testing.T, streamsJSON, frameScript string) (ffprobe, argsLog string) {
	t.Helper()
	dir := t.TempDir()
	ffprobe = filepath.Join(dir, "ffprobe")
	argsLog = filepath.Join(dir, "args.log")
	writeFakeTool(t, ffprobe, "#!/bin/sh\n"+
		"echo \"$*\" >> '"+argsLog+"'\n"+
		"case \" $* \" in\n"+
		"*\" -show_frames \"*)\n"+frameScript+"\n;;\n"+
		"*) printf '%s' '"+streamsJSON+"' ;;\n"+
		"esac\n")
	return ffprobe, argsLog
}

func frameProbeInvocations(t *testing.T, argsLog string) []string {
	t.Helper()
	data, err := os.ReadFile(argsLog)
	if err != nil {
		t.Fatal(err)
	}
	var frames []string
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if strings.Contains(line, "-show_frames") {
			frames = append(frames, line)
		}
	}
	return frames
}

func TestProbeFileDetectsHDR10PlusFromFirstFrame(t *testing.T) {
	ffprobe, argsLog := writeHDR10PlusFFprobe(t, hdr10PlusStreamsJSON, "printf '%s' '"+hdr10PlusFrameJSON+"'")

	probe, err := ProbeFile(context.Background(), ffprobe, filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatalf("ProbeFile: %v", err)
	}
	if len(probe.VideoTracks) != 1 {
		t.Fatalf("video tracks = %d, want 1", len(probe.VideoTracks))
	}
	track := probe.VideoTracks[0]
	if !track.HDR10Plus || track.VideoRangeType != "HDR10Plus" {
		t.Fatalf("HDR10Plus = %v, VideoRangeType = %q; want true, HDR10Plus", track.HDR10Plus, track.VideoRangeType)
	}
	frames := frameProbeInvocations(t, argsLog)
	if len(frames) != 1 {
		t.Fatalf("frame probes = %d, want 1: %q", len(frames), frames)
	}
	if !strings.Contains(frames[0], "-select_streams 1 ") || !strings.Contains(frames[0], "-read_intervals %+#1 ") {
		t.Fatalf("frame probe args = %q, want the first frame of stream 1", frames[0])
	}
}

func TestProbeFileSkipsFrameProbeForNonPQVideo(t *testing.T) {
	sdr := strings.Replace(hdr10PlusStreamsJSON, `"smpte2084"`, `"bt709"`, 1)
	ffprobe, argsLog := writeHDR10PlusFFprobe(t, sdr, "printf '%s' '"+hdr10PlusFrameJSON+"'")

	probe, err := ProbeFile(context.Background(), ffprobe, filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatalf("ProbeFile: %v", err)
	}
	if probe.VideoTracks[0].HDR10Plus {
		t.Fatal("SDR track was reported as HDR10+")
	}
	if frames := frameProbeInvocations(t, argsLog); len(frames) != 0 {
		t.Fatalf("frame probes = %q, want none for an SDR file", frames)
	}
}

func TestProbeFileKeepsStreamFactsWhenFrameProbeFails(t *testing.T) {
	ffprobe, _ := writeHDR10PlusFFprobe(t, hdr10PlusStreamsJSON, "echo 'Invalid data found when processing input' >&2; exit 1")

	probe, err := ProbeFile(context.Background(), ffprobe, filepath.Join(t.TempDir(), "movie.mkv"))
	if err != nil {
		t.Fatalf("ProbeFile: %v", err)
	}
	track := probe.VideoTracks[0]
	if track.HDR10Plus || track.VideoRangeType != "HDR10" || probe.CodecVideo != "hevc" {
		t.Fatalf("track = %+v, codec = %q; want the stream-level HDR10 facts", track, probe.CodecVideo)
	}
}

func TestProbeFileReturnsContextErrorWhenCanceledDuringFrameProbe(t *testing.T) {
	ffprobe, _ := writeHDR10PlusFFprobe(t, hdr10PlusStreamsJSON, "exec sleep 30")
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	_, err := ProbeFile(ctx, ffprobe, filepath.Join(t.TempDir(), "movie.mkv"))
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("ProbeFile error = %v, want context.DeadlineExceeded so an unchecked frame is not stored", err)
	}
}
