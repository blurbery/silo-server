package pgstore

import (
	"context"
	"fmt"
	"os"
	"slices"
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
	seriesIDs := []string{id("newer"), id("older"), id("hidden"), id("equal"), id("other-user")}
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

	base := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	// Series 0: a newer completed rewatch row supersedes episode 1.
	// Series 1: completion predates the candidate and must not supersede it.
	// Series 2: completion is newer but hidden by its watermark.
	// Series 3: equal timestamps do not satisfy the strict freshness gate.
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_watch_progress
			(user_id, profile_id, media_item_id, position_seconds, duration_seconds, completed, updated_at)
		VALUES
			($1, $2, $3, 300, 1200, TRUE, $8),
			($1, $2, $4, 0, 1200, TRUE, $7),
			($1, $2, $5, 0, 1200, TRUE, $8),
			($1, $2, $6, 0, 1200, TRUE, $7),
			($9, $2, $10, 0, 1200, TRUE, $8)
	`, userID, profileID,
		episodeID(0, 2), episodeID(1, 2), episodeID(2, 2), episodeID(3, 2),
		base, base.Add(time.Minute), otherUserID, episodeID(4, 2)); err != nil {
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
	`, userID, profileID, id("imported-"), base.Add(3*time.Minute)); err != nil {
		t.Fatalf("seed imported completion tail: %v", err)
	}

	candidates := []userstore.SupersededEpisodeCandidate{
		{MediaItemID: episodeID(0, 1), UpdatedAt: base},
		{MediaItemID: episodeID(1, 1), UpdatedAt: base.Add(time.Minute)},
		{MediaItemID: episodeID(2, 1), UpdatedAt: base},
		{MediaItemID: episodeID(3, 1), UpdatedAt: base},
		{MediaItemID: episodeID(4, 1), UpdatedAt: base},
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

func TestListProgressByMediaItemsReusedPlan(t *testing.T) {
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	config, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	// Exercise the plan used after repeated browse requests, including when
	// the same prepared statement receives a different number of item IDs.
	config.ConnConfig.RuntimeParams["plan_cache_mode"] = "force_generic_plan"
	config.MaxConns = 1
	ctx := t.Context()
	pool, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	userIDs := make([]int, 0, 2)
	t.Cleanup(func() {
		_, err := pool.Exec(context.WithoutCancel(ctx), `DELETE FROM users WHERE id = ANY($1)`, userIDs)
		if err != nil {
			t.Errorf("clean up users: %v", err)
		}
	})
	for i := range 2 {
		var id int
		if err := pool.QueryRow(ctx, `INSERT INTO users (username, role) VALUES ($1, 'user') RETURNING id`,
			fmt.Sprintf("progress-array-%d-%d", time.Now().UnixNano(), i)).Scan(&id); err != nil {
			t.Fatal(err)
		}
		userIDs = append(userIDs, id)
	}
	const quotedID = "episode-'quoted'\\path"
	at := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_watch_progress
			(user_id, profile_id, media_item_id, position_seconds, duration_seconds, completed, updated_at)
		VALUES ($1, 'a', $3, 30, 120, FALSE, $4),
		       ($1, 'a', 'hidden', 40, 120, FALSE, $4),
		       ($1, 'a', 'newer', 120, 120, TRUE, $4),
		       ($1, 'a', 'unrequested', 50, 120, FALSE, $4),
		       ($1, 'b', 'profile-only', 60, 120, FALSE, $4),
		       ($2, 'a', 'account-only', 70, 120, FALSE, $4)
	`, userIDs[0], userIDs[1], quotedID, at); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `
		INSERT INTO user_history_hidden_items (user_id, profile_id, media_item_id, hidden_before)
		VALUES ($1, 'a', 'hidden', $2), ($1, 'a', 'newer', $3)
	`, userIDs[0], at, at.Add(-time.Second)); err != nil {
		t.Fatal(err)
	}
	store := newStore(pool, userIDs[0])
	ids := []string{quotedID, quotedID, "hidden", "newer", "profile-only", "account-only", "missing"}
	largeIDs := slices.Clone(ids)
	for i := range 5000 {
		largeIDs = append(largeIDs, fmt.Sprintf("missing-%d", i))
	}
	for _, batch := range [][]string{ids, largeIDs, ids, nil, {}} {
		got, err := store.ListProgressByMediaItems(ctx, "a", batch)
		if err != nil {
			t.Fatalf("batch size %d: %v", len(batch), err)
		}
		if len(batch) == 0 {
			if len(got) != 0 {
				t.Fatalf("empty batch returned %d items", len(got))
			}
			continue
		}
		if len(got) != 2 || got[quotedID].PositionSeconds != 30 || !got["newer"].Completed {
			t.Fatalf("batch size %d: unexpected progress: %+v", len(batch), got)
		}
	}
}
