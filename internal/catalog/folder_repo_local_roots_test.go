package catalog

import (
	"context"
	"fmt"
	"os"
	"sort"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestLibraryRootsForContent(t *testing.T) {
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
	contentID := fmt.Sprintf("roots-test-%d", suffix)
	var folderID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO media_folders (type, name, enabled) VALUES ('movies', $1, true) RETURNING id`,
		fmt.Sprintf("roots-test-%d", suffix)).Scan(&folderID); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM media_item_libraries WHERE content_id = $1`, contentID)
		_, _ = pool.Exec(ctx, `DELETE FROM media_items WHERE content_id = $1`, contentID)
		_, _ = pool.Exec(ctx, `DELETE FROM media_folder_paths WHERE media_folder_id = $1`, folderID)
		_, _ = pool.Exec(ctx, `DELETE FROM media_folders WHERE id = $1`, folderID)
	})
	if _, err := pool.Exec(ctx,
		`INSERT INTO media_items (content_id, type, title, status, genres) VALUES ($1, 'movie', 'Roots Test', 'matched', '{}'::text[])`,
		contentID); err != nil {
		t.Fatalf("seed item: %v", err)
	}
	for _, path := range []string{"/media/movies-a", "/media/movies-b"} {
		if _, err := pool.Exec(ctx,
			`INSERT INTO media_folder_paths (media_folder_id, path) VALUES ($1, $2)`,
			folderID, fmt.Sprintf("%s-%d", path, suffix)); err != nil {
			t.Fatalf("seed folder path: %v", err)
		}
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO media_item_libraries (content_id, media_folder_id) VALUES ($1, $2)`,
		contentID, folderID); err != nil {
		t.Fatalf("seed membership: %v", err)
	}

	repo := NewFolderRepository(pool)
	roots, err := repo.LibraryRootsForContent(ctx, contentID)
	if err != nil {
		t.Fatalf("LibraryRootsForContent: %v", err)
	}
	sort.Strings(roots)
	want := []string{
		fmt.Sprintf("/media/movies-a-%d", suffix),
		fmt.Sprintf("/media/movies-b-%d", suffix),
	}
	if len(roots) != 2 || roots[0] != want[0] || roots[1] != want[1] {
		t.Fatalf("roots = %v, want %v", roots, want)
	}

	// Unknown content has no roots (and no error).
	none, err := repo.LibraryRootsForContent(ctx, contentID+"-missing")
	if err != nil || len(none) != 0 {
		t.Fatalf("missing content: roots=%v err=%v", none, err)
	}
}

func TestFoldersHoldingContentMedia(t *testing.T) {
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
	contentID := fmt.Sprintf("media-folders-test-%d", suffix)
	var folderID int
	if err := pool.QueryRow(ctx,
		`INSERT INTO media_folders (type, name, enabled) VALUES ('series', $1, true) RETURNING id`,
		fmt.Sprintf("media-folders-test-%d", suffix)).Scan(&folderID); err != nil {
		t.Fatalf("seed folder: %v", err)
	}
	t.Cleanup(func() {
		_, _ = pool.Exec(ctx, `DELETE FROM media_files WHERE media_folder_id = $1`, folderID)
		_, _ = pool.Exec(ctx, `DELETE FROM media_folders WHERE id = $1`, folderID)
	})
	root := fmt.Sprintf("/media/series-%d", suffix)
	show := root + "/Show"
	season := show + "/Season 01"
	gone := root + "/Gone"
	for _, file := range []struct {
		contentID string
		path      string
		missing   bool
	}{
		{contentID, season + "/Show S01E01.mkv", false},
		{contentID, gone + "/Show S02E01.mkv", true},
		{contentID + "-other", root + "/Other/Other S01E01.mkv", false},
	} {
		var missingSince any
		if file.missing {
			missingSince = time.Now()
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO media_files (content_id, media_folder_id, file_path, missing_since) VALUES ($1, $2, $3, $4)`,
			file.contentID, folderID, file.path, missingSince); err != nil {
			t.Fatalf("seed media file: %v", err)
		}
	}

	repo := NewFolderRepository(pool)
	held, err := repo.FoldersHoldingContentMedia(ctx, contentID, []string{
		season,
		show,
		show + "/Season",    // a name prefix of a folder that holds media, not a parent
		gone,                // only a missing file
		root + "/Other",     // another item's media
		show + "/Season 02", // nothing
	})
	if err != nil {
		t.Fatalf("FoldersHoldingContentMedia: %v", err)
	}
	sort.Strings(held)
	if want := []string{show, season}; len(held) != 2 || held[0] != want[0] || held[1] != want[1] {
		t.Fatalf("held = %v, want %v", held, want)
	}

	none, err := repo.FoldersHoldingContentMedia(ctx, contentID, nil)
	if err != nil || len(none) != 0 {
		t.Fatalf("no candidates: held=%v err=%v", none, err)
	}
}
