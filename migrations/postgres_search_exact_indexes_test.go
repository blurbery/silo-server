package migrations

import (
	"os"
	"strings"
	"testing"
)

func TestPostgresSearchExactTitleIndexesAreConcurrentAndRetrySafe(t *testing.T) {
	data, err := os.ReadFile("sql/20260829025159_optimize_postgres_search_exact_titles.sql")
	if err != nil {
		t.Fatalf("read migration: %v", err)
	}
	sql := string(data)
	for _, required := range []string{
		"-- +goose NO TRANSACTION",
		"ADD COLUMN IF NOT EXISTS search_title_normalized text",
		"ADD COLUMN IF NOT EXISTS search_title_vector tsvector",
		"ADD COLUMN IF NOT EXISTS search_overview_vector tsvector",
		"CREATE OR REPLACE FUNCTION public.set_episode_catalog_entry_search_fields()",
		"CREATE TRIGGER trg_episode_catalog_entries_search_fields",
		"still_thumbhash, overview, created_at OR DELETE",
		"UPDATE public.episode_catalog_entries ece",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_media_items_title_normalized_exact",
		"ON public.media_items (title_normalized text_pattern_ops, content_id)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_media_item_aliases_normalized_content",
		"ON public.media_item_aliases (normalized_title text_pattern_ops, content_id)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_episode_catalog_entries_search_title_normalized",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_episode_catalog_entries_search_title",
		"ON public.episode_catalog_entries USING gin (search_title_vector)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_episode_catalog_entries_search_overview",
		"ON public.episode_catalog_entries USING gin (search_overview_vector)",
		"CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_episode_catalog_entries_search_episode",
		"ON public.episode_catalog_entries (episode_id, media_folder_id)",
		"AND NOT i.indisvalid",
	} {
		if !strings.Contains(sql, required) {
			t.Fatalf("migration missing %q:\n%s", required, sql)
		}
	}
	downMarker := strings.Index(sql, "-- +goose Down")
	if downMarker < 0 || !strings.Contains(sql[downMarker:], "still_thumbhash, created_at OR DELETE") {
		t.Fatalf("migration down path does not restore the original episode refresh trigger:\n%s", sql)
	}
}
