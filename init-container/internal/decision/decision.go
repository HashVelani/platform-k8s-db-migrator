package decision

import (
	"fmt"

	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/region"
)

// Action represents the migration action the liquibase runner should take.
type Action string

const (
	ActionUpdate   Action = "update"   // DB behind changelog — run liquibase update
	ActionRollback Action = "rollback" // DB ahead of changelog — run liquibase rollback
	ActionNone     Action = "none"     // DB matches changelog — nothing to do
	ActionBlock    Action = "block"    // EUW2 mismatch — block deployment
)

// TODO Phase 2: implement migration direction detection
// Decide compares current DB changeset state against the changelog files
// and returns the Action the liquibase runner should take.
// Writes the Action to dest/migration-action.txt so the main container
// can read it and run the correct liquibase command.
func Decide(role region.RegionRole, latestDB string, changelogPath string) (Action, error) {
	// TODO Phase 2
	return "", fmt.Errorf("not implemented: Phase 2")
}
