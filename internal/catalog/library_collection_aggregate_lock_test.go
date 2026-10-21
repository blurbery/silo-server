package catalog

import (
	"context"
	"fmt"
	"os"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/Silo-Server/silo-server/internal/models"
)

// Pause the first transaction after it changes its first library counter. The
// opposing writer must block without retaining the other counter. Statement-
// local trigger ordering alone deadlocks here: each transaction retains its
// first library counter while trying to acquire the other's second counter.
func TestLibraryCollectionAggregateTransactionLockOrderDB(t *testing.T) {
	for _, operation := range []string{"swap-legacy", "swap-guarded", "create-opposite-order"} {
		t.Run(operation, func(t *testing.T) {
			first, second := newLibraryCASFixture(t), newLibraryCASFixture(t)
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			trigger := fmt.Sprintf("test_library_aggregate_%d", first.libraryID)
			firstName, secondName := trigger+"_first", trigger+"_second"
			firstRepo, secondRepo := namedLibraryCollectionRepository(t, ctx, firstName), namedLibraryCollectionRepository(t, ctx, secondName)
			firstRevision, secondRevision := first.revision(t, false), second.revision(t, false)
			gate, err := first.pool.Begin(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = gate.Rollback(context.Background()) }()
			key := int64(860000000000) + int64(first.libraryID)
			if _, err := gate.Exec(ctx, `SELECT pg_advisory_xact_lock($1)`, key); err != nil {
				t.Fatal(err)
			}
			sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF current_setting('application_name')='%s' THEN PERFORM pg_advisory_xact_lock(%d::bigint); END IF; RETURN NEW; END; $$; CREATE TRIGGER %s AFTER UPDATE ON library_collection_order_revisions FOR EACH ROW WHEN (NEW.library_id=%d) EXECUTE FUNCTION %s();`, trigger, firstName, key, trigger, first.libraryID, trigger)
			if _, err := first.pool.Exec(ctx, sql); err != nil {
				t.Fatal(err)
			}
			defer func() {
				_, _ = first.pool.Exec(context.Background(), fmt.Sprintf(`DROP TRIGGER %s ON library_collection_order_revisions; DROP FUNCTION %s();`, trigger, trigger))
			}()
			run := func(repo *LibraryCollectionRepository, fixture libraryCASFixture, target int, revision int64) libraryAggregateResult {
				if operation == "create-opposite-order" {
					c, err := repo.Create(ctx, CreateLibraryCollectionInput{LibraryIDs: []int{fixture.libraryID, target}, Slug: fmt.Sprintf("opposite-%d", fixture.libraryID), Title: "opposite"})
					return libraryAggregateResult{c, err}
				}
				in := UpdateLibraryCollectionInput{ID: fixture.collectionID, LibraryIDs: new([]int{target})}
				if operation == "swap-guarded" {
					in.ExpectedRevision = &revision
				}
				return libraryAggregateResult{err: repo.Update(ctx, in)}
			}
			firstDone, secondDone := make(chan libraryAggregateResult, 1), make(chan libraryAggregateResult, 1)
			go func() { firstDone <- run(firstRepo, first, second.libraryID, firstRevision) }()
			waitLibraryCollectionBlocked(t, ctx, first.pool, firstName, "advisory")
			go func() { secondDone <- run(secondRepo, second, first.libraryID, secondRevision) }()
			waitLibraryCollectionBlocked(t, ctx, first.pool, secondName, "transactionid")
			if err := gate.Commit(ctx); err != nil {
				t.Fatal(err)
			}
			firstResult, secondResult := <-firstDone, <-secondDone
			for _, r := range []libraryAggregateResult{firstResult, secondResult} {
				if r.collection != nil {
					t.Cleanup(func() {
						_ = first.repo.Delete(context.Background(), r.collection.ID)
						_, _ = first.pool.Exec(context.Background(), `DELETE FROM library_collection_revisions WHERE collection_id=$1`, r.collection.ID)
					})
				}
			}
			if firstResult.err != nil || secondResult.err != nil {
				t.Fatalf("first=%v second=%v", firstResult.err, secondResult.err)
			}
			if operation == "create-opposite-order" {
				checkLibraryAggregateCreates(t, first, second, firstResult, secondResult)
			} else {
				checkLibraryAggregateSwaps(t, ctx, first, second)
			}
		})
	}
}

type libraryAggregateResult struct {
	collection *models.LibraryCollection
	err        error
}

// namedLibraryCollectionRepository opens a one-connection pool whose sessions
// carry name as their application_name, so a trigger can pause only them.
func namedLibraryCollectionRepository(t *testing.T, ctx context.Context, name string) *LibraryCollectionRepository {
	t.Helper()
	cfg, err := pgxpool.ParseConfig(os.Getenv("SILO_TEST_DATABASE_URL"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.ConnConfig.RuntimeParams["application_name"] = name
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return NewLibraryCollectionRepository(pool)
}

// checkLibraryAggregateCreates checks that both opposite-order creates kept
// the full scope and their submitted primary library.
func checkLibraryAggregateCreates(t *testing.T, first, second libraryCASFixture, results ...libraryAggregateResult) {
	t.Helper()
	for i, r := range results {
		if !slices.Equal(r.collection.LibraryIDs, []int{first.libraryID, second.libraryID}) {
			t.Fatalf("create scope=%v", r.collection.LibraryIDs)
		}
		wantLegacy := first.libraryID
		if i == 1 {
			wantLegacy = second.libraryID
		}
		if r.collection.LibraryID != wantLegacy {
			t.Fatalf("submitted primary library changed: got%d want%d", r.collection.LibraryID, wantLegacy)
		}
	}
}

// checkLibraryAggregateSwaps checks that each swapped collection now belongs
// to the other fixture's library.
func checkLibraryAggregateSwaps(t *testing.T, ctx context.Context, first, second libraryCASFixture) {
	t.Helper()
	for _, pair := range []struct {
		fixture libraryCASFixture
		want    int
	}{{first, second.libraryID}, {second, first.libraryID}} {
		c, err := pair.fixture.repo.GetByID(ctx, pair.fixture.collectionID)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(c.LibraryIDs, []int{pair.want}) {
			t.Fatalf("swap scope=%v want%d", c.LibraryIDs, pair.want)
		}
	}
}

type libraryCASFixture struct {
	pool         *pgxpool.Pool
	repo         *LibraryCollectionRepository
	groups       *LibraryCollectionGroupRepository
	libraryID    int
	collectionID string
	groupID      string
}

func newLibraryCASFixture(t *testing.T) libraryCASFixture {
	t.Helper()
	pool := collectionSortTestPool(t)
	ctx := t.Context()
	var libraryID int
	if err := pool.QueryRow(ctx, `INSERT INTO media_folders(type,name,enabled) VALUES('movies',$1,true) RETURNING id`, fmt.Sprintf("library-cas-%d", time.Now().UnixNano())).Scan(&libraryID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM media_folders WHERE id=$1`, libraryID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM library_collection_order_revisions WHERE library_id=$1`, libraryID)
	})
	repo := NewLibraryCollectionRepository(pool)
	groups := NewLibraryCollectionGroupRepository(pool)
	c, err := repo.Create(ctx, CreateLibraryCollectionInput{LibraryID: libraryID, Title: "first", Slug: fmt.Sprintf("cas-%d", libraryID)})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = repo.Delete(context.Background(), c.ID)
		_, _ = pool.Exec(context.Background(), `DELETE FROM library_collection_revisions WHERE collection_id=$1`, c.ID)
	})
	g, err := groups.Create(ctx, CreateLibraryCollectionGroupInput{LibraryID: libraryID, Name: "first"})
	if err != nil {
		t.Fatal(err)
	}
	return libraryCASFixture{pool, repo, groups, libraryID, c.ID, g.ID}
}
func (f libraryCASFixture) revision(t *testing.T, group bool) int64 {
	t.Helper()
	var rev int64
	var err error
	if group {
		rev, err = f.repo.CollectionOrderRevision(t.Context(), f.libraryID)
	} else {
		rev, err = f.repo.CollectionRevision(t.Context(), f.collectionID)
	}
	if err != nil {
		t.Fatal(err)
	}
	return rev
}

func waitLibraryCollectionBlocked(t *testing.T, ctx context.Context, pool *pgxpool.Pool, name, event string) {
	t.Helper()
	ticker := time.NewTicker(5 * time.Millisecond)
	defer ticker.Stop()
	for {
		var waiting bool
		if err := pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM pg_stat_activity WHERE application_name=$1 AND wait_event=$2)`, name, event).Scan(&waiting); err != nil {
			t.Fatal(err)
		}
		if waiting {
			return
		}
		select {
		case <-ctx.Done():
			t.Fatalf("writer %s did not wait on %s: %v", name, event, ctx.Err())
		case <-ticker.C:
		}
	}
}
