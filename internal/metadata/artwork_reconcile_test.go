package metadata

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// fakeObjectChecker treats every key as present unless listed in missing or
// erroring. Defaulting to present keeps sweeps over rows seeded by other
// tests in the shared test database side-effect free.
type fakeObjectChecker struct {
	mu       sync.Mutex
	missing  map[string]bool
	erroring map[string]bool
	errorAll bool
	checked  map[string]int
}

func (f *fakeObjectChecker) Bucket() string { return "test-bucket" }

func (f *fakeObjectChecker) ObjectExists(_ context.Context, _ string, key string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.checked == nil {
		f.checked = map[string]int{}
	}
	f.checked[key]++
	if f.errorAll || f.erroring[key] {
		return false, errors.New("simulated storage error")
	}
	return !f.missing[key], nil
}

func TestShouldBulkReset(t *testing.T) {
	if shouldBulkReset(0, 0) {
		t.Fatal("empty probe must not trigger bulk reset")
	}
	if shouldBulkReset(100, 94) {
		t.Fatal("94% missing is below the bulk threshold")
	}
	if !shouldBulkReset(100, 95) {
		t.Fatal("95% missing must trigger bulk reset")
	}
	// A probe thinned below the minimum successful-sample bar (transport
	// errors, tiny catalog) must take the safe per-row path even at a 100%
	// miss rate — a handful of surviving 404s is not a mandate to bulk-reset.
	if shouldBulkReset(artworkReconcileBulkMinSample-1, artworkReconcileBulkMinSample-1) {
		t.Fatal("below-minimum sample must not trigger bulk reset")
	}
	if !shouldBulkReset(artworkReconcileBulkMinSample, artworkReconcileBulkMinSample) {
		t.Fatal("all-missing probe at the minimum sample size must trigger bulk reset")
	}
}

func TestArtworkSweepSurfacesUseIndexablePaginationKeys(t *testing.T) {
	for _, surface := range artworkSweepSurfaces() {
		for _, keyCol := range surface.keyCols {
			if strings.Contains(keyCol.column, "::") {
				t.Fatalf("surface %q paginates on expression %q; use the native key column so PostgreSQL can use its index", surface.name, keyCol.column)
			}
		}
	}
}

func TestBuildSweepBatchQueryUsesNativeNumericKeys(t *testing.T) {
	var peopleSurface, folderSurface artworkSweepSurface
	for _, surface := range artworkSweepSurfaces() {
		switch surface.name {
		case "person photos":
			peopleSurface = surface
		case "library posters":
			folderSurface = surface
		}
	}

	query, args, err := buildSweepBatchQuery(peopleSurface, []string{"128111764822294558"})
	if err != nil {
		t.Fatalf("build people query: %v", err)
	}
	if !strings.Contains(query, "SELECT (id)::text") ||
		!strings.Contains(query, "AND (id) > ($1)") ||
		!strings.Contains(query, "ORDER BY id LIMIT $2") {
		t.Fatalf("people query does not keep pagination on native id: %s", query)
	}
	if _, ok := args[0].(int64); !ok {
		t.Fatalf("people cursor type = %T, want int64", args[0])
	}

	_, args, err = buildSweepBatchQuery(folderSurface, []string{"42"})
	if err != nil {
		t.Fatalf("build folder query: %v", err)
	}
	if _, ok := args[0].(int32); !ok {
		t.Fatalf("folder cursor type = %T, want int32", args[0])
	}
}

func TestArtworkReconcileVerifySweep(t *testing.T) {
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
	id := func(name string) string { return fmt.Sprintf("arc-%s-%d", name, suffix) }
	key := func(name string) string { return fmt.Sprintf("tmdb/movies/arc-%d/%s/original.webp", suffix, name) }

	// Four items covering the sweep verdicts: intact, missing with a provider
	// source, missing with an upload source, and an uncached provider URL.
	seedItem := func(contentID, posterPath, posterSource string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO media_items (content_id, type, title, status, genres, poster_path, poster_source_path, last_refreshed)
			VALUES ($1, 'movie', 'ARC Test', 'matched', '{}'::text[], $2, $3, NOW())
		`, contentID, posterPath, posterSource); err != nil {
			t.Fatalf("seed item %s: %v", contentID, err)
		}
	}
	seedItem(id("intact"), key("intact"), "https://img.example/intact.jpg")
	seedItem(id("missing"), key("missing"), "https://img.example/missing.jpg")
	seedItem(id("upload"), key("upload"), "upload://admin/poster.jpg")
	seedItem(id("uncached"), "https://img.example/direct.jpg", "https://img.example/direct.jpg")

	personID := suffix
	personKey := key("person-missing")
	if _, err := pool.Exec(ctx, `
		INSERT INTO people (id, name, photo_path, photo_source_path)
		VALUES ($1, 'ARC Person', $2, 'https://img.example/person.jpg')
	`, personID, personKey); err != nil {
		t.Fatalf("seed person: %v", err)
	}

	var fileID int64
	chapters := fmt.Sprintf(
		`[{"index":0,"title":"One","thumbnail_path":%q,"thumbnail_thumbhash":"aGFzaA==","custom":"kept"},`+
			`{"index":1,"title":"Two","thumbnail_path":%q,"thumbnail_thumbhash":"aGFzaA==","thumbnail_failed_at":"2026-01-01T00:00:00Z"}]`,
		key("chapter-intact"), key("chapter-missing"),
	)
	var folderID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO media_folders (type, name, enabled, poster_path) VALUES ('movies', 'ARC Folder', true, $1) RETURNING id`,
		fmt.Sprintf("library-posters/arc-%d.png", suffix),
	).Scan(&folderID); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	if err := pool.QueryRow(ctx, `
		INSERT INTO media_files (content_id, media_folder_id, file_path, chapters)
		VALUES ($1, $2, $3, $4::jsonb) RETURNING id
	`, id("intact"), folderID, fmt.Sprintf("/arc-%d/movie.mkv", suffix), chapters).Scan(&fileID); err != nil {
		t.Fatalf("seed file: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO library_collections (id, library_id, slug, title, collection_type, poster_url, poster_thumbhash, poster_from_template)
		VALUES ($1, $2, $1, 'ARC Collection', 'manual', $3, 'aGFzaA==', TRUE)
	`, id("coll"), folderID, key("coll")); err != nil {
		t.Fatalf("seed collection: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM library_collections WHERE id = $1`, id("coll"))
		_, _ = pool.Exec(ctx, `DELETE FROM media_files WHERE id = $1`, fileID)
		_, _ = pool.Exec(ctx, `DELETE FROM media_folders WHERE id = $1`, folderID)
		_, _ = pool.Exec(ctx, `DELETE FROM people WHERE id = $1`, personID)
		for _, name := range []string{"intact", "missing", "upload", "uncached"} {
			_, _ = pool.Exec(ctx, `DELETE FROM media_items WHERE content_id = $1`, id(name))
		}
	})

	checker := &fakeObjectChecker{missing: map[string]bool{
		key("missing"):         true,
		key("upload"):          true,
		personKey:              true,
		key("chapter-missing"): true,
		key("coll"):            true,
		fmt.Sprintf("library-posters/arc-%d.png", suffix): true,
	}}
	stats, err := NewArtworkCacheReconciler(pool, checker).Run(ctx, nil)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Mode != "verify" {
		t.Fatalf("Mode = %q, want verify (fake checker defaults to present)", stats.Mode)
	}

	var posterPath string
	var lastRefreshed *time.Time
	mustScanItem := func(contentID string) (string, *time.Time) {
		if err := pool.QueryRow(ctx,
			`SELECT poster_path, last_refreshed FROM media_items WHERE content_id = $1`, contentID,
		).Scan(&posterPath, &lastRefreshed); err != nil {
			t.Fatalf("read item %s: %v", contentID, err)
		}
		return posterPath, lastRefreshed
	}

	if got, _ := mustScanItem(id("intact")); got != key("intact") {
		t.Fatalf("intact poster_path = %q, want untouched %q", got, key("intact"))
	}
	if got, _ := mustScanItem(id("missing")); got != "https://img.example/missing.jpg" {
		t.Fatalf("missing poster_path = %q, want reset to provider source", got)
	}
	if got, refreshed := mustScanItem(id("upload")); got != "" || refreshed != nil {
		t.Fatalf("upload poster_path = %q (last_refreshed %v), want cleared with last_refreshed NULL", got, refreshed)
	}
	if got, _ := mustScanItem(id("uncached")); got != "https://img.example/direct.jpg" {
		t.Fatalf("uncached poster_path = %q, want untouched provider URL", got)
	}
	if checker.checked["https://img.example/direct.jpg"] != 0 {
		t.Fatal("provider URLs must not be HEAD-checked")
	}

	var personPhoto string
	if err := pool.QueryRow(ctx, `SELECT photo_path FROM people WHERE id = $1`, personID).Scan(&personPhoto); err != nil {
		t.Fatalf("read person: %v", err)
	}
	if personPhoto != "https://img.example/person.jpg" {
		t.Fatalf("missing person photo_path = %q, want reset to provider source", personPhoto)
	}

	var rawChapters string
	var retryAfter *time.Time
	if err := pool.QueryRow(ctx,
		`SELECT chapters::text, chapter_thumbnail_retry_after FROM media_files WHERE id = $1`, fileID,
	).Scan(&rawChapters, &retryAfter); err != nil {
		t.Fatalf("read chapters: %v", err)
	}
	assertContains := func(s, substr, what string) {
		t.Helper()
		if !strings.Contains(s, substr) {
			t.Fatalf("%s: %q not found in %s", what, substr, s)
		}
	}
	assertContains(rawChapters, key("chapter-intact"), "intact chapter thumbnail kept")
	assertContains(rawChapters, `"kept"`, "unknown chapter fields preserved")
	if strings.Contains(rawChapters, key("chapter-missing")) || strings.Contains(rawChapters, "thumbnail_failed_at") {
		t.Fatalf("missing chapter thumbnail not cleared: %s", rawChapters)
	}
	if retryAfter != nil {
		t.Fatal("chapter_thumbnail_retry_after not cleared")
	}

	var collPoster, collHash string
	var fromTemplate bool
	if err := pool.QueryRow(ctx,
		`SELECT poster_url, poster_thumbhash, poster_from_template FROM library_collections WHERE id = $1`, id("coll"),
	).Scan(&collPoster, &collHash, &fromTemplate); err != nil {
		t.Fatalf("read collection: %v", err)
	}
	if collPoster != "" || collHash != "" || fromTemplate {
		t.Fatalf("collection artwork not fully cleared: url=%q hash=%q from_template=%v", collPoster, collHash, fromTemplate)
	}

	var folderPoster string
	if err := pool.QueryRow(ctx, `SELECT poster_path FROM media_folders WHERE id = $1`, folderID).Scan(&folderPoster); err != nil {
		t.Fatalf("read folder: %v", err)
	}
	if folderPoster != "" {
		t.Fatalf("library poster not cleared: %q", folderPoster)
	}
}

func TestArtworkReconcileResumesFromSavedBatch(t *testing.T) {
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

	prefix := fmt.Sprintf("arc-resume-%d-", time.Now().UnixNano())
	if _, err := pool.Exec(ctx, `
		INSERT INTO media_items (content_id, type, title, status, genres, poster_path, poster_source_path)
		SELECT
			$1 || lpad(n::text, 4, '0'),
			'movie', 'ARC Resume', 'matched', '{}'::text[],
			'tmdb/movies/' || $1 || lpad(n::text, 4, '0') || '/poster/original.webp',
			'https://img.example/' || $1 || lpad(n::text, 4, '0') || '.jpg'
		FROM generate_series(0, 500) AS n
	`, prefix); err != nil {
		t.Fatalf("seed resumable artwork: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM media_items WHERE content_id LIKE $1`, prefix+"%")
	})

	surface := artworkSweepSurface{
		name:      "resume test posters",
		table:     "media_items",
		keyCols:   []artworkSweepKey{textSweepKey("content_id")},
		pathCol:   "poster_path",
		sourceCol: "poster_source_path",
		clearSet:  `poster_path = '', last_refreshed = NULL, updated_at = NOW()`,
	}
	checker := &fakeObjectChecker{}
	reconciler := NewArtworkCacheReconciler(pool, checker)
	checkpoint := ArtworkReconcileCheckpoint{
		Version: artworkReconcileCheckpointVersion,
		Totals:  []int{501},
		Stats:   ArtworkReconcileStats{Mode: "verify"},
	}
	stopAfterFirstBatch := errors.New("simulated restart")
	var saved ArtworkReconcileCheckpoint
	_, err = reconciler.runVerifySweep(ctx, []artworkSweepSurface{surface}, checkpoint,
		func(next ArtworkReconcileCheckpoint) error {
			saved = cloneArtworkReconcileCheckpoint(next)
			if next.SurfaceDone == artworkReconcileBatchSize {
				return stopAfterFirstBatch
			}
			return nil
		}, func(float64, string) {})
	if !errors.Is(err, stopAfterFirstBatch) {
		t.Fatalf("first sweep error = %v, want simulated restart", err)
	}
	if saved.SurfaceDone != artworkReconcileBatchSize || len(saved.SurfaceCursor) != 1 {
		t.Fatalf("saved checkpoint = %#v, want one completed batch", saved)
	}

	checker.mu.Lock()
	checker.checked = map[string]int{}
	checker.mu.Unlock()
	stats, err := reconciler.runVerifySweep(ctx, []artworkSweepSurface{surface}, saved, nil, func(float64, string) {})
	if err != nil {
		t.Fatalf("resumed sweep: %v", err)
	}
	if stats.Verified != 501 {
		t.Fatalf("resumed Verified = %d, want 501", stats.Verified)
	}
	firstKey := fmt.Sprintf("tmdb/movies/%s%04d/poster/original.webp", prefix, 0)
	lastKey := fmt.Sprintf("tmdb/movies/%s%04d/poster/original.webp", prefix, 500)
	checker.mu.Lock()
	firstChecks := checker.checked[firstKey]
	lastChecks := checker.checked[lastKey]
	checker.mu.Unlock()
	if firstChecks != 0 || lastChecks != 1 {
		t.Fatalf("resume checks: first=%d last=%d, want first=0 last=1", firstChecks, lastChecks)
	}
}

func TestArtworkReconcileLeavesRowsAloneOnStorageErrors(t *testing.T) {
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
	contentID := fmt.Sprintf("arc-err-%d", suffix)
	okContentID := fmt.Sprintf("arc-err-ok-%d", suffix)
	cachedKey := fmt.Sprintf("tmdb/movies/%s/poster/original.webp", contentID)
	okKey := fmt.Sprintf("tmdb/movies/%s/poster/original.webp", okContentID)
	seed := func(id, key string) {
		if _, err := pool.Exec(ctx, `
			INSERT INTO media_items (content_id, type, title, status, genres, poster_path, poster_source_path)
			VALUES ($1, 'movie', 'ARC Err', 'matched', '{}'::text[], $2, 'https://img.example/err.jpg')
		`, id, key); err != nil {
			t.Fatalf("seed item %s: %v", id, err)
		}
	}
	// The healthy sibling keeps the probe from concluding storage is
	// unreachable (an all-errored probe aborts before any sweep runs), so
	// the sweep-level skip-on-error behavior is what gets exercised.
	seed(contentID, cachedKey)
	seed(okContentID, okKey)
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM media_items WHERE content_id = ANY($1)`, []string{contentID, okContentID})
	})

	checker := &fakeObjectChecker{erroring: map[string]bool{cachedKey: true}}
	var saved ArtworkReconcileCheckpoint
	stats, err := NewArtworkCacheReconciler(pool, checker).RunResumable(ctx, nil,
		func(next ArtworkReconcileCheckpoint) error {
			saved = cloneArtworkReconcileCheckpoint(next)
			return nil
		}, nil)
	if err != nil {
		t.Fatalf("RunResumable: %v", err)
	}
	if stats.Errors == 0 || stats.SweepErrors == 0 {
		t.Fatal("expected the erroring key to be counted")
	}
	if saved.SurfaceIndex != 0 || len(saved.SurfaceCursor) != 0 || saved.Finished {
		t.Fatalf("checkpoint advanced past an errored batch: %#v", saved)
	}

	var posterPath string
	if err := pool.QueryRow(ctx, `SELECT poster_path FROM media_items WHERE content_id = $1`, contentID).Scan(&posterPath); err != nil {
		t.Fatalf("read item: %v", err)
	}
	if posterPath != cachedKey {
		t.Fatalf("poster_path = %q, want untouched %q after storage error", posterPath, cachedKey)
	}
}

func TestArtworkReconcileAbortsWhenStorageUnreachable(t *testing.T) {
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
	contentID := fmt.Sprintf("arc-down-%d", suffix)
	cachedKey := fmt.Sprintf("tmdb/movies/%s/poster/original.webp", contentID)
	if _, err := pool.Exec(ctx, `
		INSERT INTO media_items (content_id, type, title, status, genres, poster_path, poster_source_path)
		VALUES ($1, 'movie', 'ARC Down', 'matched', '{}'::text[], $2, 'https://img.example/down.jpg')
	`, contentID, cachedKey); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(ctx, `DELETE FROM media_items WHERE content_id = $1`, contentID) })

	// Every probe HEAD errors: storage is unreachable, which must abort the
	// run (missing ≠ unreachable) and leave the row untouched.
	checker := &fakeObjectChecker{errorAll: true}
	if _, err := NewArtworkCacheReconciler(pool, checker).Run(ctx, nil); err == nil {
		t.Fatal("Run with unreachable storage returned nil error")
	}

	var posterPath string
	if err := pool.QueryRow(ctx, `SELECT poster_path FROM media_items WHERE content_id = $1`, contentID).Scan(&posterPath); err != nil {
		t.Fatalf("read item: %v", err)
	}
	if posterPath != cachedKey {
		t.Fatalf("poster_path = %q, want untouched %q after unreachable storage", posterPath, cachedKey)
	}
}
