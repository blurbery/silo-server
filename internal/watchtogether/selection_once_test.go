package watchtogether

import (
	"context"
	"errors"
	"os"
	"runtime"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

func selectionPG(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	cfg, err := pgxpool.ParseConfig(dsn)
	if err != nil {
		t.Fatal(err)
	}
	schema := "selection_" + uuid.NewString()
	ident := pgx.Identifier{schema}.Sanitize()
	admin, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = admin.Exec(t.Context(), "CREATE SCHEMA "+ident); err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = schema
	cfg.AfterConnect = func(ctx context.Context, c *pgx.Conn) error {
		_, err := c.Exec(ctx, "SET search_path TO "+ident)
		return err
	}
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+ident+" CASCADE")
		admin.Close()
	})
	_, err = pool.Exec(t.Context(), `CREATE TABLE watch_together_rooms (
 id text PRIMARY KEY, code text, join_token text, host_user_id integer, host_profile_id text,
 phase text, playback_state text, resume_on_ready boolean, selection_mode text, selection_revision bigint,
 selected_content_id text, selected_file_id integer, selected_library_id integer, guest_control_policy text,
 anchor_position_seconds double precision, is_paused boolean, anchor_updated_at timestamptz,
 generation bigint, created_at timestamptz, closed_at timestamptz)`)
	if err != nil {
		t.Fatal(err)
	}
	return pool
}

func TestSelectionOncePreservesAttachedReadinessPG(t *testing.T) {
	pool := selectionPG(t)
	repo := NewRepository(pool)
	now := time.Now().UTC()
	room := baseRoom(now)
	if _, err := repo.CreateRoom(t.Context(), room); err != nil {
		t.Fatal(err)
	}
	s := newServiceForTest(now, &stubRepo{room: room}, nil, nil, &stubSelectionResolver{resolved: &ResolvedSelection{ContentID: "movie", FileID: new(7), LibraryID: new(8)}})
	s.repo = repo
	conn := new(recordingConn)
	member := &memberState{userID: 7, profileID: "host", connection: conn}
	s.rooms[room.ID].members["host"] = member
	first, err := s.SelectItemOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	member.sessionID = "new-session"
	member.isReady = true
	member.isBuffering = true
	member.ignoreWait = true
	before := len(conn.payloads)
	second, err := s.SelectItemOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "movie"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Generation != first.Generation || second.SelectionRevision != first.SelectionRevision || second.AnchorUpdatedAt != first.AnchorUpdatedAt || member.sessionID != "new-session" || !member.isReady || !member.isBuffering || !member.ignoreWait || len(conn.payloads) != before {
		t.Fatal("identical selection reset state or broadcast")
	}
	// A separate service has stale local state but must still resolve the same DB identity as a no-op.
	other := newServiceForTest(now, &stubRepo{room: room}, nil, nil, s.selectionResolver)
	other.repo = repo
	third, err := other.SelectItemOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "movie"})
	if err != nil || third.Generation != first.Generation {
		t.Fatalf("stale node: %+v %v", third, err)
	}
	// A genuinely different resolved selection still resets prior readiness once.
	s.selectionResolver = &stubSelectionResolver{resolved: &ResolvedSelection{ContentID: "next", FileID: new(9), LibraryID: new(8)}}
	changed, err := s.SelectItemOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "next"})
	if err != nil || changed.SelectionRevision != first.SelectionRevision+1 || changed.Generation != first.Generation+1 || member.sessionID != "" || member.isReady || member.isBuffering || member.ignoreWait || len(conn.payloads) != before+1 {
		t.Fatalf("new selection reset: %+v %v", changed, err)
	}

	for _, authority := range []struct {
		user    int
		profile string
	}{{8, "host"}, {7, "guest"}} {
		if _, err := s.SelectItemOnce(t.Context(), room.ID, authority.user, authority.profile, SelectItemInput{ContentID: "movie"}); !errors.Is(err, ErrRoomForbidden) {
			t.Fatalf("authority: %v", err)
		}
	}
	for _, tc := range []struct {
		phase, mode string
		want        error
	}{{"playing", "vote", ErrVoteRoomSelection}, {"ended", "host_pick", ErrRoomClosed}} {
		if _, err := pool.Exec(t.Context(), `UPDATE watch_together_rooms SET phase=$2,selection_mode=$3 WHERE id=$1`, room.ID, tc.phase, tc.mode); err != nil {
			t.Fatal(err)
		}
		if _, _, err := repo.SelectOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "next", FileID: new(9), LibraryID: new(8)}, false, changed.Generation, now); !errors.Is(err, tc.want) {
			t.Fatalf("no-op refusal: %v", err)
		}
	}

}

func TestSelectionOnceWaitsForCommittedIdentityPG(t *testing.T) {
	pool := selectionPG(t)
	repo := NewRepository(pool)
	now := time.Now().UTC()
	room := baseRoom(now)
	if _, err := repo.CreateRoom(t.Context(), room); err != nil {
		t.Fatal(err)
	}
	tx, err := pool.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback(t.Context()) }()
	_, err = tx.Exec(t.Context(), `UPDATE watch_together_rooms SET selected_content_id='movie',selected_file_id=7,selected_library_id=8,selection_revision=9,generation=20,anchor_position_seconds=123,is_paused=false WHERE id=$1`, room.ID)
	if err != nil {
		t.Fatal(err)
	}
	type receipt struct {
		room    *Room
		applied bool
		err     error
	}
	done := make(chan receipt, 1)
	go func() {
		r, a, e := repo.SelectOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "movie", FileID: new(7), LibraryID: new(8)}, false, room.Generation, now)
		done <- receipt{r, a, e}
	}()
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	for {
		var blocked bool
		err = pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=current_setting('application_name') AND wait_event_type='Lock')`).Scan(&blocked)
		if err != nil {
			t.Fatal(err)
		}
		if blocked {
			break
		}
		runtime.Gosched()
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case r := <-done:
		if r.err != nil || r.applied || r.room.Generation != 20 || r.room.SelectionRevision != 9 || r.room.AnchorPositionSeconds != 123 || r.room.IsPaused {
			t.Fatalf("locked no-op: %+v %v %v", r.room, r.applied, r.err)
		}
	case <-ctx.Done():
		t.Fatal(ctx.Err())
	}
	// Different selection with an old generation returns the winner without resetting it.
	r, applied, err := repo.SelectOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "different"}, false, room.Generation, now)
	if err != nil || applied || r.Generation != 20 || *r.SelectedContentID != "movie" {
		t.Fatalf("conflict %v %v %+v", err, applied, r)
	}
}
