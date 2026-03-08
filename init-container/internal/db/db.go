package db

import (
	"context"
	"fmt"
)

// TODO Phase 2: implement DB state check
// GetLatestChangeset connects to PostgreSQL and queries DATABASECHANGELOG
// to get the latest applied changeset ID.
// Returns empty string if table does not exist (fresh database).
// Connection details come from DB_URL, DB_USERNAME, DB_PASSWORD env vars.
// JDBC URL must be parsed to plain DSN for Go's database/sql driver.
func GetLatestChangeset(ctx context.Context) (string, error) {
	// TODO Phase 2
	return "", fmt.Errorf("not implemented: Phase 2")
}
