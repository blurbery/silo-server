package catalog

import (
	"context"
	"os"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPersonRefreshPrioritisesRecentCreditsWithoutStarvingBacklog(t *testing.T) {
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	config.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	// A connection-local table isolates ordering assertions from other fixtures.
	_, err = pool.Exec(ctx, `
		CREATE TEMP TABLE people (
			id bigint PRIMARY KEY, tmdb_id text DEFAULT '', imdb_id text DEFAULT '',
			tvdb_id text DEFAULT '', bio text DEFAULT '', photo_path text DEFAULT '',
			birth_date date, updated_at timestamptz NOT NULL,
			metadata_refresh_attempted_at timestamptz
		);
		INSERT INTO people (id, tmdb_id, updated_at)
		SELECT id, id::text, NOW() - CASE WHEN id <= 10 THEN interval '100 days' ELSE interval '1 hour' END
		FROM generate_series(1, 20) AS id;
		INSERT INTO people (id, tmdb_id, updated_at, metadata_refresh_attempted_at)
		VALUES (21, '21', NOW(), NOW()), (22, '', NOW(), NULL);
		INSERT INTO people (id, tmdb_id, updated_at, bio, photo_path, birth_date)
		VALUES (23, '23', NOW(), 'Complete', '-', '1980-01-01');
	`)
	if err != nil {
		t.Fatal(err)
	}
	repo := NewPersonRepository(pool)
	for _, tc := range []struct {
		name  string
		limit int
		want  []int64
	}{
		{"bounded recent and oldest shares", 10, []int64{20, 19, 18, 17, 16, 15, 14, 13, 1, 2}},
		{"single slot favours recent", 1, []int64{20}},
		{"zero slots", 0, []int64{}},
		{"short batch has no duplicates or ineligible people", 25, []int64{20, 19, 18, 17, 16, 15, 14, 13, 12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.FindRefreshCandidates(ctx, tc.limit)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tc.want) {
				t.Fatalf("candidates = %v, want %v", got, tc.want)
			}
		})
	}
	// Enrichment from a new credit updates an existing person's timestamp.
	if _, err := pool.Exec(ctx, `UPDATE people SET updated_at = NOW() WHERE id = 5`); err != nil {
		t.Fatal(err)
	}
	got, err := repo.FindRefreshCandidates(ctx, 1)
	if err != nil || !slices.Equal(got, []int64{5}) {
		t.Fatalf("enriched credit candidates = %v, err = %v", got, err)
	}
}
