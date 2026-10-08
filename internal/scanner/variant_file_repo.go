package scanner

import (
	"context"
	"fmt"

	"github.com/Silo-Server/silo-server/internal/pathscope"
	"github.com/jackc/pgx/v5"
)

// VariantFile is the narrow media_files projection that variant finalization
// reads and writes: the row identity, its owner, its path and the edition,
// presentation and multi-episode columns derived from that path. It has no
// probe, track, chapter or identity fields on purpose, so a finalization row
// can never reach Upsert and blank the columns it does not carry.
type VariantFile struct {
	ID                    int
	ContentID             string
	EpisodeID             string
	FilePath              string
	EditionRaw            string
	EditionKey            string
	EditionConfidence     *float64
	EditionSource         string
	PresentationKind      string
	PresentationGroupKey  string
	PresentationPartIndex int
	PresentationPartTotal int
	MultiEpisodeStart     int
	MultiEpisodeEnd       int
}

const variantFileColumns = `id, content_id, episode_id, file_path,
	edition_raw, edition_key, edition_confidence, edition_source,
	presentation_kind, presentation_group_key, presentation_part_index, presentation_part_total,
	multi_episode_start, multi_episode_end`

const (
	// variantOwnerLookupChunk caps the owner ids sent in one query.
	variantOwnerLookupChunk = 5000
	// variantUpdateChunk caps the rows written by one UPDATE statement.
	variantUpdateChunk = 1000
)

// ListVariantFilesByPathPrefix returns the present files of a media folder at
// or under pathPrefix in the narrow variant projection. The range form of the
// path predicate keeps the folder/path index usable under a generic plan.
func (r *FileRepository) ListVariantFilesByPathPrefix(ctx context.Context, folderID int, pathPrefix string) ([]VariantFile, error) {
	clauses, clauseArgs := pathscope.RangeCoverageClauses("file_path", []string{pathPrefix}, 2)
	query := `SELECT ` + variantFileColumns + ` FROM media_files
		WHERE media_folder_id = $1
		  AND missing_since IS NULL
		  AND ` + clauses[0]
	args := append([]any{folderID}, clauseArgs...)
	rows, err := r.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("querying variant files by path prefix: %w", err)
	}
	return scanVariantFiles(rows)
}

// ListVariantFilesByOwners returns the present files of a media folder owned
// by any of the given episodes, or by any of the given content ids without an
// episode link (the owner rule stableOwnerKey applies). Ids are queried in
// chunks and the result is deduplicated by file id.
func (r *FileRepository) ListVariantFilesByOwners(ctx context.Context, folderID int, episodeIDs, contentIDs []string) ([]VariantFile, error) {
	var files []VariantFile
	seen := make(map[int]struct{})
	lookup := func(predicate string, ids []string) error {
		for start := 0; start < len(ids); start += variantOwnerLookupChunk {
			chunk := ids[start:min(start+variantOwnerLookupChunk, len(ids))]
			rows, err := r.pool.Query(ctx, `SELECT `+variantFileColumns+` FROM media_files
				WHERE media_folder_id = $1
				  AND missing_since IS NULL
				  AND `+predicate, folderID, chunk)
			if err != nil {
				return fmt.Errorf("querying variant files by owner: %w", err)
			}
			found, err := scanVariantFiles(rows)
			if err != nil {
				return err
			}
			for _, file := range found {
				if _, ok := seen[file.ID]; ok {
					continue
				}
				seen[file.ID] = struct{}{}
				files = append(files, file)
			}
		}
		return nil
	}
	if err := lookup(`episode_id = ANY($2::text[])`, episodeIDs); err != nil {
		return nil, err
	}
	if err := lookup(`episode_id IS NULL AND content_id = ANY($2::text[])`, contentIDs); err != nil {
		return nil, err
	}
	return files, nil
}

// UpdateVariantMetadata writes only the edition, presentation and
// multi-episode columns (and updated_at) of the given files, in batches. A row
// is skipped when it has gone missing or moved to another path since it was
// loaded, or already holds these values. Returns the number of rows changed.
func (r *FileRepository) UpdateVariantMetadata(ctx context.Context, folderID int, files []VariantFile) (int, error) {
	updated := 0
	for start := 0; start < len(files); start += variantUpdateChunk {
		chunk := files[start:min(start+variantUpdateChunk, len(files))]
		ids := make([]int32, len(chunk))
		paths := make([]string, len(chunk))
		editionRaw := make([]string, len(chunk))
		editionKey := make([]string, len(chunk))
		editionConfidence := make([]*float64, len(chunk))
		editionSource := make([]string, len(chunk))
		presentationKind := make([]string, len(chunk))
		presentationGroupKey := make([]string, len(chunk))
		partIndex := make([]*int32, len(chunk))
		partTotal := make([]*int32, len(chunk))
		multiStart := make([]*int32, len(chunk))
		multiEnd := make([]*int32, len(chunk))
		for i, file := range chunk {
			ids[i] = int32(file.ID)
			paths[i] = file.FilePath
			editionRaw[i] = file.EditionRaw
			editionKey[i] = file.EditionKey
			editionConfidence[i] = file.EditionConfidence
			editionSource[i] = file.EditionSource
			presentationKind[i] = file.PresentationKind
			presentationGroupKey[i] = file.PresentationGroupKey
			partIndex[i] = nullableVariantInt(file.PresentationPartIndex)
			partTotal[i] = nullableVariantInt(file.PresentationPartTotal)
			multiStart[i] = nullableVariantInt(file.MultiEpisodeStart)
			multiEnd[i] = nullableVariantInt(file.MultiEpisodeEnd)
		}
		tag, err := r.pool.Exec(ctx, `
			UPDATE media_files AS mf SET
				edition_raw = u.edition_raw,
				edition_key = u.edition_key,
				edition_confidence = u.edition_confidence,
				edition_source = u.edition_source,
				presentation_kind = u.presentation_kind,
				presentation_group_key = u.presentation_group_key,
				presentation_part_index = u.presentation_part_index,
				presentation_part_total = u.presentation_part_total,
				multi_episode_start = u.multi_episode_start,
				multi_episode_end = u.multi_episode_end,
				updated_at = NOW()
			FROM unnest(
				$2::integer[], $3::text[], $4::text[], $5::text[], $6::double precision[], $7::text[],
				$8::text[], $9::text[], $10::integer[], $11::integer[], $12::integer[], $13::integer[]
			) AS u(id, file_path, edition_raw, edition_key, edition_confidence, edition_source,
				presentation_kind, presentation_group_key, presentation_part_index, presentation_part_total,
				multi_episode_start, multi_episode_end)
			WHERE mf.id = u.id
			  AND mf.media_folder_id = $1
			  AND mf.file_path = u.file_path
			  AND mf.missing_since IS NULL
			  AND (mf.edition_raw, mf.edition_key, mf.edition_confidence, mf.edition_source,
				mf.presentation_kind, mf.presentation_group_key, mf.presentation_part_index, mf.presentation_part_total,
				mf.multi_episode_start, mf.multi_episode_end)
				IS DISTINCT FROM
				(u.edition_raw, u.edition_key, u.edition_confidence, u.edition_source,
				u.presentation_kind, u.presentation_group_key, u.presentation_part_index, u.presentation_part_total,
				u.multi_episode_start, u.multi_episode_end)`,
			folderID, ids, paths, editionRaw, editionKey, editionConfidence, editionSource,
			presentationKind, presentationGroupKey, partIndex, partTotal, multiStart, multiEnd)
		if err != nil {
			return updated, fmt.Errorf("updating variant metadata: %w", err)
		}
		updated += int(tag.RowsAffected())
	}
	return updated, nil
}

// nullableVariantInt stores an unset part or episode number as NULL, as
// Upsert does.
func nullableVariantInt(n int) *int32 {
	if n == 0 {
		return nil
	}
	v := int32(n)
	return &v
}

func scanVariantFiles(rows pgx.Rows) ([]VariantFile, error) {
	defer rows.Close()
	var files []VariantFile
	for rows.Next() {
		var (
			file                                       VariantFile
			contentID, episodeID                       *string
			editionRaw, editionKey, editionSource      *string
			presentationKind, presentationGroupKey     *string
			partIndex, partTotal, multiStart, multiEnd *int
		)
		if err := rows.Scan(
			&file.ID, &contentID, &episodeID, &file.FilePath,
			&editionRaw, &editionKey, &file.EditionConfidence, &editionSource,
			&presentationKind, &presentationGroupKey, &partIndex, &partTotal,
			&multiStart, &multiEnd,
		); err != nil {
			return nil, fmt.Errorf("scanning variant file: %w", err)
		}
		file.ContentID = stringPtrValue(contentID)
		file.EpisodeID = stringPtrValue(episodeID)
		file.EditionRaw = stringPtrValue(editionRaw)
		file.EditionKey = stringPtrValue(editionKey)
		file.EditionSource = stringPtrValue(editionSource)
		file.PresentationKind = stringPtrValue(presentationKind)
		file.PresentationGroupKey = stringPtrValue(presentationGroupKey)
		file.PresentationPartIndex = intPtrValue(partIndex)
		file.PresentationPartTotal = intPtrValue(partTotal)
		file.MultiEpisodeStart = intPtrValue(multiStart)
		file.MultiEpisodeEnd = intPtrValue(multiEnd)
		files = append(files, file)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterating variant files: %w", err)
	}
	return files, nil
}

func intPtrValue(value *int) int {
	if value == nil {
		return 0
	}
	return *value
}
