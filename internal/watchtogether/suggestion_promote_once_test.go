package watchtogether

import (
	"errors"
	"testing"
	"time"
)

func TestPromotionOncePreservesSelectedVariantPG(t *testing.T) {
	pool := selectionPG(t)
	repo := NewRepository(pool)
	now := time.Now().UTC()
	room := baseRoom(now)
	room.SelectionMode = RoomSelectionModeVote
	if _, err := repo.CreateRoom(t.Context(), room); err != nil {
		t.Fatal(err)
	}
	resolver := &stubSelectionResolver{resolved: &ResolvedSelection{ContentID: "winner-content", FileID: new(7), LibraryID: new(8)}}
	s := newServiceForTest(now, &stubRepo{room: room}, nil, nil, resolver)
	s.repo = repo
	suggestions := &stubSuggestions{ordered: []Suggestion{{ID: "winner", RoomID: room.ID, ContentID: "winner-content", VoteCount: 3}, {ID: "loser", RoomID: room.ID, ContentID: "loser-content", VoteCount: 1}, {ID: "foreign", RoomID: "other", ContentID: "foreign"}}}
	s.suggestions = suggestions
	conn := new(recordingConn)
	member := &memberState{userID: 7, profileID: "host", connection: conn}
	live := s.rooms[room.ID]
	live.members["host"] = member
	for _, tc := range []struct {
		id      string
		user    int
		profile string
		want    error
	}{{"winner", 8, "host", ErrRoomForbidden}, {"winner", 7, "guest", ErrRoomForbidden}, {"foreign", 7, "host", ErrSuggestionNotFound}, {"loser", 7, "host", ErrNotVoteWinner}} {
		if _, err := s.PromoteSuggestionOnce(t.Context(), room.ID, tc.id, tc.user, tc.profile); !errors.Is(err, tc.want) {
			t.Fatalf("refusal: %v", err)
		}
	}
	first, err := s.PromoteSuggestionOnce(t.Context(), room.ID, "winner", 7, "host")
	if err != nil {
		t.Fatal(err)
	}
	member.sessionID = "fresh"
	member.isReady = true
	member.isBuffering = true
	member.ignoreWait = true
	advanced, err := repo.UpdateAnchor(t.Context(), room.ID, 99, false, RoomPlaybackStatePlaying, false, now.Add(time.Second), first.Generation+1, first.Generation)
	if err != nil {
		t.Fatal(err)
	}
	live.room = *advanced
	before := len(conn.payloads)
	resolver.resolved = &ResolvedSelection{ContentID: "winner-content", FileID: new(9), LibraryID: new(8)}
	repeat, err := s.PromoteSuggestionOnce(t.Context(), room.ID, "winner", 7, "host")
	if err != nil || repeat.Generation != advanced.Generation || repeat.SelectionRevision != first.SelectionRevision || repeat.AnchorPositionSeconds != 99 || repeat.IsPaused || *repeat.SelectedFileID != 7 || member.sessionID != "fresh" || !member.isReady || !member.isBuffering || !member.ignoreWait || len(conn.payloads) != before {
		t.Fatalf("repeat reset: %+v %v", repeat, err)
	}
	// Direct host-pick selection still compares the explicit resolved file variant.
	if _, err = pool.Exec(t.Context(), `UPDATE watch_together_rooms SET selection_mode='host_pick' WHERE id=$1`, room.ID); err != nil {
		t.Fatal(err)
	}
	live.room.SelectionMode = RoomSelectionModeHostPick
	changed, err := s.SelectItemOnce(t.Context(), room.ID, 7, "host", SelectItemInput{ContentID: "winner-content", FileID: new(9)})
	if err != nil || *changed.SelectedFileID != 9 || changed.SelectionRevision != first.SelectionRevision+1 || member.sessionID != "" {
		t.Fatalf("direct selection comparison changed: %+v %v", changed, err)
	}
	// No votes and closed rooms still refuse promotion; no-op does not bypass eligibility.
	live.room.SelectionMode = RoomSelectionModeVote
	suggestions.ordered[0].VoteCount = 0
	if _, err = s.PromoteSuggestionOnce(t.Context(), room.ID, "winner", 7, "host"); !errors.Is(err, ErrNoVotesCast) {
		t.Fatalf("no votes: %v", err)
	}
	suggestions.ordered[0].VoteCount = 3
	if _, err = pool.Exec(t.Context(), `UPDATE watch_together_rooms SET phase='ended' WHERE id=$1`, room.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = s.PromoteSuggestionOnce(t.Context(), room.ID, "winner", 7, "host"); !errors.Is(err, ErrRoomClosed) {
		t.Fatalf("closed: %v", err)
	}
}
