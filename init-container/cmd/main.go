package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/changelog"
	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/copy"
	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/db"
	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/decision"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(os.Getenv); err != nil {
		slog.Error("init container failed", "error", err)
		os.Exit(1)
	}
}

// run is the production entry point. It wires the real database querier into
// execute so that main() stays a three-line wrapper.
func run(getenv func(string) string) error {
	return execute(getenv, db.GetAppliedChangesetCount)
}

// changelogMasterFilename is the single source of truth for the master
// changelog filename. It must match the --changeLogFile arg in the Liquibase
// Job container spec and the file present in the service image at
// src/main/resources/db/changelog/db.changelog-master.xml.
const changelogMasterFilename = "db.changelog-master.xml"

// emptyChangelog is a minimal valid Liquibase changelog with no changesets.
// Liquibase reads it, finds no changesets, logs "up to date", and exits 0.
const emptyChangelog = `<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog
    xmlns="http://www.liquibase.org/xml/ns/dbchangelog"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance"
    xsi:schemaLocation="http://www.liquibase.org/xml/ns/dbchangelog
        http://www.liquibase.org/xml/ns/dbchangelog/dbchangelog-4.20.xsd">
</databaseChangeLog>`

// writeEmptyChangelog writes emptyChangelog to dest/changelogMasterFilename so
// that the Liquibase container finds a valid (empty) changelog and exits 0
// without applying any changesets. This handles services that have no database.
func writeEmptyChangelog(dest string) error {
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return fmt.Errorf("cannot create changelog destination %q: %w", dest, err)
	}
	path := filepath.Join(dest, changelogMasterFilename)
	if err := os.WriteFile(path, []byte(emptyChangelog), 0o644); err != nil {
		return fmt.Errorf("cannot write empty changelog to %q: %w", path, err)
	}
	slog.Info("wrote empty changelog for service with no database", "path", path)
	return nil
}

// execute is the testable core. queryDB is injected so tests can supply a
// controlled implementation without requiring a live PostgreSQL instance.
//
// queryDB must return the number of changesets already applied in
// DATABASECHANGELOG (0 for a fresh database).
func execute(getenv func(string) string, queryDB func(context.Context, string) (int, error)) error {
	slog.Info("platform-k8s-db-migrator init container starting")

	// ── Phase 1: copy changelogs from service image into shared emptyDir ──────

	src, err := mustGetenv(getenv, "CHANGELOG_SOURCE_PATH")
	if err != nil {
		return err
	}
	dest, err := mustGetenv(getenv, "CHANGELOG_DEST_PATH")
	if err != nil {
		return err
	}
	serviceName, err := mustGetenv(getenv, "SERVICE_NAME")
	if err != nil {
		return err
	}
	serviceImage, err := mustGetenv(getenv, "SERVICE_IMAGE")
	if err != nil {
		return err
	}

	slog.Info("copying changelogs",
		"service", serviceName,
		"image", serviceImage,
		"src", src,
		"dest", dest,
	)

	srcExists, err := copy.CopyChangelogs(src, dest)
	if err != nil {
		return fmt.Errorf("failed to copy changelogs: %w", err)
	}
	if !srcExists {
		// Source path did not exist in the service image. Write a minimal valid
		// empty changelog so the Liquibase container finds its expected file,
		// logs "up to date", and exits 0. No DB work is needed.
		return writeEmptyChangelog(dest)
	}

	slog.Info("changelogs copied successfully",
		"service", serviceName,
		"src", src,
		"dest", dest,
	)

	// ── Phase 2: count changelog changesets and query DB state ────────────────
	// Region detection is Phase 4 — not checked here.

	// Count the total changesets present in the service image's changelog
	// directory (the "desired" state). Counting from src — the original service
	// image directory — expresses intent clearly: this is what the service
	// intends to apply, independent of anything already written to dest.
	changelogCount, err := changelog.CountChangeSets(src)
	if err != nil {
		return fmt.Errorf("failed to count changesets in changelog: %w", err)
	}

	if changelogCount == 0 {
		slog.Warn("no Liquibase changelog files found — skipping database check",
			"service", serviceName,
			"src", src,
			"hint", "if this service uses a database, add changelog XML files to the service image at CHANGELOG_SOURCE_PATH",
		)
		return nil
	}

	slog.Info("changelog changesets counted",
		"service", serviceName,
		"changelogCount", changelogCount,
	)

	// Read DB credentials from environment. In Kubernetes these come from the
	// same ConfigMap (and Secret) that the service itself uses with Hikari,
	// so the init container can reference them via envFrom without any remapping.
	dbURL, err := mustGetenv(getenv, "SPRING_DATASOURCE_URL")
	if err != nil {
		return err
	}
	dbUsername, err := mustGetenv(getenv, "SPRING_DATASOURCE_USERNAME")
	if err != nil {
		return err
	}
	dbPassword, err := mustGetenv(getenv, "SPRING_DATASOURCE_PASSWORD")
	if err != nil {
		return err
	}

	dsn, err := db.ParseDSN(dbURL, dbUsername, dbPassword)
	if err != nil {
		return fmt.Errorf("invalid database URL: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	dbCount, err := queryDB(ctx, dsn)
	if err != nil {
		return fmt.Errorf("failed to query database: %w", err)
	}

	slog.Info("database changesets counted",
		"service", serviceName,
		"dbCount", dbCount,
	)

	action, err := decision.Decide(dbCount, changelogCount)
	if err != nil {
		return fmt.Errorf("failed to determine migration action: %w", err)
	}

	// Write the action to a file so the Liquibase main container can read it.
	actionFile := filepath.Join(dest, "migration-action.txt")
	if err := os.WriteFile(actionFile, []byte(action), 0o644); err != nil {
		return fmt.Errorf("failed to write migration action file: %w", err)
	}

	switch action {
	case decision.ActionUpdate:
		slog.Info("migration action: update — new changesets to apply",
			"service", serviceName,
			"dbCount", dbCount,
			"changelogCount", changelogCount,
		)
	case decision.ActionNone:
		slog.Info("migration action: none — database is already up to date",
			"service", serviceName,
			"dbCount", dbCount,
			"changelogCount", changelogCount,
		)
	}

	// ── Phase 3 stub ──────────────────────────────────────────────────────────
	// TODO Phase 3: implement rollback detection
	// Compare applied changesets against the changelog to detect when the
	// deployed changelog is behind the DB (rollback scenario). Write "rollback"
	// to migration-action.txt so the Liquibase runner can apply rollback scripts.

	// ── Phase 4 stub ──────────────────────────────────────────────────────────
	// TODO Phase 4: implement region-based block logic
	// 1. call region.Detect() → returns RegionRole (primary | replica)
	// 2. if replica and schema mismatch → write "block" to migration-action.txt,
	//    log "Schema mismatch in replica region. Sync EUW1 first.", exit 1

	return nil
}

// mustGetenv returns the value of the environment variable named by key, or
// an error if the variable is empty or unset.
func mustGetenv(getenv func(string) string, key string) (string, error) {
	val := getenv(key)
	if val == "" {
		return "", fmt.Errorf("required environment variable %q is not set", key)
	}
	return val, nil
}
