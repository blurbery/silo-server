package scanner

import (
	"reflect"
	"testing"
)

func TestSortedVariantOwnerIDs(t *testing.T) {
	ids := map[string]struct{}{
		"episode-c": {},
		"episode-a": {},
		"episode-b": {},
	}

	got := sortedVariantOwnerIDs(ids)
	want := []string{"episode-a", "episode-b", "episode-c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sortedVariantOwnerIDs() = %v, want %v", got, want)
	}
}

func TestSortedVariantOwnerIDsEmpty(t *testing.T) {
	if got := sortedVariantOwnerIDs(nil); len(got) != 0 {
		t.Fatalf("sortedVariantOwnerIDs(nil) = %v, want empty", got)
	}
}
