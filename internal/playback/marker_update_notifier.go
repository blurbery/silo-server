package playback

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/Silo-Server/silo-server/internal/models"
	"github.com/google/uuid"
)

type markerUpdateSessionLookup interface {
	GetSessionsByMediaFileID(fileID int) []*Session
}

// MarkerUpdateNotifier publishes live marker updates to active playback sessions.
type MarkerUpdateNotifier struct {
	sessions  markerUpdateSessionLookup
	hub       *RealtimeHub
	sourceID  string
	fileLocks [64]sync.Mutex

	mu      sync.RWMutex
	publish func(context.Context, string) error
}

// MarkerSnapshotFileLoader reads the persisted markers for a reconnecting session.
type MarkerSnapshotFileLoader interface {
	GetByID(context.Context, int) (*models.MediaFile, error)
}

// markerUpdateSnapshot carries enough data to deliver an update without reading
// the database: on-demand markers may never be persisted.
type markerUpdateSnapshot struct {
	SourceID string                 `json:"source_id"`
	FileID   int                    `json:"file_id"`
	Segments []models.MarkerSegment `json:"marker_segments"`
}

func NewMarkerUpdateNotifier(sessions markerUpdateSessionLookup, hub *RealtimeHub) *MarkerUpdateNotifier {
	if sessions == nil || hub == nil {
		return nil
	}
	return &MarkerUpdateNotifier{
		sessions: sessions,
		hub:      hub,
		sourceID: uuid.NewString(),
	}
}

// UseEventBus enables cross-replica delivery. Call it once during startup with
// the server lifetime context. Repeated calls do not create more subscriptions.
func (n *MarkerUpdateNotifier) UseEventBus(
	ctx context.Context,
	publish func(context.Context, string) error,
	subscribe func(context.Context, func(string)) error,
) error {
	if n == nil || publish == nil || subscribe == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.publish != nil {
		return nil
	}
	if err := subscribe(ctx, func(payload string) {
		if ctx.Err() != nil {
			return
		}
		var snapshot markerUpdateSnapshot
		if err := json.Unmarshal([]byte(payload), &snapshot); err != nil ||
			snapshot.SourceID == "" || snapshot.SourceID == n.sourceID || snapshot.FileID <= 0 {
			return
		}
		snapshot.Segments = models.EffectiveMarkerSegments(&models.MediaFile{MarkerSegments: snapshot.Segments})
		lock := n.fileLock(snapshot.FileID)
		lock.Lock()
		n.dispatch(ctx, snapshot)
		lock.Unlock()
	}); err != nil {
		return err
	}
	n.publish = publish
	return nil
}

// SendSessionSnapshot sends the current marker state after the control socket is ready.
func (n *MarkerUpdateNotifier) SendSessionSnapshot(ctx context.Context, sessionID string, file *models.MediaFile) {
	if n == nil || n.hub == nil || sessionID == "" || file == nil || file.ID <= 0 {
		return
	}
	lock := n.fileLock(file.ID)
	lock.Lock()
	defer lock.Unlock()
	n.sendSessionSnapshotLocked(ctx, sessionID, file)
}

// SendSessionSnapshotFromLoader orders the persisted read and send with updates.
func (n *MarkerUpdateNotifier) SendSessionSnapshotFromLoader(ctx context.Context, sessionID string, fileID int, loader MarkerSnapshotFileLoader) error {
	if n == nil || n.hub == nil || sessionID == "" || fileID <= 0 || loader == nil {
		return nil
	}
	lock := n.fileLock(fileID)
	lock.Lock()
	defer lock.Unlock()
	file, err := loader.GetByID(ctx, fileID)
	if err != nil || file == nil {
		return err
	}
	n.sendSessionSnapshotLocked(ctx, sessionID, file)
	return nil
}

func (n *MarkerUpdateNotifier) fileLock(fileID int) *sync.Mutex {
	return &n.fileLocks[uint(fileID)%uint(len(n.fileLocks))]
}

func (n *MarkerUpdateNotifier) sendSessionSnapshotLocked(ctx context.Context, sessionID string, file *models.MediaFile) {
	segments := models.EffectiveMarkerSegments(file)
	firstRange := func(kind string) *TimeRangePayload {
		for _, segment := range segments {
			if segment.Kind == kind {
				return &TimeRangePayload{Start: segment.StartSeconds, End: segment.EndSeconds}
			}
		}
		return nil
	}
	event, err := NewMarkersUpdatedEvent(sessionID, file.ID,
		firstRange("intro"), firstRange("credits"), firstRange("recap"), firstRange("preview"), segments...)
	if err != nil {
		slog.WarnContext(ctx, "failed to encode markers updated realtime event", "component", "playback", "session_id", sessionID, "file_id", file.ID, "error", err)
		return
	}
	if err := n.hub.Send(sessionID, event); err != nil && !errors.Is(err, ErrRealtimeConnectionNotFound) {
		slog.WarnContext(ctx, "failed to deliver markers updated realtime event", "component", "playback", "session_id", sessionID, "file_id", file.ID, "error", err)
	}
}

func (n *MarkerUpdateNotifier) MarkersUpdated(ctx context.Context, file *models.MediaFile) {
	if n == nil || file == nil || file.ID <= 0 || ctx.Err() != nil {
		return
	}
	lock := n.fileLock(file.ID)
	lock.Lock()
	defer lock.Unlock()

	snapshot := markerUpdateSnapshot{
		SourceID: n.sourceID,
		FileID:   file.ID,
		Segments: models.EffectiveMarkerSegments(file),
	}
	n.dispatch(ctx, snapshot)
	if ctx.Err() != nil {
		return
	}
	n.mu.RLock()
	publish := n.publish
	n.mu.RUnlock()
	if publish != nil {
		payload, err := json.Marshal(snapshot)
		if err == nil {
			publishCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
			err = publish(publishCtx, string(payload))
			cancel()
		}
		if err != nil {
			slog.WarnContext(ctx, "failed to publish marker update", "component", "playback", "file_id", file.ID, "error", err)
		}
	}
}

func (n *MarkerUpdateNotifier) dispatch(ctx context.Context, snapshot markerUpdateSnapshot) {
	firstRange := func(kind string) *TimeRangePayload {
		for _, segment := range snapshot.Segments {
			if segment.Kind == kind {
				return &TimeRangePayload{Start: segment.StartSeconds, End: segment.EndSeconds}
			}
		}
		return nil
	}
	intro, credits := firstRange("intro"), firstRange("credits")
	recap, preview := firstRange("recap"), firstRange("preview")
	for _, session := range n.sessions.GetSessionsByMediaFileID(snapshot.FileID) {
		if ctx.Err() != nil {
			return
		}
		if session == nil || session.ID == "" || !session.HasRealtimeConnection {
			continue
		}
		event, err := NewMarkersUpdatedEvent(session.ID, snapshot.FileID, intro, credits, recap, preview, snapshot.Segments...)
		if err != nil {
			slog.WarnContext(ctx,
				"failed to encode markers updated realtime event", "component", "playback",
				"session_id",
				session.ID,
				"file_id",
				snapshot.FileID,
				"error",
				err,
			)
			continue
		}
		if err := n.hub.Send(session.ID, event); err != nil && !errors.Is(err, ErrRealtimeConnectionNotFound) {
			slog.WarnContext(ctx,
				"failed to deliver markers updated realtime event", "component", "playback",
				"session_id",
				session.ID,
				"file_id",
				snapshot.FileID,
				"error",
				err,
			)
		}
	}
}
