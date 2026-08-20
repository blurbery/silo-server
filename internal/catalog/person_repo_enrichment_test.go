package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Silo-Server/silo-server/internal/models"
)

func TestPersonPhotoEnrichmentSQLGuardsExistingArtwork(t *testing.T) {
	single := personPhotoFillPredicate("photo_path", "$1")
	for _, fragment := range []string{
		"COALESCE(photo_path, '') = '' AND $1 <> ''",
		"photo_path = '-' AND $1 NOT IN ('', '-')",
	} {
		if !strings.Contains(single, fragment) {
			t.Fatalf("single photo predicate %q is missing %q", single, fragment)
		}
	}

	batch := batchPersonEnrichmentQuery()
	for _, field := range []string{"photo_path", "photo_source_path", "photo_thumbhash"} {
		emptyGuard := fmt.Sprintf("COALESCE(people.%s, '') = '' AND t.%s <> ''", field, field)
		sentinelGuard := fmt.Sprintf("people.%s = '-' AND t.%s NOT IN ('', '-')", field, field)
		preserveExisting := fmt.Sprintf("ELSE people.%s END", field)
		for _, fragment := range []string{emptyGuard, sentinelGuard, preserveExisting} {
			if !strings.Contains(batch, fragment) {
				t.Fatalf("batch enrichment SQL for %s is missing %q", field, fragment)
			}
		}
		destructive := fmt.Sprintf("WHEN t.%s NOT IN ('', '-') THEN t.%s", field, field)
		if strings.Contains(batch, destructive) {
			t.Fatalf("batch enrichment SQL still unconditionally overwrites %s", field)
		}
	}
	if !strings.Contains(batch, "WHERE people.id = t.id") || !strings.Contains(batch, "updated_at = NOW()") {
		t.Fatal("batch enrichment SQL lost its guarded update or timestamp assignment")
	}
}

func TestPersonCreditEnrichmentPreservesCachedArtwork(t *testing.T) {
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
	repo := NewPersonRepository(pool)

	type seededPerson struct {
		id        int64
		tmdbID    string
		photoPath string
		updatedAt time.Time
	}
	seed := func(label string) seededPerson {
		t.Helper()
		nowID := time.Now().UnixNano()
		seeded := seededPerson{
			id:        nowID,
			tmdbID:    fmt.Sprintf("credit-enrichment-%s-%d", label, nowID),
			photoPath: fmt.Sprintf("tmdb/people/%d/profile/original.cached.webp", nowID),
			updatedAt: time.Now().UTC().Add(-48 * time.Hour).Truncate(time.Microsecond),
		}
		_, err := pool.Exec(ctx, `
			INSERT INTO people (
				id, name, tmdb_id, imdb_id, tvdb_id, plex_guid,
				photo_path, photo_source_path, photo_thumbhash,
				bio, birthplace, homepage, updated_at
			) VALUES (
				$1, $2, $3, $4, $5, $6,
				$7, 'https://images.example/original.jpg', 'existing-thumbhash',
				'existing bio', 'existing birthplace', 'https://example.com', $8
			)
		`, seeded.id, "Credit Enrichment "+label, seeded.tmdbID,
			fmt.Sprintf("existing-imdb-%d", nowID), fmt.Sprintf("existing-tvdb-%d", nowID),
			fmt.Sprintf("existing-plex-%d", nowID), seeded.photoPath, seeded.updatedAt)
		if err != nil {
			t.Fatalf("seed person: %v", err)
		}
		t.Cleanup(func() {
			_, _ = pool.Exec(ctx, `DELETE FROM people WHERE id = $1`, seeded.id)
			_, _ = pool.Exec(ctx, `DELETE FROM artwork_revision_gc_candidates WHERE original_path = $1`, seeded.photoPath)
		})
		return seeded
	}

	assertPreserved := func(seed seededPerson) {
		t.Helper()
		var photoPath, sourcePath, thumbhash string
		var updatedAt time.Time
		if err := pool.QueryRow(ctx, `
			SELECT photo_path, photo_source_path, photo_thumbhash, updated_at
			FROM people WHERE id = $1
		`, seed.id).Scan(&photoPath, &sourcePath, &thumbhash, &updatedAt); err != nil {
			t.Fatalf("read enriched person: %v", err)
		}
		if photoPath != seed.photoPath {
			t.Fatalf("cached photo_path was overwritten: got %q, want %q", photoPath, seed.photoPath)
		}
		if sourcePath != "https://images.example/original.jpg" {
			t.Fatalf("photo_source_path was overwritten: %q", sourcePath)
		}
		if thumbhash != "existing-thumbhash" {
			t.Fatalf("photo_thumbhash was overwritten: %q", thumbhash)
		}
		if !updatedAt.Equal(seed.updatedAt) {
			t.Fatalf("no-op enrichment changed updated_at: got %v, want %v", updatedAt, seed.updatedAt)
		}
		var gcCandidates int
		if err := pool.QueryRow(ctx, `
			SELECT count(*) FROM artwork_revision_gc_candidates WHERE original_path = $1
		`, seed.photoPath).Scan(&gcCandidates); err != nil {
			t.Fatalf("count artwork GC candidates: %v", err)
		}
		if gcCandidates != 0 {
			t.Fatalf("no-op enrichment armed %d artwork GC candidates, want 0", gcCandidates)
		}
	}

	incoming := func(seed seededPerson) models.Person {
		return models.Person{
			Name:            "Credit Enrichment",
			TmdbID:          seed.tmdbID,
			ImdbID:          "replacement-imdb",
			TvdbID:          "replacement-tvdb",
			PlexGUID:        "replacement-plex",
			PhotoPath:       "https://images.example/replacement.jpg",
			PhotoSourcePath: "https://images.example/replacement-source.jpg",
			PhotoThumbhash:  "replacement-thumbhash",
			Bio:             "replacement bio",
			Birthplace:      "replacement birthplace",
			Homepage:        "https://replacement.example.com",
		}
	}

	t.Run("single find or create", func(t *testing.T) {
		seeded := seed("single")
		id, err := repo.FindOrCreate(ctx, incoming(seeded))
		if err != nil {
			t.Fatalf("FindOrCreate: %v", err)
		}
		if id != seeded.id {
			t.Fatalf("FindOrCreate id = %d, want %d", id, seeded.id)
		}
		assertPreserved(seeded)
	})

	t.Run("batch find or create", func(t *testing.T) {
		seeded := seed("batch")
		ids, err := repo.BatchFindOrCreate(ctx, []models.Person{incoming(seeded)})
		if err != nil {
			t.Fatalf("BatchFindOrCreate: %v", err)
		}
		if len(ids) != 1 || ids[0] != seeded.id {
			t.Fatalf("BatchFindOrCreate ids = %v, want [%d]", ids, seeded.id)
		}
		assertPreserved(seeded)
	})

	t.Run("real photo replaces no-photo sentinel", func(t *testing.T) {
		seeded := seed("sentinel")
		if _, err := pool.Exec(ctx, `
			UPDATE people
			SET photo_path = '-', photo_source_path = '', photo_thumbhash = '', updated_at = $2
			WHERE id = $1
		`, seeded.id, seeded.updatedAt); err != nil {
			t.Fatalf("set no-photo sentinel: %v", err)
		}

		ids, err := repo.BatchFindOrCreate(ctx, []models.Person{incoming(seeded)})
		if err != nil {
			t.Fatalf("BatchFindOrCreate: %v", err)
		}
		if len(ids) != 1 || ids[0] != seeded.id {
			t.Fatalf("BatchFindOrCreate ids = %v, want [%d]", ids, seeded.id)
		}

		var photoPath, sourcePath, thumbhash string
		var updatedAt time.Time
		if err := pool.QueryRow(ctx, `
			SELECT photo_path, photo_source_path, photo_thumbhash, updated_at
			FROM people WHERE id = $1
		`, seeded.id).Scan(&photoPath, &sourcePath, &thumbhash, &updatedAt); err != nil {
			t.Fatalf("read sentinel replacement: %v", err)
		}
		if photoPath != "https://images.example/replacement.jpg" ||
			sourcePath != "https://images.example/replacement-source.jpg" ||
			thumbhash != "replacement-thumbhash" {
			t.Fatalf("real photo did not replace sentinel: path=%q source=%q thumbhash=%q", photoPath, sourcePath, thumbhash)
		}
		if !updatedAt.After(seeded.updatedAt) {
			t.Fatalf("sentinel replacement did not advance updated_at: got %v, previous %v", updatedAt, seeded.updatedAt)
		}
	})
}
