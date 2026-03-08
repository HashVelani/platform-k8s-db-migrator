//go:build integration

package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
)

// startPostgresForCmd starts a PostgreSQL container and returns a plain
// postgresql:// DSN and a cleanup function.
func startPostgresForCmd(t *testing.T) (dsn string, cleanup func()) {
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
			t.Logf("warning: failed to terminate container: %v", err)
		}
	}
	return dsn, cleanup
}

// integrationEnv builds a validEnv map pointing at a real PostgreSQL container.
// The src dir is seeded with a changelog containing one <changeSet.
func integrationEnv(t *testing.T, dsn string) (map[string]string, string) {
	t.Helper()
	src := t.TempDir()
	dest := t.TempDir()

	if err := os.WriteFile(
		filepath.Join(src, "changelog-master.xml"),
		[]byte(changelogXML),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	return map[string]string{
		"CHANGELOG_SOURCE_PATH":      src,
		"CHANGELOG_DEST_PATH":        dest,
		"SERVICE_NAME":               "integration-test-service",
		"SERVICE_IMAGE":              "test-image:latest",
		"SPRING_DATASOURCE_URL":      dsn,
		"SPRING_DATASOURCE_USERNAME": "testuser",
		"SPRING_DATASOURCE_PASSWORD": "testpass",
	}, dest
}

// TestExecute_Integration_FreshDatabase verifies the full execute path against
// a real PostgreSQL instance: fresh DB should produce action=update.
func TestExecute_Integration_FreshDatabase(t *testing.T) {
	dsn, cleanup := startPostgresForCmd(t)
	defer cleanup()

	m, dest := integrationEnv(t, dsn)

	if err := run(env(m)); err != nil {
		t.Fatalf("execute failed on fresh database: %v", err)
	}

	// Changelog file must exist in dest.
	if _, err := os.Stat(filepath.Join(dest, "changelog-master.xml")); err != nil {
		t.Errorf("expected changelog-master.xml in dest: %v", err)
	}

	// Fresh DB → update.
	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "update")
}

// TestExecute_Integration_AlreadyMigrated seeds DATABASECHANGELOG with the
// same number of changesets as the fixture changelog and expects action=none.
func TestExecute_Integration_AlreadyMigrated(t *testing.T) {
	dsn, cleanup := startPostgresForCmd(t)
	defer cleanup()

	// Seed the DB so dbCount == changelogCount (1 each).
	seedDB(t, dsn, 1)

	m, dest := integrationEnv(t, dsn)

	if err := run(env(m)); err != nil {
		t.Fatalf("execute failed on already-migrated database: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "none")
}

// TestExecute_Integration_NewVersionDeployed seeds DATABASECHANGELOG with 1
// row but the changelog has 2 changesets, expecting action=update.
func TestExecute_Integration_NewVersionDeployed(t *testing.T) {
	dsn, cleanup := startPostgresForCmd(t)
	defer cleanup()

	// Seed DB with 1 applied changeset.
	seedDB(t, dsn, 1)

	m, dest := integrationEnv(t, dsn)
	// Override changelog fixture with one that has 2 changesets.
	if err := os.WriteFile(
		filepath.Join(m["CHANGELOG_SOURCE_PATH"], "changelog-master.xml"),
		[]byte(twoChangesetXML),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	if err := run(env(m)); err != nil {
		t.Fatalf("execute failed on new-version deployment: %v", err)
	}

	// changelogCount(2) > dbCount(1) → update.
	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "update")
}

// ── helpers ───────────────────────────────────────────────────────────────────

func seedDB(t *testing.T, dsn string, n int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Fatalf("seedDB: sql.Open: %v", err)
	}
	defer db.Close()

	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("seedDB: ping: %v", err)
	}

	_, err = db.ExecContext(ctx, `
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
		t.Fatalf("seedDB: create table: %v", err)
	}

	for i := 1; i <= n; i++ {
		_, err = db.ExecContext(ctx, `
			INSERT INTO databasechangelog
				(id, author, filename, dateexecuted, orderexecuted, exectype)
			VALUES ($1, $2, $3, NOW(), $4, 'EXECUTED')`,
			fmt.Sprintf("changeset-%d", i), "platform", "changelog-master.xml", i)
		if err != nil {
			t.Fatalf("seedDB: insert changeset %d: %v", i, err)
		}
	}
}
