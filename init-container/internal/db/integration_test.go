//go:build integration

package db

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgres starts a PostgreSQL container and returns its DSN and a cleanup func.
func startPostgres(t *testing.T) (dsn string, cleanup func()) {
	t.Helper()
	ctx := context.Background()

	req := testcontainers.ContainerRequest{
		Image:        "postgres:16-alpine",
		ExposedPorts: []string{"5432/tcp"},
		Env: map[string]string{
			"POSTGRES_DB":       "testdb",
			"POSTGRES_USER":     "testuser",
			"POSTGRES_PASSWORD": "testpass",
		},
		WaitingFor: wait.ForLog("database system is ready to accept connections").
			WithOccurrence(2).
			WithStartupTimeout(60 * time.Second),
	}

	container, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: req,
		Started:          true,
	})
	if err != nil {
		t.Fatalf("failed to start PostgreSQL container: %v", err)
	}

	host, err := container.Host(ctx)
	if err != nil {
		t.Fatalf("failed to get container host: %v", err)
	}
	port, err := container.MappedPort(ctx, "5432")
	if err != nil {
		t.Fatalf("failed to get container port: %v", err)
	}

	dsn = fmt.Sprintf("postgresql://testuser:testpass@%s:%s/testdb?sslmode=disable", host, port.Port())
	cleanup = func() {
		if err := container.Terminate(ctx); err != nil {
			t.Logf("warning: failed to terminate PostgreSQL container: %v", err)
		}
	}
	return dsn, cleanup
}

func TestGetAppliedChangesetCount_FreshDatabase(t *testing.T) {
	dsn, cleanup := startPostgres(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	count, err := GetAppliedChangesetCount(ctx, dsn)
	if err != nil {
		t.Fatalf("unexpected error on fresh database: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 changesets on fresh database, got %d", count)
	}
}

func TestGetAppliedChangesetCount_WithAppliedChangesets(t *testing.T) {
	dsn, cleanup := startPostgres(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := openDB(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	seedDatabaseChangeLog(t, ctx, db, 3)

	count, err := GetAppliedChangesetCount(ctx, dsn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3 applied changesets, got %d", count)
	}
}

func TestGetAppliedChangesetCount_EmptyTable(t *testing.T) {
	dsn, cleanup := startPostgres(t)
	defer cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	db, err := openDB(ctx, dsn)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	// Create the table but insert no rows — edge case Liquibase can produce.
	createDatabaseChangeLogTable(t, ctx, db)

	count, err := GetAppliedChangesetCount(ctx, dsn)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for empty table, got %d", count)
	}
}

// TestGetAppliedChangesetCount_ParseDSN_RoundTrip verifies that a DSN
// produced by ParseDSN can open a real connection.
func TestGetAppliedChangesetCount_ParseDSN_RoundTrip(t *testing.T) {
	rawDSN, cleanup := startPostgres(t)
	defer cleanup()

	// Use ParseDSN with the plain postgresql:// URL as input.
	dsn, err := ParseDSN(rawDSN, "testuser", "testpass")
	if err != nil {
		t.Fatalf("ParseDSN failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	// If DSN is well-formed the count will succeed (no error = credentials embedded correctly).
	_, err = GetAppliedChangesetCount(ctx, dsn)
	if err != nil {
		t.Fatalf("DSN round-trip failed — could not connect: %v", err)
	}
}

// ── internal helpers ──────────────────────────────────────────────────────────

func openDB(ctx context.Context, dsn string) (*sql.DB, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, fmt.Errorf("sql.Open: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return db, nil
}

func createDatabaseChangeLogTable(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	_, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS databasechangelog (
			id            VARCHAR(255) NOT NULL,
			author        VARCHAR(255) NOT NULL,
			filename      VARCHAR(255) NOT NULL,
			dateexecuted  TIMESTAMP    NOT NULL,
			orderexecuted INT          NOT NULL,
			exectype      VARCHAR(10)  NOT NULL,
			md5sum        VARCHAR(35),
			description   VARCHAR(255),
			comments      VARCHAR(255),
			tag           VARCHAR(255),
			liquibase     VARCHAR(20),
			contexts      VARCHAR(255),
			labels        VARCHAR(255),
			deployment_id VARCHAR(10)
		)`)
	if err != nil {
		t.Fatalf("failed to create DATABASECHANGELOG: %v", err)
	}
}

func seedDatabaseChangeLog(t *testing.T, ctx context.Context, db *sql.DB, n int) {
	t.Helper()
	createDatabaseChangeLogTable(t, ctx, db)
	for i := 1; i <= n; i++ {
		_, err := db.ExecContext(ctx, `
			INSERT INTO databasechangelog
				(id, author, filename, dateexecuted, orderexecuted, exectype)
			VALUES ($1, $2, $3, NOW(), $4, 'EXECUTED')`,
			fmt.Sprintf("changeset-%d", i), "test", "changelog.xml", i)
		if err != nil {
			t.Fatalf("failed to seed changeset %d: %v", i, err)
		}
	}
}

