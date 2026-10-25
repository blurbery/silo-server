package pgstore

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Silo-Server/silo-server/internal/userstore"
)

func TestCompactMediaItemIDsReturnsEmptySliceForNilInput(t *testing.T) {
	ids := compactMediaItemIDs(nil)
	if ids == nil {
		t.Fatal("compactMediaItemIDs(nil) returned nil, want empty slice for pgx array binding")
	}
	if len(ids) != 0 {
		t.Fatalf("len = %d, want 0", len(ids))
	}
}

func TestSupersededEpisodeProgressQueryIsAccountScopedAndVisibilityAware(t *testing.T) {
	t.Parallel()

	expectedFragments := []string{
		"unnest($3::text[], $4::timestamptz[])",
		"completed_progress.user_id = $1",
		"completed_progress.profile_id = $2",
		"completed_progress.completed = TRUE",
		"completed_progress.updated_at > candidate.updated_at",
		"later_episode.series_id = current_episode.series_id",
		"(later_episode.season_number, later_episode.episode_number)",
		"user_history_hidden_items hidden",
		"completed_progress.updated_at <= hidden.hidden_before",
	}
	for _, fragment := range expectedFragments {
		if !strings.Contains(supersededEpisodeProgressQuery, fragment) {
			t.Errorf("superseded query missing %q:\n%s", fragment, supersededEpisodeProgressQuery)
		}
	}
}

func TestPostgresSupersededEpisodeProgressIDs(t *testing.T) {
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
	id := func(label string) string { return fmt.Sprintf("cw-exact-%s-%d", label, suffix) }
	profileID := id("shared-profile")
	seriesIDs := []string{id("newer"), id("older"), id("hidden"), id("equal"), id("other-user"), id("special")}
	episodeID := func(seriesIndex, episode int) string {
		return id(fmt.Sprintf("s%d-e%d", seriesIndex, episode))
	}
	movieID := id("movie")

	var userID, otherUserID int
	t.Cleanup(func() {
		userIDs := make([]int, 0, 2)
		if userID != 0 {
			userIDs = append(userIDs, userID)
		}
		if otherUserID != 0 {
			userIDs = append(userIDs, otherUserID)
		}
		if len(userIDs) > 0 {
			_, _ = pool.Exec(ctx, `DELETE FROM users WHERE id = ANY($1)`, userIDs)
		}
		_, _ = pool.Exec(ctx, `DELETE FROM media_items WHERE content_id = ANY($1)`, append(seriesIDs, movieID))
	})
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, role) VALUES ($1, 'user') RETURNING id`, id("user"),
	).Scan(&userID); err != nil {
		t.Fatalf("seed user: %v", err)
	}
	if err := pool.QueryRow(ctx,
		`INSERT INTO users (username, role) VALUES ($1, 'user') RETURNING id`, id("other-user-account"),
	).Scan(&otherUserID); err != nil {
		t.Fatalf("seed other user: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_profiles (id, user_id, name)
		VALUES ($1, $2, 'Primary'), ($1, $3, 'Other account')
	`, profileID, userID, otherUserID); err != nil {
		t.Fatalf("seed profiles: %v", err)
	}

	seedSupersededProgressCatalog(t, ctx, pool, seriesIDs, episodeID, movieID)
	specialID := id("s5-special")
	if _, err := pool.Exec(ctx, `
		INSERT INTO episodes (content_id, series_id, season_number, episode_number, title)
		VALUES ($1, $2, 0, 5, 'Special')
	`, specialID, seriesIDs[5]); err != nil {
		t.Fatalf("seed special: %v", err)
	}

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	seedSupersededProgressCompletions(t, ctx, pool, userID, otherUserID, profileID, episodeID, id("imported-"), base)

	candidates := []userstore.SupersededEpisodeCandidate{
		{MediaItemID: episodeID(0, 1), UpdatedAt: base},
		{MediaItemID: episodeID(1, 1), UpdatedAt: base.Add(time.Minute)},
		{MediaItemID: episodeID(2, 1), UpdatedAt: base},
		{MediaItemID: episodeID(3, 1), UpdatedAt: base},
		{MediaItemID: episodeID(4, 1), UpdatedAt: base},
		{MediaItemID: specialID, UpdatedAt: base},
		{MediaItemID: movieID, UpdatedAt: base},
		{MediaItemID: " ", UpdatedAt: base},
		{MediaItemID: episodeID(0, 1)},
	}
	store := newStore(pool, userID)
	got, err := store.SupersededEpisodeProgressIDs(ctx, profileID, candidates)
	if err != nil {
		t.Fatalf("SupersededEpisodeProgressIDs: %v", err)
	}
	wantID := episodeID(0, 1)
	if _, ok := got[wantID]; !ok || len(got) != 1 {
		t.Fatalf("superseded = %v, want only %s", got, wantID)
	}
}

// seedSupersededProgressCatalog seeds each series with episodes 1 and 2 of
// season 1, and one movie.
func seedSupersededProgressCatalog(t *testing.T, ctx context.Context, pool *pgxpool.Pool, seriesIDs []string, episodeID func(seriesIndex, episode int) string, movieID string) {
	t.Helper()
	for i, seriesID := range seriesIDs {
		if _, err := pool.Exec(ctx, `
			INSERT INTO media_items (content_id, type, title, status, genres)
			VALUES ($1, 'series', $2, 'matched', '{}'::text[])
		`, seriesID, fmt.Sprintf("Continue Watching exact series %d", i)); err != nil {
			t.Fatalf("seed series %d: %v", i, err)
		}
		if _, err := pool.Exec(ctx, `
			INSERT INTO episodes (content_id, series_id, season_number, episode_number, title)
			VALUES ($1, $3, 1, 1, 'One'), ($2, $3, 1, 2, 'Two')
		`, episodeID(i, 1), episodeID(i, 2), seriesID); err != nil {
			t.Fatalf("seed episodes for series %d: %v", i, err)
		}
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO media_items (content_id, type, title, status, genres)
		VALUES ($1, 'movie', 'Not an episode', 'matched', '{}'::text[])
	`, movieID); err != nil {
		t.Fatalf("seed movie: %v", err)
	}
}

// seedSupersededProgressCompletions seeds the completed episode 2 rows each
// series case needs, and the unrelated rows of an import-heavy profile.
func seedSupersededProgressCompletions(t *testing.T, ctx context.Context, pool *pgxpool.Pool, userID, otherUserID int, profileID string, episodeID func(seriesIndex, episode int) string, importPrefix string, base time.Time) {
	t.Helper()
	// Series 0: a newer completed rewatch row supersedes episode 1.
	// Series 1: completion predates the candidate and must not supersede it.
	// Series 2: completion is newer but hidden by its watermark.
	// Series 3: equal timestamps do not satisfy the strict freshness gate.
	// Series 5: a regular episode finished later leaves a special in progress.
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_watch_progress
			(user_id, profile_id, media_item_id, position_seconds, duration_seconds, completed, updated_at)
		VALUES
			($1, $2, $3, 300, 1200, TRUE, $8),
			($1, $2, $4, 0, 1200, TRUE, $7),
			($1, $2, $5, 0, 1200, TRUE, $8),
			($1, $2, $6, 0, 1200, TRUE, $7),
			($9, $2, $10, 0, 1200, TRUE, $8),
			($1, $2, $11, 0, 1200, TRUE, $8)
	`, userID, profileID,
		episodeID(0, 2), episodeID(1, 2), episodeID(2, 2), episodeID(3, 2),
		base, base.Add(time.Minute), otherUserID, episodeID(4, 2), episodeID(5, 2)); err != nil {
		t.Fatalf("seed completed progress: %v", err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_history_hidden_items
			(user_id, profile_id, media_item_id, hidden_before)
		VALUES ($1, $2, $3, $4)
	`, userID, profileID, episodeID(2, 2), base.Add(2*time.Minute)); err != nil {
		t.Fatalf("hide completed progress: %v", err)
	}

	// Reproduce an import-heavy profile. These unrelated rows would force the
	// old global walk past its 2,500-row cap, but they are irrelevant to the
	// candidate-driven query.
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_watch_progress
			(user_id, profile_id, media_item_id, position_seconds, duration_seconds, completed, updated_at)
		SELECT $1, $2, $3 || n::text, 0, 1, TRUE, $4
		FROM generate_series(1, 3000) AS n
	`, userID, profileID, importPrefix, base.Add(3*time.Minute)); err != nil {
		t.Fatalf("seed imported completion tail: %v", err)
	}
}
