package catalog

import (
	"slices"
	"testing"
	"time"
)

func TestLibraryCollectionPosterMutationLocksSerializePerCollection(t *testing.T) {
	var locks libraryCollectionPosterMutationLocks
	unlockFirst := locks.lock("collection-1")
	defer func() {
		if unlockFirst != nil {
			unlockFirst()
		}
	}()

	sameStarted := make(chan struct{})
	sameAcquired := make(chan struct{})
	go func() {
		close(sameStarted)
		unlock := locks.lock("collection-1")
		close(sameAcquired)
		unlock()
	}()
	<-sameStarted
	select {
	case <-sameAcquired:
		t.Fatal("same collection acquired lock concurrently")
	case <-time.After(50 * time.Millisecond):
	}

	otherAcquired := make(chan struct{})
	go func() {
		unlock := locks.lock("collection-2")
		close(otherAcquired)
		unlock()
	}()
	select {
	case <-otherAcquired:
	case <-time.After(time.Second):
		t.Fatal("different collection was unnecessarily blocked")
	}

	unlockFirst()
	unlockFirst = nil
	select {
	case <-sameAcquired:
	case <-time.After(time.Second):
		t.Fatal("same collection did not acquire lock after release")
	}
}

func TestLibraryCollectionLifecycleLockIDs(t *testing.T) {
	tests := []struct {
		name  string
		oldID int
		newID int
		want  []int
	}{
		{name: "same", oldID: 4, newID: 4, want: []int{4}},
		{name: "ascending", oldID: 4, newID: 9, want: []int{4, 9}},
		{name: "stable sorted", oldID: 9, newID: 4, want: []int{4, 9}},
		{name: "removed", oldID: 4, newID: 0, want: []int{4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := libraryCollectionLifecycleLockIDs(tt.oldID, tt.newID)
			if !slices.Equal(got, tt.want) {
				t.Fatalf("lock IDs = %v, want %v", got, tt.want)
			}
		})
	}
}
