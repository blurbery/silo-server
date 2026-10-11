package mediaprobe

import (
	"path/filepath"
	"testing"
)

func TestFFprobePathFromFFmpegRewritesOnlyExecutableName(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{name: "plain executable", in: "ffmpeg", want: "ffprobe"},
		{name: "executable suffix", in: "jellyfin-ffmpeg.exe", want: "jellyfin-ffprobe.exe"},
		{name: "case-insensitive Windows executable", in: "FFMPEG.EXE", want: "ffprobe.EXE"},
		{name: "sibling path", in: filepath.Join("opt", "bin", "ffmpeg"), want: filepath.Join("opt", "bin", "ffprobe")},
		{name: "parent name is not rewritten", in: filepath.Join("opt", "ffmpeg-suite", "bin", "custom"), want: "ffprobe"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := FFprobePathFromFFmpeg(tt.in); got != tt.want {
				t.Fatalf("FFprobePathFromFFmpeg(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// hdr10PlusFrameSideData is the side data FFmpeg 8.1.3 (jellyfin-ffmpeg)
// reports on the first decoded frame of an HDR10+ HEVC file. The stream-level
// probe of the same file carries none of it.
var hdr10PlusFrameSideData = []SideData{
	{SideDataType: "Mastering display metadata"},
	{SideDataType: "Content light level metadata"},
	{SideDataType: "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"},
}

func TestNormalizeVideoTrackReadsHDR10PlusFromFrameSideData(t *testing.T) {
	dv8 := []SideData{{SideDataType: "DOVI configuration record", DVProfile: 8, DVLevel: 6, DVBlPresent: 1, DVRPUPresent: 1, DVBLCompatID: 1, DVBLCompatIDPresent: true}}
	tests := []struct {
		name          string
		stream        Stream
		wantHDR10Plus bool
		wantRangeType string
	}{
		{
			name:          "PQ stream with HDR10+ frame metadata",
			stream:        Stream{CodecType: "video", CodecName: "hevc", ColorTransfer: "smpte2084", FrameSideDataList: hdr10PlusFrameSideData},
			wantHDR10Plus: true,
			wantRangeType: "HDR10Plus",
		},
		{
			name:          "Dolby Vision 8.1 with HDR10+ frame metadata",
			stream:        Stream{CodecType: "video", CodecName: "hevc", ColorTransfer: "smpte2084", SideDataList: dv8, FrameSideDataList: hdr10PlusFrameSideData},
			wantHDR10Plus: true,
			wantRangeType: "DOVIWithHDR10Plus",
		},
		{
			name:          "PQ stream whose frame has only static metadata",
			stream:        Stream{CodecType: "video", CodecName: "hevc", ColorTransfer: "smpte2084", FrameSideDataList: hdr10PlusFrameSideData[:2]},
			wantRangeType: "HDR10",
		},
		{
			name:          "PQ stream whose frame was not read",
			stream:        Stream{CodecType: "video", CodecName: "hevc", ColorTransfer: "smpte2084"},
			wantRangeType: "HDR10",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			track := NormalizeVideoTrack(tt.stream)
			if track.HDR10Plus != tt.wantHDR10Plus || track.VideoRangeType != tt.wantRangeType {
				t.Fatalf("HDR10Plus = %v, VideoRangeType = %q; want %v, %q", track.HDR10Plus, track.VideoRangeType, tt.wantHDR10Plus, tt.wantRangeType)
			}
		})
	}
}

func TestNeedsFrameSideDataOnlyForPrimaryPQVideo(t *testing.T) {
	tests := []struct {
		name   string
		stream Stream
		want   bool
	}{
		{name: "PQ video", stream: Stream{CodecType: "video", ColorTransfer: "smpte2084"}, want: true},
		{name: "HLG video", stream: Stream{CodecType: "video", ColorTransfer: "arib-std-b67"}},
		{name: "SDR video", stream: Stream{CodecType: "video", ColorTransfer: "bt709"}},
		{name: "PQ cover art", stream: Stream{CodecType: "video", ColorTransfer: "smpte2084", Disposition: Disposition{AttachedPic: 1}}},
		{name: "audio", stream: Stream{CodecType: "audio"}},
	}
	for _, tt := range tests {
		if got := NeedsFrameSideData(tt.stream); got != tt.want {
			t.Errorf("%s: NeedsFrameSideData() = %v, want %v", tt.name, got, tt.want)
		}
	}
}
