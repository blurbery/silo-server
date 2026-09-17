package watchtogether

import (
	"context"
	"errors"
)

var ErrSuggestionPromotionUnavailable = errors.New("suggestion promotion unavailable")

func (s *Service) PromoteSuggestionOnce(
	ctx context.Context,
	roomID string,
	suggestionID string,
	userID int,
	profileID string,
) (Snapshot, error) {
	if s == nil || s.suggestions == nil {
		return Snapshot{}, ErrSuggestionPromotionUnavailable
	}

	_, live, err := s.getOrLoadLiveRoom(ctx, roomID)
	if err != nil {
		return Snapshot{}, err
	}

	s.mu.Lock()
	if live.room.HostUserID != userID || live.room.HostProfileID != profileID {
		s.mu.Unlock()
		return Snapshot{}, ErrRoomForbidden
	}
	s.mu.Unlock()

	suggestion, err := s.suggestions.GetSuggestion(ctx, suggestionID)
	if err != nil {
		return Snapshot{}, err
	}
	if suggestion.RoomID != roomID {
		return Snapshot{}, ErrSuggestionNotFound
	}

	// In a vote room the host starts the winner; they do not get to overrule it.
	// Being able to promote any suggestion would make "vote" host_pick with
	// extra steps, and the tally on everyone else's screen would be a lie.
	//
	// The winner is read here and the selection commits a moment later, so a
	// vote landing in between can start a title that has just stopped being the
	// head of the tally. That is deliberate: the host pressed start on the
	// standings they and the room could see, and a vote arriving during the
	// round trip should not retroactively overrule the press. Closing the window
	// would mean holding the room lock across a suggestion-store read, which
	// stalls every other room for a race whose worst case is off by one vote.
	s.mu.Lock()
	isVoteRoom := live.room.SelectionMode == RoomSelectionModeVote
	s.mu.Unlock()
	if isVoteRoom {
		winner, err := s.VoteWinner(ctx, roomID)
		if err != nil {
			return Snapshot{}, err
		}
		if winner.ID != suggestion.ID {
			return Snapshot{}, ErrNotVoteWinner
		}
	}

	return s.selectItemOnce(ctx, roomID, userID, profileID, SelectItemInput{
		ContentID: suggestion.ContentID,
	}, isVoteRoom, true)
}
