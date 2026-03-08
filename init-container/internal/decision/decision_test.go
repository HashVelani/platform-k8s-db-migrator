package decision

import (
	"strings"
	"testing"
)

func TestDecide_FreshDatabase_ReturnsUpdate(t *testing.T) {
	// dbCount=0, changelogCount=2: fresh DB, needs all changesets applied.
	action, err := Decide(0, 2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionUpdate {
		t.Errorf("expected %q for fresh database, got %q", ActionUpdate, action)
	}
}

func TestDecide_NewChangesets_ReturnsUpdate(t *testing.T) {
	// dbCount=1, changelogCount=3: 2 new changesets added in this release.
	action, err := Decide(1, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionUpdate {
		t.Errorf("expected %q when new changesets exist, got %q", ActionUpdate, action)
	}
}

// TestDecide_SingleStepUpdate checks the boundary where exactly one new
// changeset has been added — the most common case during a normal release.
func TestDecide_SingleStepUpdate(t *testing.T) {
	action, err := Decide(4, 5)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionUpdate {
		t.Errorf("expected %q for single new changeset, got %q", ActionUpdate, action)
	}
}

func TestDecide_UpToDate_ReturnsNone(t *testing.T) {
	// dbCount==changelogCount: nothing to do.
	action, err := Decide(3, 3)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionNone {
		t.Errorf("expected %q when up to date, got %q", ActionNone, action)
	}
}

func TestDecide_BothZero_ReturnsNone(t *testing.T) {
	// Empty changelog + fresh DB: nothing to migrate.
	action, err := Decide(0, 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionNone {
		t.Errorf("expected %q when both counts are zero, got %q", ActionNone, action)
	}
}

// TestDecide_LargeNumbersUpToDate verifies there is no integer-overflow or
// boundary issue when the DB and changelog both have a large history.
func TestDecide_LargeNumbersUpToDate(t *testing.T) {
	action, err := Decide(100, 100)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if action != ActionNone {
		t.Errorf("expected %q for large equal counts, got %q", ActionNone, action)
	}
}

func TestDecide_ChangesetRemoved_ReturnsError(t *testing.T) {
	// dbCount=5, changelogCount=3: someone deleted changesets from the file — error.
	_, err := Decide(5, 3)
	if err == nil {
		t.Fatal("expected error when changelogCount < dbCount, got nil")
	}
	if !strings.Contains(err.Error(), "manual intervention") {
		t.Errorf("error should mention manual intervention, got: %v", err)
	}
}

// TestDecide_ErrorMentionsChangesetCounts verifies the error message includes
// both counts so operators know exactly what went wrong without digging through
// logs.
func TestDecide_ErrorMentionsChangesetCounts(t *testing.T) {
	_, err := Decide(10, 7)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "10") {
		t.Errorf("error should include dbCount=10, got: %v", err)
	}
	if !strings.Contains(err.Error(), "7") {
		t.Errorf("error should include changelogCount=7, got: %v", err)
	}
}

// TestDecide_NoRegionCheck asserts that Decide takes only counts, not a region
// parameter — region-based block logic is Phase 4.
func TestDecide_NoRegionCheck(t *testing.T) {
	// Compile-time guard: if the signature ever gains a region parameter this
	// test will fail to compile.
	_, err := Decide(0, 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
