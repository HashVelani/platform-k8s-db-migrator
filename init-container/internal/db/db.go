package db

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"

	"github.com/lib/pq" // also registers the "postgres" driver with database/sql
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
	if jdbcURL == "" {
		return "", fmt.Errorf("DB URL must not be empty")
	}

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

// GetAppliedChangesetCount connects to PostgreSQL using dsn and returns the
// number of changesets recorded in DATABASECHANGELOG.
//
// Returns 0 when the database is fresh — i.e. the DATABASECHANGELOG table has
// not been created yet (Liquibase has never run against this database).
func GetAppliedChangesetCount(ctx context.Context, dsn string) (int, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return 0, fmt.Errorf("cannot open database connection: %w", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		return 0, fmt.Errorf("cannot reach database: %w", err)
	}

	var count int
	err = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM databasechangelog`).Scan(&count)
	if err != nil {
		// 42P01 = undefined_table: DATABASECHANGELOG not yet created (fresh DB).
		if pqErr, ok := err.(*pq.Error); ok && pqErr.Code == "42P01" {
			return 0, nil
		}
		return 0, fmt.Errorf("cannot query DATABASECHANGELOG: %w", err)
	}

	return count, nil
}
