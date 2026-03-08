package decision

// Action represents the migration action the Liquibase runner should take.
type Action string

const (
	ActionUpdate   Action = "update"   // DB behind changelog — run liquibase update
	ActionRollback Action = "rollback" // DB ahead of changelog — run liquibase rollback (Phase 3)
	ActionNone     Action = "none"     // DB matches changelog — nothing to do
	ActionBlock    Action = "block"    // Region mismatch — block deployment (Phase 4)
)

// Decide returns the migration Action based on the current database state.
//
// latestDB is the most recent changeset ID recorded in DATABASECHANGELOG, or
// empty string when the database is fresh (table not yet created by Liquibase).
//
// Current logic:
//   - Empty latestDB  → ActionUpdate  (fresh database, run all changesets)
//   - Non-empty latestDB → ActionNone (already migrated; rollback detection is Phase 3)
//
// Rollback detection will be added in Phase 3.
// Region-based block logic will be added in Phase 4.
func Decide(latestDB string) (Action, error) {
	if latestDB == "" {
		return ActionUpdate, nil
	}
	return ActionNone, nil
}
