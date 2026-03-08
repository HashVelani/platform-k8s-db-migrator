package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq"
	_ "github.com/lib/pq" // PostgreSQL driver
)

// ParseDSN converts a JDBC PostgreSQL URL to a Go connection string understood
// by database/sql + lib/pq.
//
// Accepts both JDBC form:
//
//	jdbc:postgresql://host:port/dbname[?params]
//
// and plain Go form:
//
//	postgresql://host:port/dbname[?params]
//
// The supplied username and password are embedded in the returned URL.
func ParseDSN(jdbcURL, username, password string) (string, error) {
	// Strip the "jdbc:" scheme prefix that Java drivers require.
	raw := strings.TrimPrefix(jdbcURL, "jdbc:")

	if !strings.HasPrefix(raw, "postgresql://") && !strings.HasPrefix(raw, "postgres://") {
		return "", fmt.Errorf("unsupported DB URL %q: must start with jdbc:postgresql:// or postgresql://", jdbcURL)
	}

	u, err := url.Parse(raw)
	if err != nil {
		return "", fmt.Errorf("invalid DB URL %q: %w", jdbcURL, err)
	}

	u.User = url.UserPassword(username, password)
	return u.String(), nil
}

// GetLatestChangeset connects to PostgreSQL using dsn and queries
// DATABASECHANGELOG for the most recently applied changeset ID.
//
// Returns ("", nil) when the database is fresh — i.e. the DATABASECHANGELOG
// table has not been created yet (Liquibase has never run against it).
func GetLatestChangeset(ctx context.Context, dsn string) (string, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return "", fmt.Errorf("cannot open database connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return "", fmt.Errorf("cannot reach database: %w", err)
	}

	const query = `
		SELECT id
		FROM databasechangelog
		ORDER BY dateexecuted DESC, orderexecuted DESC
		LIMIT 1`

	var id string
	err = db.QueryRowContext(ctx, query).Scan(&id)
	switch {
	case err == sql.ErrNoRows:
		// Table exists but Liquibase has never applied a changeset.
		return "", nil
	case err != nil:
		// 42P01 = undefined_table: DATABASECHANGELOG not yet created (fresh DB).
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "42P01" {
			return "", nil
		}
		return "", fmt.Errorf("cannot query DATABASECHANGELOG: %w", err)
	}

	return id, nil
}
