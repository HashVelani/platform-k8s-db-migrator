package region

import "fmt"

// RegionRole identifies whether this pod is running in the primary or replica region.
type RegionRole string

const (
	Primary RegionRole = "primary" // EUW1 — Aurora writer, runs migrations
	Replica RegionRole = "replica" // EUW2 — Aurora reader, check only
)

// TODO Phase 2: implement region detection
// Detect reads REGION_ROLE env var and returns the RegionRole.
// primary → EUW1: full migration logic (update or rollback)
// replica → EUW2: check only — if DB does not match changelog, exit 1 with
//
//	message: "Schema mismatch in replica region <REGION>.
//	          EUW1 (primary) has not been synced yet.
//	          Sync the EUW1 ArgoCD application first, then retry."
func Detect() (RegionRole, error) {
	// TODO Phase 2
	return "", fmt.Errorf("not implemented: Phase 2")
}
