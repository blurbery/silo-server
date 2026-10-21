package handlers

import (
	"context"
	"encoding/json"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
)

// adminSubtitleListFixture is one account and one media file with subtitle
// rows inserted through insert. snapshot reads every stored row of the file.
type adminSubtitleListFixture struct {
	pool    *pgxpool.Pool
	account int
	file    int
}

func newAdminSubtitleListFixture(t *testing.T) adminSubtitleListFixture {
	t.Helper()
	dsn := os.Getenv("SILO_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("SILO_TEST_DATABASE_URL is not set")
	}
	pool, err := pgxpool.New(t.Context(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	ctx := t.Context()
	var account, folder, file int
	suffix := uuid.NewString()
	if err = pool.QueryRow(ctx, `INSERT INTO users(username,role) VALUES($1,'user') RETURNING id`, "subtitle-list-"+suffix).Scan(&account); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM users WHERE id=$1`, account) })
	if err = pool.QueryRow(ctx, `INSERT INTO media_folders(type,name,enabled) VALUES('movies',$1,true) RETURNING id`, suffix).Scan(&folder); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM media_folders WHERE id=$1`, folder) })
	if err = pool.QueryRow(ctx, `INSERT INTO media_files(media_folder_id,file_path) VALUES($1,$2) RETURNING id`, folder, "/fixture/"+suffix).Scan(&file); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = pool.Exec(context.Background(), `DELETE FROM media_files WHERE id=$1`, file) })
	t.Cleanup(func() {
		_, _ = pool.Exec(context.Background(), `DELETE FROM downloaded_subtitles WHERE media_file_id=$1`, file)
	})
	return adminSubtitleListFixture{pool: pool, account: account, file: file}
}

func (f adminSubtitleListFixture) insert(t *testing.T, provider, language string, at time.Time) int {
	t.Helper()
	var id int
	if err := f.pool.QueryRow(t.Context(), `INSERT INTO downloaded_subtitles(media_file_id,provider,language,format,release_name,s3_key,downloaded_by,created_at) VALUES($1,$2,$3,'srt','Release Example',$4,$5,$6) RETURNING id`, f.file, provider, language, "private-object-"+uuid.NewString(), f.account, at).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f adminSubtitleListFixture) snapshot(t *testing.T) string {
	t.Helper()
	var s string
	if err := f.pool.QueryRow(t.Context(), `SELECT jsonb_agg(to_jsonb(ds) ORDER BY id)::text FROM downloaded_subtitles ds WHERE media_file_id=$1`, f.file).Scan(&s); err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAdminSubtitleListPageDB(t *testing.T) {
	fx := newAdminSubtitleListFixture(t)
	ctx := t.Context()
	timestamp := time.Date(2026, 1, 2, 3, 4, 5, 123456000, time.UTC)
	ids := []int{fx.insert(t, "upload", "en", timestamp), fx.insert(t, "provider", "fr", timestamp), fx.insert(t, "provider", "de", timestamp)}
	before := fx.snapshot(t)
	h := &AdminSubtitleHandler{pool: fx.pool}
	filter := AdminSubtitleListFilter{MediaFileID: fx.file}
	first, err := h.ListAdminSubtitlesPage(ctx, filter, nil, 2)
	if err != nil {
		t.Fatal(err)
	}
	if first.Total != 3 || first.Uploads != 1 || first.ProviderDownloads != 2 || !first.HasMore || len(first.Items) != 2 || first.Items[0].ID != ids[2] || first.Items[1].ID != ids[1] {
		t.Fatalf("first: %+v", first)
	}
	last := first.Items[1]
	after := &AdminSubtitlePageKey{ID: last.ID, CreatedAt: last.CreatedAt}
	second, err := h.ListAdminSubtitlesPage(ctx, filter, after, 2)
	if err != nil || second.HasMore || len(second.Items) != 1 || second.Items[0].ID != ids[0] || second.Total != 3 {
		t.Fatalf("second: %+v %v", second, err)
	}
	checkAdminSubtitleListFilters(t, h, fx)
	if fx.snapshot(t) != before {
		t.Fatal("listing changed persisted rows")
	}
	checkAdminSubtitleListLegacyLanguage(t, h, fx, timestamp)
	// A later insert sorts before the cursor and must not repeat an old page.
	fx.insert(t, "provider", "es", timestamp.Add(time.Second))
	second, err = h.ListAdminSubtitlesPage(ctx, filter, after, 2)
	if err != nil || len(second.Items) != 1 || second.Items[0].ID != ids[0] || second.Total != 4 {
		t.Fatalf("live insert: %+v %v", second, err)
	}
	// A deleted cursor row still identifies the same strict ordering boundary.
	if _, err = fx.pool.Exec(ctx, `DELETE FROM downloaded_subtitles WHERE id=$1`, last.ID); err != nil {
		t.Fatal(err)
	}
	next, err := h.ListAdminSubtitlesPage(ctx, filter, after, 2)
	if err != nil || !reflect.DeepEqual(next.Items, second.Items) {
		t.Fatalf("deleted cursor: %+v %v", next, err)
	}
	encoded, _ := json.Marshal(next.Items)
	if string(encoded) == "null" {
		t.Fatal("null items")
	}
	t.Logf("equal-time ordering, filters, live insert/deleted cursor, full row snapshot unchanged; file %d", fx.file)
}

// checkAdminSubtitleListFilters checks the provider, language and search
// filters, and that a page with no match still lists an empty slice.
func checkAdminSubtitleListFilters(t *testing.T, h *AdminSubtitleHandler, fx adminSubtitleListFixture) {
	t.Helper()
	ctx := t.Context()
	for _, f := range []AdminSubtitleListFilter{{MediaFileID: fx.file, Provider: "upload"}, {MediaFileID: fx.file, Language: "en"}, {MediaFileID: fx.file, UserID: fx.account, Search: "EXAMPLE"}} {
		page, err := h.ListAdminSubtitlesPage(ctx, f, nil, 200)
		want := 1
		if f.Search != "" {
			want = 3
		}
		if err != nil || page.Total != want || len(page.Items) != want {
			t.Fatalf("filtered %+v: %+v %v", f, page, err)
		}
	}
	empty, err := h.ListAdminSubtitlesPage(ctx, AdminSubtitleListFilter{MediaFileID: fx.file, Search: "missing"}, nil, 2)
	if err != nil || empty.Items == nil || len(empty.Items) != 0 || empty.Total != 0 {
		t.Fatalf("empty: %+v %v", empty, err)
	}
}

// checkAdminSubtitleListLegacyLanguage checks that a provider row stored with
// a legacy language spelling matches and lists by its canonical code without
// rewriting the stored row, then removes it.
func checkAdminSubtitleListLegacyLanguage(t *testing.T, h *AdminSubtitleHandler, fx adminSubtitleListFixture, at time.Time) {
	t.Helper()
	ctx := t.Context()
	legacyID := fx.insert(t, "subdl", " English ", at)
	legacySnapshot := fx.snapshot(t)
	page, err := h.ListAdminSubtitlesPage(ctx, AdminSubtitleListFilter{MediaFileID: fx.file, Provider: "subdl", Language: "en"}, nil, 200)
	if err != nil || page.Total != 1 || len(page.Items) != 1 || page.Items[0].ID != legacyID || page.Items[0].Language != "en" {
		t.Fatalf("legacy provider language filter: %+v %v", page, err)
	}
	if fx.snapshot(t) != legacySnapshot {
		t.Fatal("legacy language filter changed stored rows")
	}
	if _, err = fx.pool.Exec(ctx, `DELETE FROM downloaded_subtitles WHERE id=$1`, legacyID); err != nil {
		t.Fatal(err)
	}
}
