package playback

import (
	"encoding/json"
	"fmt"
	"os"
	"testing"

	"github.com/Silo-Server/silo-server/internal/models"
)

// appleDVFixtureRow mirrors the media_files columns the planner reads, with the
// JSONB track columns decoded exactly as the repository decodes them.
type appleDVFixtureRow struct {
	ID                int                       `json:"id"`
	ContentID         string                    `json:"content_id"`
	Container         string                    `json:"container"`
	CodecVideo        string                    `json:"codec_video"`
	CodecAudio        string                    `json:"codec_audio"`
	Resolution        string                    `json:"resolution"`
	HDR               bool                      `json:"hdr"`
	Bitrate           int                       `json:"bitrate"`
	Duration          int                       `json:"duration"`
	VideoTracks       []models.VideoTrack       `json:"video_tracks"`
	AudioTracks       []models.AudioTrack       `json:"audio_tracks"`
	SubtitleTracks    []models.SubtitleTrack    `json:"subtitle_tracks"`
	ExternalSubtitles []models.ExternalSubtitle `json:"external_subtitles"`
}

func (r appleDVFixtureRow) mediaFile() *models.MediaFile {
	return &models.MediaFile{
		ID: r.ID, ContentID: r.ContentID, Container: r.Container,
		CodecVideo: r.CodecVideo, CodecAudio: r.CodecAudio, Resolution: r.Resolution,
		HDR: r.HDR, Bitrate: r.Bitrate, Duration: r.Duration,
		FilePath:          fmt.Sprintf("/media/movies/file-%d.%s", r.ID, r.Container),
		VideoTracks:       r.VideoTracks,
		AudioTracks:       r.AudioTracks,
		SubtitleTracks:    r.SubtitleTracks,
		ExternalSubtitles: r.ExternalSubtitles,
	}
}

func loadAppleDVFixturesV3(t *testing.T) (StartRequestV3, map[int]*models.MediaFile) {
	t.Helper()
	raw, err := os.ReadFile("testdata/apple_tvos_dv_start_request.json")
	if err != nil {
		t.Fatalf("read start request fixture: %v", err)
	}
	var request StartRequestV3
	if err := json.Unmarshal(raw, &request); err != nil {
		t.Fatalf("decode start request fixture: %v", err)
	}
	rawRows, err := os.ReadFile("testdata/apple_tvos_dv_media_files.json")
	if err != nil {
		t.Fatalf("read media file fixture: %v", err)
	}
	var rows []appleDVFixtureRow
	if err := json.Unmarshal(rawRows, &rows); err != nil {
		t.Fatalf("decode media file fixture: %v", err)
	}
	files := make(map[int]*models.MediaFile, len(rows))
	for _, row := range rows {
		files[row.ID] = row.mediaFile()
	}
	return request, files
}

// mirrorShippingAppleClientV3 upgrades the captured probe request to everything
// the shipping tvOS client sends (silo-apple ApplePlaybackV3Capabilities.swift):
// the full client_features list, the per-delivery executor features, and the
// eight-channel ceiling. The captured probe omitted them, and a route gate must
// be proven against what the real client advertises.
func mirrorShippingAppleClientV3(request *StartRequestV3, originalHTTPFeatures []string) {
	request.ClientFeatures = []string{
		FeaturePlaybackPlanV3,
		FeatureClientVideoTransforms,
		FeatureRouteDiagnostics,
		FeatureDeviceQuirksV3,
		FeatureSeekReanchorV3,
		FeatureDirectStreamResumeV3,
	}
	features := map[string][]string{
		DeliveryClassOriginalHTTPV3: originalHTTPFeatures,
		DeliveryClassProgressiveV3:  {"apple_avplayer_progressive"},
		DeliveryClassHLSV3:          {"apple_avplayer_hls"},
	}
	// Rebuild the map so each case owns its deliveries: the fixture request is
	// copied by value and would otherwise share one map across subtests.
	deliveries := make(map[string]DeliveryCapabilityV3, len(request.ClientPlaybackContext.Deliveries))
	for class, capability := range request.ClientPlaybackContext.Deliveries {
		capability.Features = features[class]
		channels := 8
		capability.MaxChannels = &channels
		deliveries[class] = capability
	}
	request.ClientPlaybackContext.Deliveries = deliveries
}

func appleDVPlannerInputV3(request StartRequestV3, file *models.MediaFile, audioIndex int) PlannerInputV3 {
	request.FileID = file.ID
	return PlannerInputV3{
		Request: request, RequestedFile: file, EffectiveFile: file, AudioTrackIndex: audioIndex,
		Settings: PlannerSettingsV3{TranscodeEnabled: true},
		Registry: testTransformationRegistryV3(),
	}
}

// TestPlanPlaybackV3AppleOriginalHTTPHonorsClientAudioTrackSelection replays the
// three real dev media rows that exposed the defect: an Apple TV that advertises
// Dolby Vision was handed server_remux_progressive for the two DV titles and
// original_http for the HDR10 title. Dolby Vision was coincidental — the DV rows
// carry a non-English container default (zh, de) while the HDR10 row's only
// alternative is Spanish, so the profile's English audio preference resolved to
// a non-default track only for the DV rows.
func TestPlanPlaybackV3AppleOriginalHTTPHonorsClientAudioTrackSelection(t *testing.T) {
	baseRequest, files := loadAppleDVFixturesV3(t)

	tests := []struct {
		name string
		// audioIndex is what the handler's preferredAudioTrackIndexV3 resolves
		// for this profile: the English track, which is the container default
		// only for the HDR10 row (it has no English track at all).
		fileID     int
		audioIndex int
		wantDV     bool
	}{
		{name: "dolby vision profile 5", fileID: 741, audioIndex: 1, wantDV: true},
		{name: "dolby vision profile 8", fileID: 223792, audioIndex: 1, wantDV: true},
		{name: "hdr10 container default", fileID: 223805, audioIndex: 0},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			file := files[test.fileID]
			if file == nil {
				t.Fatalf("fixture row %d missing", test.fileID)
			}
			request := baseRequest
			mirrorShippingAppleClientV3(&request, []string{
				"apple_native_direct", "apple_local_loopback", "apple_playercore",
				ClientAudioTrackSelectionV3,
			})
			result := PlanPlaybackV3(appleDVPlannerInputV3(request, file, test.audioIndex))
			if result.Plan == nil || result.Plan.Delivery != DeliveryOriginalHTTPV3 ||
				result.Plan.DecisionReason != "validated_original_playback" || result.PlayMethod != PlayDirect {
				t.Fatalf("result = %s", ExplainPlannerResultV3(result))
			}
			if result.Plan.Stream.Container != "mkv" {
				t.Fatalf("stream container = %q, want mkv", result.Plan.Stream.Container)
			}
			// The route only holds because the plan tells the client which
			// embedded audio track to select from the untouched container.
			if result.Plan.SelectedTracks.Audio == nil || result.Plan.SelectedTracks.Audio.Index == nil ||
				*result.Plan.SelectedTracks.Audio.Index != test.audioIndex {
				t.Fatalf("selected audio = %#v, want index %d", result.Plan.SelectedTracks.Audio, test.audioIndex)
			}
			if result.Plan.Claims.Video.DolbyVision != test.wantDV {
				t.Fatalf("dolby vision claim = %#v, want %v", result.Plan.Claims.Video, test.wantDV)
			}
		})
	}
}

// TestPlanPlaybackV3OriginalHTTPKeepsRemuxWithoutAudioSelectionFeature pins the
// other half of the gate: a client that does not advertise the selection
// capability still gets the remux that maps its chosen track explicitly.
func TestPlanPlaybackV3OriginalHTTPKeepsRemuxWithoutAudioSelectionFeature(t *testing.T) {
	baseRequest, files := loadAppleDVFixturesV3(t)
	file := files[741]
	if file == nil {
		t.Fatal("fixture row 741 missing")
	}
	request := baseRequest
	mirrorShippingAppleClientV3(&request, []string{"apple_native_direct", "apple_local_loopback", "apple_playercore"})
	result := PlanPlaybackV3(appleDVPlannerInputV3(request, file, 1))
	if result.Plan == nil || result.Plan.Delivery != DeliveryRemuxProgressiveV3 {
		t.Fatalf("result = %s", ExplainPlannerResultV3(result))
	}
}

func TestPlanPlaybackV3Profile7OriginalHTTPHonorsClientAudioTrackSelection(t *testing.T) {
	file := detailedFixtureFileV3()
	file.VideoTracks[0].DVProfile = 7
	file.VideoTracks[0].DVBLCompatID = 1
	file.VideoTracks[0].DVELPresent = true
	file.VideoTracks[0].DVEnhancementLayer = "unknown"
	file.VideoTracks[0].VideoRange = "DolbyVision"
	file.VideoTracks[0].VideoRangeType = "DOVIWithEL"
	file.AudioTracks = []models.AudioTrack{
		{Codec: "aac", Channels: 2, Layout: "stereo", Default: true},
		{Codec: "aac", Channels: 2, Layout: "stereo"},
	}

	request := validStartRequestV3()
	request.ClientFeatures = append(request.ClientFeatures, FeatureClientVideoTransforms)
	request.Capabilities.VideoDecode = []VideoDecodeCapabilityV3{{Codec: "hevc", Profiles: []string{"main 10"}, Levels: []int{153}, BitDepths: []int{10}, MaxWidth: 3840, MaxHeight: 2160, MaxFrameRate: 60, MaxBitrateKbps: 80_000, Hardware: true}}
	request.Capabilities.HDRDetails = &HDRCapabilitiesV3{HDR10: true, DolbyVisionProfiles: []int{8}}
	request.ClientPlaybackContext.Output.HDRDetails = request.Capabilities.HDRDetails
	direct := request.ClientPlaybackContext.Deliveries[DeliveryClassOriginalHTTPV3]
	direct.Features = append(direct.Features, ClientAudioTrackSelectionV3)
	direct.Transformations = []TransformationV3{
		{Name: ClientDV7ToDV81V3, Executor: ExecutorClientV3, RecipeVersion: ClientDVTransformVersionV3},
		{Name: ClientDV7ToHDR10V3, Executor: ExecutorClientV3, RecipeVersion: ClientDVTransformVersionV3},
	}
	request.ClientPlaybackContext.Deliveries[DeliveryClassOriginalHTTPV3] = direct

	input := PlannerInputV3{
		Request: request, RequestedFile: file, EffectiveFile: file, AudioTrackIndex: 1,
		Settings: PlannerSettingsV3{TranscodeEnabled: true}, Registry: testTransformationRegistryV3(),
	}
	first := PlanPlaybackV3(input)
	if first.Plan == nil || first.Plan.Delivery != DeliveryOriginalHTTPV3 || first.Plan.DecisionReason != "client_dv7_to_dv81" {
		t.Fatalf("first = %s", ExplainPlannerResultV3(first))
	}
	if first.Plan.SelectedTracks.Audio == nil || first.Plan.SelectedTracks.Audio.Index == nil || *first.Plan.SelectedTracks.Audio.Index != 1 {
		t.Fatalf("first selected audio = %#v, want index 1", first.Plan.SelectedTracks.Audio)
	}

	input.AttemptedKeys = []string{first.Plan.PlanAttemptKey}
	second := PlanPlaybackV3(input)
	if second.Plan == nil || second.Plan.Delivery != DeliveryOriginalHTTPV3 || second.Plan.DecisionReason != "client_dv7_to_hdr10" {
		t.Fatalf("second = %s", ExplainPlannerResultV3(second))
	}
	if second.Plan.SelectedTracks.Audio == nil || second.Plan.SelectedTracks.Audio.Index == nil || *second.Plan.SelectedTracks.Audio.Index != 1 {
		t.Fatalf("second selected audio = %#v, want index 1", second.Plan.SelectedTracks.Audio)
	}
}

func TestPlanPlaybackV3AudioOnlyOriginalHTTPHonorsClientAudioTrackSelection(t *testing.T) {
	file := audioOnlyFixtureFileV3()
	file.AudioTracks = []models.AudioTrack{
		{Codec: "aac", Channels: 2, Layout: "stereo", Default: true},
		{Codec: "aac", Channels: 2, Layout: "stereo"},
	}
	request := validStartRequestV3()
	request.FileID = file.ID
	request.Capabilities.Containers = []string{"mp4"}
	direct := request.ClientPlaybackContext.Deliveries[DeliveryClassOriginalHTTPV3]
	direct.Features = append(direct.Features, ClientAudioTrackSelectionV3)
	request.ClientPlaybackContext.Deliveries[DeliveryClassOriginalHTTPV3] = direct

	result := PlanPlaybackV3(PlannerInputV3{
		Request: request, RequestedFile: file, EffectiveFile: file, AudioTrackIndex: 1,
		Settings: PlannerSettingsV3{TranscodeEnabled: true}, Registry: testTransformationRegistryV3(),
	})
	if result.Plan == nil || result.Plan.Delivery != DeliveryOriginalHTTPV3 || result.PlayMethod != PlayDirect {
		t.Fatalf("result = %s", ExplainPlannerResultV3(result))
	}
	if result.Plan.SelectedTracks.Audio == nil || result.Plan.SelectedTracks.Audio.Index == nil || *result.Plan.SelectedTracks.Audio.Index != 1 {
		t.Fatalf("selected audio = %#v, want index 1", result.Plan.SelectedTracks.Audio)
	}
}
