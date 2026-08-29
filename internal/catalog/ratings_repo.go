package catalog

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// UserRating represents a user's rating for a media item.
type UserRating struct {
	UserID      int       `json:"user_id"`
	ProfileID   string    `json:"profile_id"`
	MediaItemID string    `json:"media_item_id"`
	Rating      int       `json:"rating"`
	RatedAt     time.Time `json:"rated_at"`
}

// CommunityRating is a watched profile's rating plus its server-local reaction
// totals. User and profile IDs stay internal; handlers expose only an opaque
// per-rating key and an abbreviated display name.
type CommunityRating struct {
	UserID           int
	ProfileID        string
	ProfileName      string
	Avatar           string
	ProfileUpdatedAt time.Time
	Rating           int
	RatedAt          time.Time
	UpCount          int
	DownCount        int
	ViewerReaction   int
	AverageRating    float64
	TotalVoteCount   int
}

// RatingsRepo provides access to the user_ratings table.
type RatingsRepo struct {
	pool *pgxpool.Pool
}

// NewRatingsRepo creates a new RatingsRepo.
func NewRatingsRepo(pool *pgxpool.Pool) *RatingsRepo {
	return &RatingsRepo{pool: pool}
}

// Set creates or updates a user's rating for an item.
func (r *RatingsRepo) Set(ctx context.Context, userID int, profileID, mediaItemID string, rating int) error {
	_, err := r.pool.Exec(ctx, `
		INSERT INTO user_ratings (user_id, profile_id, media_item_id, rating, rated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (user_id, profile_id, media_item_id)
		DO UPDATE SET rating = EXCLUDED.rating, rated_at = EXCLUDED.rated_at`,
		userID, profileID, mediaItemID, rating,
	)
	if err != nil {
		return fmt.Errorf("set rating: %w", err)
	}
	return nil
}

// Get retrieves a user's rating for an item. Returns nil if not rated.
func (r *RatingsRepo) Get(ctx context.Context, userID int, profileID, mediaItemID string) (*UserRating, error) {
	var ur UserRating
	err := r.pool.QueryRow(ctx, `
		SELECT user_id, profile_id, media_item_id, rating, rated_at
		FROM user_ratings
		WHERE user_id = $1 AND profile_id = $2 AND media_item_id = $3`,
		userID, profileID, mediaItemID,
	).Scan(&ur.UserID, &ur.ProfileID, &ur.MediaItemID, &ur.Rating, &ur.RatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get rating: %w", err)
	}
	return &ur, nil
}

// Delete removes a user's rating for an item.
func (r *RatingsRepo) Delete(ctx context.Context, userID int, profileID, mediaItemID string) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM user_ratings
		WHERE user_id = $1 AND profile_id = $2 AND media_item_id = $3`,
		userID, profileID, mediaItemID,
	)
	if err != nil {
		return fmt.Errorf("delete rating: %w", err)
	}
	return nil
}

// List returns all ratings for a user+profile with pagination.
func (r *RatingsRepo) List(ctx context.Context, userID int, profileID string, limit, offset int) ([]UserRating, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT user_id, profile_id, media_item_id, rating, rated_at
		FROM user_ratings
		WHERE user_id = $1 AND profile_id = $2
		ORDER BY rated_at DESC
		LIMIT $3 OFFSET $4`,
		userID, profileID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list ratings: %w", err)
	}
	defer rows.Close()

	var ratings []UserRating
	for rows.Next() {
		var ur UserRating
		if err := rows.Scan(&ur.UserID, &ur.ProfileID, &ur.MediaItemID, &ur.Rating, &ur.RatedAt); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		ratings = append(ratings, ur)
	}
	return ratings, rows.Err()
}

// ListForItems returns ratings for a specific set of item IDs (used by recommendation filtering).
func (r *RatingsRepo) ListForItems(ctx context.Context, userID int, profileID string, itemIDs []string) (map[string]int, error) {
	rows, err := r.pool.Query(ctx, `
		SELECT media_item_id, rating
		FROM user_ratings
		WHERE user_id = $1 AND profile_id = $2 AND media_item_id = ANY($3)`,
		userID, profileID, itemIDs,
	)
	if err != nil {
		return nil, fmt.Errorf("list ratings for items: %w", err)
	}
	defer rows.Close()

	result := make(map[string]int, len(itemIDs))
	for rows.Next() {
		var itemID string
		var rating int
		if err := rows.Scan(&itemID, &rating); err != nil {
			return nil, fmt.Errorf("scan rating: %w", err)
		}
		result[itemID] = rating
	}
	return result, rows.Err()
}

// ListCommunity returns ratings only from profiles that have watched the item.
// A movie counts directly; episode progress rolls up to its parent series. The
// watched threshold and hidden-history handling mirror recommendation signals,
// but the returned average is display-only and never feeds those signals.
func (r *RatingsRepo) ListCommunity(
	ctx context.Context,
	viewerUserID int,
	viewerProfileID string,
	mediaItemID string,
	limit int,
	offset int,
) ([]CommunityRating, error) {
	rows, err := r.pool.Query(ctx, `
		WITH eligible AS (
			SELECT ur.user_id,
			       ur.profile_id,
			       up.name AS profile_name,
			       up.avatar,
			       up.updated_at AS profile_updated_at,
			       ur.rating,
			       ur.rated_at
			FROM user_ratings ur
			JOIN user_profiles up
			  ON up.user_id = ur.user_id
			 AND up.id = ur.profile_id
			WHERE ur.media_item_id = $1
			  AND EXISTS (
				SELECT 1
				FROM user_watch_progress wp
				LEFT JOIN episodes e ON e.content_id = wp.media_item_id
				WHERE wp.user_id = ur.user_id
				  AND wp.profile_id = ur.profile_id
				  AND COALESCE(e.series_id, wp.media_item_id) = ur.media_item_id
				  AND (
					wp.completed = true
					OR (
						wp.duration_seconds > 0
						AND wp.position_seconds / wp.duration_seconds >= 0.5
					)
				  )
				  AND NOT EXISTS (
					SELECT 1
					FROM user_history_hidden_items hhi
					WHERE hhi.user_id = wp.user_id
					  AND hhi.profile_id = wp.profile_id
					  AND hhi.media_item_id = wp.media_item_id
					  AND wp.updated_at <= hhi.hidden_before
				  )
			  )
		), reaction_counts AS (
			SELECT target_user_id,
			       target_profile_id,
			       COUNT(*) FILTER (WHERE reaction = 1)::integer AS up_count,
			       COUNT(*) FILTER (WHERE reaction = -1)::integer AS down_count,
			       COALESCE(
				MAX(reaction) FILTER (
					WHERE reactor_user_id = $2 AND reactor_profile_id = $3
				),
				0
			   )::integer AS viewer_reaction
			FROM household_rating_reactions
			WHERE media_item_id = $1
			GROUP BY target_user_id, target_profile_id
		)
		SELECT e.user_id,
		       e.profile_id,
		       e.profile_name,
		       e.avatar,
		       e.profile_updated_at,
		       e.rating,
		       e.rated_at,
		       COALESCE(rc.up_count, 0),
		       COALESCE(rc.down_count, 0),
		       COALESCE(rc.viewer_reaction, 0),
		       AVG(e.rating) OVER ()::double precision AS average_rating,
		       COUNT(*) OVER ()::integer AS total_vote_count
		FROM eligible e
		LEFT JOIN reaction_counts rc
		  ON rc.target_user_id = e.user_id
		 AND rc.target_profile_id = e.profile_id
		ORDER BY (e.user_id = $2 AND e.profile_id = $3) DESC,
		         e.rated_at DESC,
		         e.user_id,
		         e.profile_id
		LIMIT $4 OFFSET $5`,
		mediaItemID, viewerUserID, viewerProfileID, limit, offset,
	)
	if err != nil {
		return nil, fmt.Errorf("list community ratings: %w", err)
	}
	defer rows.Close()

	ratings := make([]CommunityRating, 0, limit)
	for rows.Next() {
		var rating CommunityRating
		if err := rows.Scan(
			&rating.UserID,
			&rating.ProfileID,
			&rating.ProfileName,
			&rating.Avatar,
			&rating.ProfileUpdatedAt,
			&rating.Rating,
			&rating.RatedAt,
			&rating.UpCount,
			&rating.DownCount,
			&rating.ViewerReaction,
			&rating.AverageRating,
			&rating.TotalVoteCount,
		); err != nil {
			return nil, fmt.Errorf("scan community rating: %w", err)
		}
		ratings = append(ratings, rating)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate community ratings: %w", err)
	}
	return ratings, nil
}

// SetCommunityReaction records one profile-scoped reaction for a watched
// profile's rating. The INSERT rechecks watched eligibility so an unwatch or
// history-hide racing the request cannot leave a reaction on an ineligible row.
func (r *RatingsRepo) SetCommunityReaction(
	ctx context.Context,
	viewerUserID int,
	viewerProfileID string,
	mediaItemID string,
	targetUserID int,
	targetProfileID string,
	reaction int,
) (bool, error) {
	var inserted int
	err := r.pool.QueryRow(ctx, `
		INSERT INTO household_rating_reactions (
			media_item_id,
			target_user_id,
			target_profile_id,
			reactor_user_id,
			reactor_profile_id,
			reaction,
			reacted_at
		)
		SELECT ur.media_item_id, ur.user_id, ur.profile_id, $4, $5, $6, NOW()
		FROM user_ratings ur
		WHERE ur.media_item_id = $1
		  AND ur.user_id = $2
		  AND ur.profile_id = $3
		  AND (ur.user_id <> $4 OR ur.profile_id <> $5)
		  AND EXISTS (
			SELECT 1
			FROM user_watch_progress wp
			LEFT JOIN episodes e ON e.content_id = wp.media_item_id
			WHERE wp.user_id = ur.user_id
			  AND wp.profile_id = ur.profile_id
			  AND COALESCE(e.series_id, wp.media_item_id) = ur.media_item_id
			  AND (
				wp.completed = true
				OR (
					wp.duration_seconds > 0
					AND wp.position_seconds / wp.duration_seconds >= 0.5
				)
			  )
			  AND NOT EXISTS (
				SELECT 1
				FROM user_history_hidden_items hhi
				WHERE hhi.user_id = wp.user_id
				  AND hhi.profile_id = wp.profile_id
				  AND hhi.media_item_id = wp.media_item_id
				  AND wp.updated_at <= hhi.hidden_before
			  )
		  )
		ON CONFLICT (
			media_item_id,
			target_user_id,
			target_profile_id,
			reactor_user_id,
			reactor_profile_id
		) DO UPDATE
		SET reaction = EXCLUDED.reaction,
		    reacted_at = EXCLUDED.reacted_at
		RETURNING 1`,
		mediaItemID,
		targetUserID,
		targetProfileID,
		viewerUserID,
		viewerProfileID,
		reaction,
	).Scan(&inserted)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("set community rating reaction: %w", err)
	}
	return inserted == 1, nil
}

// DeleteCommunityReaction removes the current profile's reaction to a rating.
func (r *RatingsRepo) DeleteCommunityReaction(
	ctx context.Context,
	viewerUserID int,
	viewerProfileID string,
	mediaItemID string,
	targetUserID int,
	targetProfileID string,
) error {
	_, err := r.pool.Exec(ctx, `
		DELETE FROM household_rating_reactions
		WHERE media_item_id = $1
		  AND target_user_id = $2
		  AND target_profile_id = $3
		  AND reactor_user_id = $4
		  AND reactor_profile_id = $5`,
		mediaItemID,
		targetUserID,
		targetProfileID,
		viewerUserID,
		viewerProfileID,
	)
	if err != nil {
		return fmt.Errorf("delete community rating reaction: %w", err)
	}
	return nil
}
