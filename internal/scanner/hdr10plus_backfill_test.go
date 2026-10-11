package scanner

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Silo-Server/silo-server/internal/models"
)

func TestPlaybackDoesNotRepairPQProbeWithoutHDR10PlusCheck(t *testing.T) {
	probedAt := time.Now()
	file := models.MediaFile{
		ProbeSource: "local", ProbeUpdatedAt: &probedAt, Duration: 7200, Container: "matroska",
		CodecVideo: "hevc", Resolution: "2160p", Chapters: []models.MediaChapter{},
		VideoTracks: []models.VideoTrack{{Codec: "hevc", ColorRange: "tv", ColorTransfer: "smpte2084", HDR10PlusUnchecked: true}},
	}
	if NeedsCriticalProbeRepair(&file) {
		t.Fatal("playback would reprobe a PQ file only to check HDR10+, which plays as HDR10 unchecked")
	}
}

func TestVideoTrackJSONKeepsHDR10PlusCheckState(t *testing.T) {
	checked, err := json.Marshal(models.VideoTrack{Codec: "hevc", ColorTransfer: "smpte2084"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(checked), `"hdr10_plus":false`) {
		t.Fatalf("checked track = %s, want an explicit hdr10_plus false", checked)
	}

	var legacy models.VideoTrack
	if err := json.Unmarshal([]byte(`{"codec":"hevc","color_transfer":"smpte2084"}`), &legacy); err != nil {
		t.Fatal(err)
	}
	if !legacy.HDR10PlusUnchecked {
		t.Fatal("a stored track without hdr10_plus decoded as checked")
	}
	rewritten, err := json.Marshal(legacy)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(rewritten), "hdr10_plus") {
		t.Fatalf("rewriting an unchecked track = %s, want hdr10_plus left out so the backfill still checks it", rewritten)
	}
}

// Only the primary stream's frame is read, so a second PQ video stream must
// not be stored as unchecked: nothing would ever check it.
func TestConvertProbeDataLeavesOnlyThePrimaryVideoUnchecked(t *testing.T) {
	stream := ffprobeStream{CodecType: "video", CodecName: "hevc", ColorTransfer: "smpte2084"}
	probe := convertProbeData(&ffprobeOutput{Streams: []ffprobeStream{stream, stream}})
	if len(probe.VideoTracks) != 2 {
		t.Fatalf("video tracks = %d, want 2", len(probe.VideoTracks))
	}
	if !probe.VideoTracks[0].HDR10PlusUnchecked || probe.VideoTracks[1].HDR10PlusUnchecked {
		t.Fatalf("unchecked = %v, %v; want only the primary track unchecked",
			probe.VideoTracks[0].HDR10PlusUnchecked, probe.VideoTracks[1].HDR10PlusUnchecked)
	}
}

// commandError runs script and returns the *exec.ExitError it produces, the
// shape a failed ffprobe read returns.
func commandError(t *testing.T, script string) error {
	t.Helper()
	_, err := exec.Command("sh", "-c", script).Output()
	if _, ok := errors.AsType[*exec.ExitError](err); !ok {
		t.Fatalf("script %q returned %v, want an exit error", script, err)
	}
	return err
}

// hdr10PlusBackfillRow is one stored file and the fake frame read's answer
// for it.
type hdr10PlusBackfillRow struct {
	name     string
	tracks   string
	sideData []ffprobeSideData
	readErr  error
}

func insertHDR10PlusBackfillRow(t *testing.T, ctx context.Context, pool *pgxpool.Pool, folderID int, path, tracks string) (int, time.Time) {
	t.Helper()
	probedAt := time.Date(2026, 8, 13, 5, 47, 23, 0, time.UTC)
	modifiedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	var fileID int
	if err := pool.QueryRow(ctx, `
		INSERT INTO media_files (content_id, media_folder_id, file_path, file_size, file_modified_at,
			container, codec_video, video_tracks, audio_tracks, chapters, probe_source, probe_updated_at)
		VALUES ($1, $2, $3, 1000, $4, 'mkv', 'hevc', $5::jsonb, '[{"codec":"eac3"}]',
			'[{"start":0,"end":60,"title":"Opening","thumbnail_path":"chapters/1.jpg"}]', 'local', $6)
		RETURNING id`,
		fmt.Sprintf("hdr10plus-backfill-%d", time.Now().UnixNano()), folderID, path, modifiedAt, tracks, probedAt,
	).Scan(&fileID); err != nil {
		t.Fatalf("insert media file %s: %v", path, err)
	}
	return fileID, probedAt
}

// The backfill reads the first frame of each unchecked PQ file, sets only
// hdr10_plus and, for an HDR10+ file, video_range_type on its first track,
// and leaves the rest of the row as it found it. A read that could give a
// different answer later leaves the file a candidate; a second pass reads
// only those.
func TestHDR10PlusBackfillRecordsFrameStatusDB(t *testing.T) {
	pool := newDeadRootTestPool(t)
	ctx := context.Background()
	folderID := seedDeadRootTestFolder(t, pool, "movies", fmt.Sprintf("hdr10plus-backfill-%d", time.Now().UnixNano()))

	hdr10PlusFrame := []ffprobeSideData{
		{SideDataType: "Mastering display metadata"},
		{SideDataType: "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"},
	}
	staticFrame := hdr10PlusFrame[:1]
	const (
		hdr10Track = `{"codec":"hevc","color_transfer":"smpte2084","video_range":"HDR","video_range_type":"HDR10","future_field":"kept"}`
		dv81Track  = `{"codec":"hevc","color_transfer":"smpte2084","video_range":"DolbyVision","video_range_type":"DOVIWithHDR10","dv_profile":8,"dv_bl_compat_id":1,"dv_config_present":true,"dv_bl_compat_id_present":true}`
		sdrTrack   = `{"codec":"h264","color_transfer":"bt709","video_range_type":"SDR"}`
	)
	rows := []hdr10PlusBackfillRow{
		{name: "hdr10plus", tracks: "[" + hdr10Track + "]", sideData: hdr10PlusFrame},
		{name: "dv81-hdr10plus", tracks: "[" + dv81Track + "]", sideData: hdr10PlusFrame},
		{name: "plain-hdr10", tracks: `[{"codec":"hevc","color_transfer":"smpte2084"}]`, sideData: staticFrame},
		{name: "undecodable", tracks: "[" + hdr10Track + "]", readErr: commandError(t, "echo 'Invalid data found when processing input' >&2; exit 1")},
		{name: "storage-error", tracks: "[" + hdr10Track + "]", readErr: commandError(t, "echo 'movie.mkv: Input/output error' >&2; exit 1")},
		{name: "timed-out", tracks: "[" + hdr10Track + "]", readErr: commandError(t, "kill -9 $$")},
		{name: "sdr", tracks: "[" + sdrTrack + "]", sideData: hdr10PlusFrame},
		{name: "already-checked", tracks: `[{"codec":"hevc","color_transfer":"smpte2084","hdr10_plus":false}]`, sideData: hdr10PlusFrame},
	}
	byPath := make(map[string]hdr10PlusBackfillRow, len(rows))
	ids := make(map[string]int, len(rows))
	var probedAt time.Time
	dir := t.TempDir()
	for _, row := range rows {
		path := dir + "/" + row.name + ".mkv"
		byPath[path] = row
		ids[row.name], probedAt = insertHDR10PlusBackfillRow(t, ctx, pool, folderID, path, row.tracks)
	}

	var mu sync.Mutex
	reads := map[string]int{}
	backfiller := NewHDR10PlusBackfiller(NewFileRepository(pool), "ffprobe")
	backfiller.readFrame = func(_ context.Context, _, filePath, streamSpecifier string) ([]ffprobeSideData, error) {
		row, ok := byPath[filePath]
		if !ok {
			// Another test's row: leave it untouched.
			return nil, errors.New("not this test's file")
		}
		if streamSpecifier != "V:0" {
			t.Errorf("stream specifier = %q, want the primary video V:0", streamSpecifier)
		}
		mu.Lock()
		reads[row.name]++
		mu.Unlock()
		return row.sideData, row.readErr
	}

	if _, err := backfiller.Run(ctx, nil); err != nil {
		t.Fatalf("first pass: %v", err)
	}
	track := func(name string) map[string]any {
		t.Helper()
		var raw []byte
		if err := pool.QueryRow(ctx, `SELECT video_tracks->0 FROM media_files WHERE id = $1`, ids[name]).Scan(&raw); err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		var fields map[string]any
		if err := json.Unmarshal(raw, &fields); err != nil {
			t.Fatal(err)
		}
		return fields
	}

	if got := track("hdr10plus"); got["hdr10_plus"] != true || got["video_range_type"] != "HDR10Plus" || got["future_field"] != "kept" {
		t.Fatalf("HDR10+ file = %v, want hdr10_plus true, range HDR10Plus and its other fields kept", got)
	}
	if got := track("dv81-hdr10plus"); got["hdr10_plus"] != true || got["video_range_type"] != "DOVIWithHDR10Plus" {
		t.Fatalf("DV 8.1 file = %v, want hdr10_plus true, range DOVIWithHDR10Plus", got)
	}
	if got := track("plain-hdr10"); got["hdr10_plus"] != false {
		t.Fatalf("plain HDR10 file = %v, want hdr10_plus false", got)
	} else if _, ok := got["video_range_type"]; ok {
		t.Fatalf("plain HDR10 file = %v, want no video_range_type added", got)
	}
	if got := track("undecodable"); got["hdr10_plus"] != false || got["video_range_type"] != "HDR10" {
		t.Fatalf("undecodable file = %v, want it recorded as not HDR10+", got)
	}
	for _, name := range []string{"storage-error", "timed-out"} {
		if _, ok := track(name)["hdr10_plus"]; ok {
			t.Fatalf("%s file = %v, want it left unchecked for the next pass", name, track(name))
		}
	}
	if got := track("already-checked"); got["hdr10_plus"] != false {
		t.Fatalf("already checked file = %v, want it unchanged", got)
	}
	for _, name := range []string{"sdr", "already-checked"} {
		if reads[name] != 0 {
			t.Fatalf("%s file was read %d times, want it skipped", name, reads[name])
		}
	}

	// Everything else in the row stays as it was.
	var chapters string
	var storedProbe time.Time
	if err := pool.QueryRow(ctx, `SELECT chapters::text, probe_updated_at FROM media_files WHERE id = $1`, ids["hdr10plus"]).Scan(&chapters, &storedProbe); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(chapters, "chapters/1.jpg") || !storedProbe.Equal(probedAt) {
		t.Fatalf("chapters = %s, probe_updated_at = %v; want both untouched", chapters, storedProbe)
	}

	for name := range reads {
		reads[name] = 0
	}
	if _, err := backfiller.Run(ctx, nil); err != nil {
		t.Fatalf("second pass: %v", err)
	}
	for name, count := range reads {
		want := 0
		if name == "storage-error" || name == "timed-out" {
			want = 1
		}
		if count != want {
			t.Fatalf("second pass read %s %d times, want %d", name, count, want)
		}
	}
}

// A row rewritten while its frame was read keeps what the rewrite stored.
func TestHDR10PlusBackfillLeavesARowRewrittenDuringTheReadDB(t *testing.T) {
	pool := newDeadRootTestPool(t)
	ctx := context.Background()
	folderID := seedDeadRootTestFolder(t, pool, "movies", fmt.Sprintf("hdr10plus-rewrite-%d", time.Now().UnixNano()))
	path := t.TempDir() + "/rewritten.mkv"
	fileID, _ := insertHDR10PlusBackfillRow(t, ctx, pool, folderID, path, `[{"codec":"hevc","color_transfer":"smpte2084","video_range_type":"HDR10"}]`)

	backfiller := NewHDR10PlusBackfiller(NewFileRepository(pool), "ffprobe")
	backfiller.readFrame = func(ctx context.Context, _, filePath, _ string) ([]ffprobeSideData, error) {
		if filePath != path {
			return nil, errors.New("not this test's file")
		}
		// A scan reprobes the file while its frame is being read.
		if _, err := pool.Exec(ctx, `UPDATE media_files SET video_tracks = '[{"codec":"hevc","color_transfer":"smpte2084","video_range_type":"HDR10","bit_depth":10}]' WHERE id = $1`, fileID); err != nil {
			t.Errorf("rewrite row: %v", err)
		}
		return []ffprobeSideData{{SideDataType: "HDR Dynamic Metadata SMPTE2094-40 (HDR10+)"}}, nil
	}
	result, err := backfiller.Run(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if result.Changed < 1 {
		t.Fatalf("result = %+v, want the rewritten row counted as changed", result)
	}
	var kept bool
	if err := pool.QueryRow(ctx, `SELECT video_tracks->0->>'hdr10_plus' IS NULL AND (video_tracks->0->>'bit_depth') = '10' FROM media_files WHERE id = $1`, fileID).Scan(&kept); err != nil {
		t.Fatal(err)
	}
	if !kept {
		t.Fatal("the backfill wrote over a row a scan rewrote during the read")
	}
}

// Consecutive timeouts mean stalled storage: the pass ends rather than wait
// out every remaining file.
func TestHDR10PlusBackfillStopsWhenStorageStallsDB(t *testing.T) {
	pool := newDeadRootTestPool(t)
	ctx := context.Background()
	folderID := seedDeadRootTestFolder(t, pool, "movies", fmt.Sprintf("hdr10plus-stall-%d", time.Now().UnixNano()))
	dir := t.TempDir()
	paths := map[string]bool{}
	for i := range hdr10PlusBackfillMaxTimeouts + 4 {
		path := fmt.Sprintf("%s/stalled-%d.mkv", dir, i)
		paths[path] = true
		insertHDR10PlusBackfillRow(t, ctx, pool, folderID, path, `[{"codec":"hevc","color_transfer":"smpte2084"}]`)
	}
	killed := commandError(t, "kill -9 $$")
	backfiller := NewHDR10PlusBackfiller(NewFileRepository(pool), "ffprobe")
	backfiller.workers = 1
	backfiller.readFrame = func(_ context.Context, _, filePath, _ string) ([]ffprobeSideData, error) {
		if !paths[filePath] {
			return nil, errors.New("not this test's file")
		}
		return nil, killed
	}
	_, err := backfiller.Run(ctx, nil)
	if !errors.Is(err, errHDR10PlusStorageStalled) {
		t.Fatalf("Run error = %v, want the stalled-storage stop", err)
	}
}

func TestNewHDR10PlusBackfillerNeedsADatabase(t *testing.T) {
	if NewHDR10PlusBackfiller(nil, "ffprobe") != nil || NewHDR10PlusBackfiller(&FileRepository{}, "ffprobe") != nil {
		t.Fatal("a backfiller without a database was built")
	}
}
