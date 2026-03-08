package decision

import "fmt"

// Action represents the migration action the Liquibase runner should take.
type Action string

const (
	ActionUpdate   Action = "update"   // DB behind changelog — run liquibase update
	ActionRollback Action = "rollback" // DB ahead of changelog — run liquibase rollback (Phase 3)
	ActionNone     Action = "none"     // DB matches changelog — nothing to do
	ActionBlock    Action = "block"    // Region mismatch — block deployment (Phase 4)
)

// Decide returns the migration Action by comparing the number of changesets
// already applied in DATABASECHANGELOG (dbCount) against the total number of
// changesets present in the copied changelog files (changelogCount).
//
//   - changelogCount > dbCount  → ActionUpdate  (new changesets to apply)
//   - changelogCount == dbCount → ActionNone    (database is up to date)
//   - changelogCount < dbCount  → error         (changeset removed; manual fix needed)
//
// Rollback detection will be added in Phase 3.
// Region-based block logic will be added in Phase 4.
func Decide(dbCount, changelogCount int) (Action, error) {
	switch {
	case changelogCount > dbCount:
		return ActionUpdate, nil
	case changelogCount == dbCount:
		return ActionNone, nil
	default:
		// changelogCount < dbCount: a changeset that was previously applied no
		// longer exists in the changelog files. This should never happen in normal
		// operation — it means someone deleted a changeset. Fail loudly.
		return "", fmt.Errorf(
			"database has %d applied changesets but changelog only contains %d — "+
				"a changeset was removed from the changelog files; manual intervention required",
			dbCount, changelogCount,
		)
	}
}
