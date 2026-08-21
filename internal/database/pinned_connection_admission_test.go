package database

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPinnedConnectionCapacity(t *testing.T) {
	tests := []struct {
		maxConns int32
		want     int
	}{
		{maxConns: 1, want: 0},
		{maxConns: 2, want: 1},
		{maxConns: 3, want: 1},
		{maxConns: 20, want: 10},
	}
	for _, tt := range tests {
		if got := pinnedConnectionCapacity(tt.maxConns); got != tt.want {
			t.Fatalf("capacity(%d) = %d, want %d", tt.maxConns, got, tt.want)
		}
	}
}

func TestPinnedConnectionAdmissionSharedAcrossSubsystems(t *testing.T) {
	var registry pinnedConnectionAdmissionRegistry
	pool := new(pgxpool.Pool)
	collectionAdmission := registry.forPool(pool, 1)
	playbackAdmission := registry.forPool(pool, 1)
	if collectionAdmission != playbackAdmission {
		t.Fatal("same pool received independent pinned-connection budgets")
	}

	release, err := collectionAdmission.acquire(context.Background())
	if err != nil {
		t.Fatalf("acquire collection slot: %v", err)
	}
	defer release()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := playbackAdmission.acquire(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("playback acquire error = %v, want context canceled while shared slot is occupied", err)
	}

	if registry.forPool(new(pgxpool.Pool), 1) == collectionAdmission {
		t.Fatal("different pools unexpectedly share an admission budget")
	}
}
