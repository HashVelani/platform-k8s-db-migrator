package decision

import "testing"

func TestDecide_FreshDatabase_ReturnsUpdate(t *testing.T) {
	action, err := Decide("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionUpdate {
		t.Errorf("expected %q for fresh database, got %q", ActionUpdate, action)
	}
}

func TestDecide_AlreadyMigrated_ReturnsNone(t *testing.T) {
	action, err := Decide("create-users-table")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionNone {
		t.Errorf("expected %q when DB is already migrated, got %q", ActionNone, action)
	}
}

// TestDecide_NoRegionCheck asserts that Decide has no region parameter —
// region-based block logic is Phase 4 and must not appear in current scope.
func TestDecide_NoRegionCheck(t *testing.T) {
	// This test intentionally calls Decide with only latestDB.
	// If the signature ever gains a region parameter, this test will fail to
	// compile, acting as a scope-creep guard.
	_, err := Decide("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
