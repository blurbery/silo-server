package watchtogether

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/Silo-Server/silo-server/internal/catalog"
)

// selectionOnceStore serializes identity comparison and selection replacement.
// V1 writers keep their existing unconditional reset semantics.
type selectionOnceStore interface {
	SelectOnce(context.Context, string, int, string, SelectItemInput, bool, int64, time.Time) (*Room, bool, error)
}

func (r *Repository) SelectOnce(ctx context.Context, roomID string, user int, profile string, selection SelectItemInput, viaVote bool, expected int64, now time.Time) (*Room, bool, error) {
	return r.selectOnce(ctx, roomID, user, profile, selection, viaVote, expected, now, false)
}

// PromoteOnce preserves the currently selected content variant. Suggestions
// identify content only; resolving a new default file must not restart it.
func (r *Repository) PromoteOnce(ctx context.Context, roomID string, user int, profile string, selection SelectItemInput, viaVote bool, expected int64, now time.Time) (*Room, bool, error) {
	return r.selectOnce(ctx, roomID, user, profile, selection, viaVote, expected, now, true)
}

func (r *Repository) selectOnce(ctx context.Context, roomID string, user int, profile string, selection SelectItemInput, viaVote bool, expected int64, now time.Time, contentOnly bool) (*Room, bool, error) {
	if r == nil || r.pool == nil {
		return nil, false, fmt.Errorf("watch together repository unavailable")
	}
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	room, err := scanRoom(tx.QueryRow(ctx, `SELECT `+roomColumns+` FROM watch_together_rooms WHERE id=$1 FOR UPDATE`, roomID))
	if err != nil {
		return nil, false, err
	}
	if room.HostUserID != user || room.HostProfileID != profile {
		return nil, false, ErrRoomForbidden
	}
	if room.Phase == RoomPhaseEnded {
		return nil, false, ErrRoomClosed
	}
	if room.SelectionMode == RoomSelectionModeVote && !viaVote {
		return nil, false, ErrVoteRoomSelection
	}
	identical := room.SelectedContentID != nil && *room.SelectedContentID == selection.ContentID && (contentOnly || (equalSelectionID(room.SelectedFileID, selection.FileID) && equalSelectionID(room.SelectedLibraryID, selection.LibraryID)))
	if identical || room.Generation != expected {
		if err := tx.Commit(ctx); err != nil {
			return nil, false, err
		}
		return room, false, nil
	}
	room, err = scanRoom(tx.QueryRow(ctx, `UPDATE watch_together_rooms SET
 phase='playing', playback_state='waiting', resume_on_ready=true,
 selected_content_id=$2, selected_file_id=$3, selected_library_id=$4,
 anchor_position_seconds=0, is_paused=true, anchor_updated_at=$5,
 selection_revision=selection_revision+1, generation=generation+1
 WHERE id=$1 RETURNING `+roomColumns, roomID, selection.ContentID, selection.FileID, selection.LibraryID, now.UTC()))
	if err != nil {
		return nil, false, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, false, err
	}
	return room, true, nil
}

func equalSelectionID(a, b *int) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// SelectItemOnce is the v2 selection path. It never resets an identical current
// resolved selection; a different intervening selection is not a replay receipt.
func (s *Service) SelectItemOnce(ctx context.Context, roomID string, user int, profile string, input SelectItemInput) (Snapshot, error) {
	return s.selectItemOnce(ctx, roomID, user, profile, input, false, false)
}

func (s *Service) selectItemOnce(ctx context.Context, roomID string, user int, profile string, input SelectItemInput, viaVote, promotion bool) (Snapshot, error) {
	if s == nil {
		return Snapshot{}, fmt.Errorf("watch together unavailable")
	}
	store, ok := s.repo.(selectionOnceStore)
	if !ok || s.selectionResolver == nil {
		return Snapshot{}, fmt.Errorf("watch together selection unavailable")
	}
	write := store.SelectOnce
	if promotion {
		promoting, ok := s.repo.(interface {
			PromoteOnce(context.Context, string, int, string, SelectItemInput, bool, int64, time.Time) (*Room, bool, error)
		})
		if !ok {
			return Snapshot{}, ErrSuggestionPromotionUnavailable
		}
		write = promoting.PromoteOnce
	}
	if strings.TrimSpace(input.ContentID) == "" {
		return Snapshot{}, ErrInvalidSelection
	}
	resolved, err := s.selectionResolver.ResolveSelection(ctx, user, profile, input)
	if err != nil {
		if errors.Is(err, catalog.ErrWatchTargetNotPlayable) {
			return Snapshot{}, ErrInvalidSelection
		}
		return Snapshot{}, err
	}
	if resolved == nil || strings.TrimSpace(resolved.ContentID) == "" {
		return Snapshot{}, ErrInvalidSelection
	}
	_, live, err := s.getOrLoadLiveRoom(ctx, roomID)
	if err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	expected := live.room.Generation
	s.mu.Unlock()
	room, applied, err := write(ctx, roomID, user, profile, SelectItemInput{ContentID: resolved.ContentID, FileID: resolved.FileID, LibraryID: resolved.LibraryID}, viaVote, expected, s.now())
	if err != nil {
		return Snapshot{}, err
	}
	s.mu.Lock()
	// A local close/removal must never be resurrected by a delayed receipt.
	if s.rooms[roomID] != live || live.room.Phase == RoomPhaseEnded {
		s.mu.Unlock()
		return Snapshot{}, ErrRoomClosed
	}
	if room.Generation >= live.room.Generation {
		if room.SelectionRevision > live.room.SelectionRevision {
			for _, member := range live.members {
				if member == nil {
					continue
				}
				member.sessionID = ""
				member.isReady = false
				member.isBuffering = false
				member.ignoreWait = false
			}
			s.disarmWaitingDeadlineLocked(live)
		}
		live.room = *room
	}
	snapshot := s.buildSnapshotLocked(live, user, profile)
	if !applied {
		s.mu.Unlock()
		return snapshot, nil
	}
	dispatches := s.prepareSnapshotDispatchesLocked(live)
	s.mu.Unlock()
	s.runDispatches(dispatches)
	return snapshot, nil
}
