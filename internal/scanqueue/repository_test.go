package scanqueue

import (
	"strings"
	"testing"
)

func TestCreateScanRunQueryHandlesConflictWithoutDatabaseError(t *testing.T) {
	normalized := strings.Join(strings.Fields(createScanRunQuery), " ")
	if !strings.Contains(normalized, "ON CONFLICT DO NOTHING RETURNING") {
		t.Fatalf("create scan-run query must suppress expected active-scope conflicts, got %q", normalized)
	}
}
