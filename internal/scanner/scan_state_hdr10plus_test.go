package scanner

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Silo-Server/silo-server/internal/models"
)

func completeHDRScanState(hdr10PlusProbeCurrent *bool) *scanStateFile {
	probedAt := time.Now().UTC()
	dvCurrent := true
	return &scanStateFile{
		FileSize:               40_000_000_000,
		CodecVideo:             "hevc",
		CodecAudio:             "eac3",
		Resolution:             "2160p",
		Container:              "matroska",
		Duration:               7200,
		ProbeSource:            "local",
		ProbeUpdatedAt:         &probedAt,
		HasVideoTracks:         true,
		HasNonImageVideoTracks: true,
		HasAudioTracks:         true,
		HasChapters:            true,
		DVProvenanceCurrent:    &dvCurrent,
		HDR10PlusProbeCurrent:  hdr10PlusProbeCurrent,
	}
}

func TestScanRepairsPQProbeWithoutHDR10PlusCheck(t *testing.T) {
	current, legacy := true, false
	if needsCriticalProbeRepairScanState(completeHDRScanState(&current)) {
		t.Fatal("a PQ probe that carries the hdr10_plus key was sent for repair")
	}
	if needsCriticalProbeRepairScanState(completeHDRScanState(nil)) {
		t.Fatal("a scan state built in memory was sent for repair")
	}
	if !needsCriticalProbeRepairScanState(completeHDRScanState(&legacy)) {
		t.Fatal("a PQ probe written before frame-level HDR10+ detection was not repaired")
	}
}

func TestPlaybackDoesNotRepairPQProbeWithoutHDR10PlusCheck(t *testing.T) {
	probedAt := time.Now()
	file := models.MediaFile{
		ProbeSource: "local", ProbeUpdatedAt: &probedAt, Duration: 7200, Container: "matroska",
		CodecVideo: "hevc", Resolution: "2160p", Chapters: []models.MediaChapter{},
		VideoTracks: []models.VideoTrack{{Codec: "hevc", ColorRange: "tv", ColorTransfer: "smpte2084"}},
	}
	if NeedsCriticalProbeRepair(&file) {
		t.Fatal("playback would reprobe a PQ file only to check HDR10+, which plays as HDR10 unchecked")
	}
}

// TestScanStateFlagsPQTracksWithoutHDR10PlusKeyDB checks the scan-state query
// against stored JSON: only a PQ track written without the hdr10_plus key
// predates frame-level HDR10+ detection.
func TestScanStateFlagsPQTracksWithoutHDR10PlusKeyDB(t *testing.T) {
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		t.Fatalf("connect test database: %v", err)
	}
	t.Cleanup(pool.Close)

	suffix := time.Now().UnixNano()
	var folderID int
	if err := pool.QueryRow(ctx, `INSERT INTO media_folders (type, name, enabled) VALUES ('movies', $1, true) RETURNING id`,
		fmt.Sprintf("hdr10plus-probe-%d", suffix)).Scan(&folderID); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM media_files WHERE media_folder_id = $1`, folderID)
		_, _ = pool.Exec(ctx, `DELETE FROM media_folders WHERE id = $1`, folderID)
	})

	repo := NewFileRepository(pool)
	cases := []struct {
		name        string
		legacyJSON  string
		wantCurrent bool
	}{
		{name: "legacy PQ track", legacyJSON: `[{"codec":"hevc","color_range":"tv","color_transfer":"smpte2084","video_range_type":"HDR10"}]`, wantCurrent: false},
		{name: "legacy SDR track", legacyJSON: `[{"codec":"h264","color_range":"tv","color_transfer":"bt709","video_range_type":"SDR"}]`, wantCurrent: true},
		{name: "current PQ track", wantCurrent: true},
	}
	paths := make(map[string]bool, len(cases))
	for i, tc := range cases {
		path := fmt.Sprintf("/tmp/hdr10plus-probe-%d-%d.mkv", suffix, i)
		paths[path] = tc.wantCurrent
		file, err := repo.Upsert(ctx, models.MediaFile{
			MediaFolderID: folderID,
			FilePath:      path,
			FileSize:      1_000,
			VideoTracks:   []models.VideoTrack{{Codec: "hevc", ColorRange: "tv", ColorTransfer: "smpte2084", VideoRangeType: "HDR10"}},
		})
		if err != nil {
			t.Fatalf("seed %s: %v", tc.name, err)
		}
		if tc.legacyJSON != "" {
			if _, err := pool.Exec(ctx, `UPDATE media_files SET video_tracks = $1::jsonb WHERE id = $2`, tc.legacyJSON, file.ID); err != nil {
				t.Fatalf("store legacy JSON for %s: %v", tc.name, err)
			}
		}
	}

	states, err := repo.GetScanStateByFolder(ctx, folderID)
	if err != nil {
		t.Fatalf("GetScanStateByFolder: %v", err)
	}
	if len(states) != len(cases) {
		t.Fatalf("scan state rows = %d, want %d", len(states), len(cases))
	}
	for _, state := range states {
		want := paths[state.FilePath]
		if state.HDR10PlusProbeCurrent == nil || *state.HDR10PlusProbeCurrent != want {
			t.Errorf("%s: HDR10PlusProbeCurrent = %v, want %v", state.FilePath, state.HDR10PlusProbeCurrent, want)
		}
	}
}
