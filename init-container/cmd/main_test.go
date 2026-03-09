package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ── fixtures ─────────────────────────────────────────────────────────────────

// changelogXML is a minimal Liquibase changelog containing exactly one
// <changeSet element. This lets CountChangeSets return 1 in unit tests.
const changelogXML = `<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <changeSet id="1-create-users" author="platform">
    <createTable tableName="users">
      <column name="id" type="BIGINT" autoIncrement="true"/>
      <column name="name" type="VARCHAR(255)"/>
    </createTable>
  </changeSet>
</databaseChangeLog>`

// twoChangesetXML has two <changeSet elements, for tests that need
// changelogCount=2.
const twoChangesetXML = `<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <changeSet id="1-create-users" author="platform">
    <createTable tableName="users"><column name="id" type="BIGINT"/></createTable>
  </changeSet>
  <changeSet id="2-add-email" author="platform">
    <addColumn tableName="users"><column name="email" type="VARCHAR(255)"/></addColumn>
  </changeSet>
</databaseChangeLog>`

// ── mock queryDB functions ────────────────────────────────────────────────────

// freshDB simulates a brand-new database: DATABASECHANGELOG does not exist yet.
// changelogCount(1) > dbCount(0) → Decide returns ActionUpdate.
func freshDB(_ context.Context, _ string) (int, error) { return 0, nil }

// migratedDB simulates a database where exactly 1 changeset has been applied.
// Matches the single changeset in changelogXML → changelogCount(1)==dbCount(1)
// → Decide returns ActionNone.
func migratedDB(_ context.Context, _ string) (int, error) { return 1, nil }

// partiallyMigratedDB simulates a DB with 1 changeset applied when the
// changelog now has 2. changelogCount(2) > dbCount(1) → ActionUpdate.
func partiallyMigratedDB(_ context.Context, _ string) (int, error) { return 1, nil }

// failDB simulates an unreachable database.
func failDB(_ context.Context, _ string) (int, error) {
	return 0, fmt.Errorf("connection refused")
}

// aheadDB simulates a DB that has 2 applied changesets when the changelog
// only contains 1 (<changeSet in changelogXML). This is the "changeset
// removed from changelog" scenario — Decide must return an error.
func aheadDB(_ context.Context, _ string) (int, error) { return 2, nil }

// ── env helpers ───────────────────────────────────────────────────────────────

// env returns a getenv func that reads from the supplied map.
func env(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

// validEnv returns the full set of env vars needed by execute, backed by real
// temp directories. The src dir is seeded with changelogXML (1 changeSet).
// REGION_ROLE and REGION are intentionally absent — they are Phase 4.
func validEnv(t *testing.T) (map[string]string, string) {
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
		"CHANGELOG_SOURCE_PATH":   src,
		"CHANGELOG_DEST_PATH":     dest,
		"SERVICE_NAME":            "test-service",
		"SERVICE_IMAGE":           "test-image:latest",
		"SPRING_DATASOURCE_URL":      "jdbc:postgresql://localhost:5432/testdb",
		"SPRING_DATASOURCE_USERNAME": "testuser",
		"SPRING_DATASOURCE_PASSWORD": "testpass",
	}, dest
}

// ── Phase 1 + 2 happy-path tests ─────────────────────────────────────────────

func TestExecute_FreshDB_WritesUpdateAction(t *testing.T) {
	m, dest := validEnv(t)

	if err := execute(env(m), freshDB); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Changelog must be copied.
	assertFileContent(t, filepath.Join(dest, "changelog-master.xml"), changelogXML)

	// Fresh DB (0 applied) vs 1 changelog changeset → update.
	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "update")
}

func TestExecute_AlreadyMigrated_WritesNoneAction(t *testing.T) {
	m, dest := validEnv(t)

	if err := execute(env(m), migratedDB); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// 1 applied == 1 changelog → none.
	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "none")
}

func TestExecute_NewVersionDeployed_WritesUpdateAction(t *testing.T) {
	m, dest := validEnv(t)
	// Replace the fixture with a changelog that has 2 changesets.
	if err := os.WriteFile(
		filepath.Join(m["CHANGELOG_SOURCE_PATH"], "changelog-master.xml"),
		[]byte(twoChangesetXML),
		0o644,
	); err != nil {
		t.Fatal(err)
	}

	// partiallyMigratedDB returns dbCount=1; changelog has 2 → update.
	if err := execute(env(m), partiallyMigratedDB); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "update")
}

func TestExecute_NoChangelogFiles_ExitsGracefully(t *testing.T) {
	m, dest := validEnv(t)
	// Override src with a directory that has no XML files at all.
	m["CHANGELOG_SOURCE_PATH"] = t.TempDir()

	// failDB proves the database is never queried — if it were, the test
	// would fail with "connection refused".
	if err := execute(env(m), failDB); err != nil {
		t.Fatalf("expected graceful exit when no changelog files exist, got: %v", err)
	}

	// migration-action.txt must NOT be written: there is nothing to act on.
	if _, err := os.Stat(filepath.Join(dest, "migration-action.txt")); !os.IsNotExist(err) {
		t.Error("migration-action.txt must not be written when no changelog files are present")
	}
}

func TestExecute_NonXMLFilesOnly_ExitsGracefully(t *testing.T) {
	m, dest := validEnv(t)
	// Src contains files, but none are XML (service doesn't use Liquibase yet).
	nonXMLSrc := t.TempDir()
	if err := os.WriteFile(filepath.Join(nonXMLSrc, "README.md"), []byte("docs"), 0o644); err != nil {
		t.Fatal(err)
	}
	m["CHANGELOG_SOURCE_PATH"] = nonXMLSrc

	if err := execute(env(m), failDB); err != nil {
		t.Fatalf("expected graceful exit for non-XML-only src, got: %v", err)
	}

	if _, err := os.Stat(filepath.Join(dest, "migration-action.txt")); !os.IsNotExist(err) {
		t.Error("migration-action.txt must not be written when no changelog files are present")
	}
}

func TestExecute_NoRegionVarsRequired(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "REGION_ROLE")
	delete(m, "REGION")

	if err := execute(env(m), freshDB); err != nil {
		t.Fatalf("REGION_ROLE and REGION must not be required (Phase 4), got: %v", err)
	}
}

// ── Missing Phase 1 env vars ──────────────────────────────────────────────────

func TestExecute_MissingChangelogSourcePath(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "CHANGELOG_SOURCE_PATH")
	assertMissingVar(t, m, "CHANGELOG_SOURCE_PATH")
}

func TestExecute_MissingChangelogDestPath(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "CHANGELOG_DEST_PATH")
	assertMissingVar(t, m, "CHANGELOG_DEST_PATH")
}

func TestExecute_MissingServiceName(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SERVICE_NAME")
	assertMissingVar(t, m, "SERVICE_NAME")
}

func TestExecute_MissingServiceImage(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SERVICE_IMAGE")
	assertMissingVar(t, m, "SERVICE_IMAGE")
}

// ── Missing Phase 2 env vars ──────────────────────────────────────────────────

func TestExecute_MissingSpringDatasourceURL(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SPRING_DATASOURCE_URL")
	assertMissingVar(t, m, "SPRING_DATASOURCE_URL")
}

func TestExecute_MissingSpringDatasourceUsername(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SPRING_DATASOURCE_USERNAME")
	assertMissingVar(t, m, "SPRING_DATASOURCE_USERNAME")
}

func TestExecute_MissingSpringDatasourcePassword(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SPRING_DATASOURCE_PASSWORD")
	assertMissingVar(t, m, "SPRING_DATASOURCE_PASSWORD")
}

// ── Other failure modes ───────────────────────────────────────────────────────

func TestExecute_SrcDoesNotExist(t *testing.T) {
	m, dest := validEnv(t)
	m["CHANGELOG_SOURCE_PATH"] = filepath.Join(t.TempDir(), "nonexistent")

	if err := execute(env(m), freshDB); err != nil {
		t.Fatalf("expected graceful exit when src does not exist, got: %v", err)
	}

	// migration-action.txt must NOT be written: the DB check is skipped.
	if _, err := os.Stat(filepath.Join(dest, "migration-action.txt")); !os.IsNotExist(err) {
		t.Error("migration-action.txt must not be written when source path does not exist")
	}

	// An empty changelog must be written so Liquibase can find a valid file,
	// read no changesets, and exit 0 instead of throwing ChangeLogParseException.
	data, err := os.ReadFile(filepath.Join(dest, "db.changelog-master.xml"))
	if err != nil {
		t.Fatalf("empty changelog must be written to dest when source path does not exist: %v", err)
	}
	if !strings.Contains(string(data), "<databaseChangeLog") {
		t.Errorf("empty changelog must contain <databaseChangeLog> element, got: %s", data)
	}
	if strings.Contains(string(data), "<changeSet") {
		t.Error("empty changelog must not contain any <changeSet elements")
	}
}

func TestExecute_DBQueryFails(t *testing.T) {
	m, _ := validEnv(t)

	err := execute(env(m), failDB)
	if err == nil {
		t.Fatal("expected error when DB query fails, got nil")
	}
	if !strings.Contains(err.Error(), "failed to query database") {
		t.Errorf("error should mention database failure, got: %v", err)
	}
}

// TestExecute_InvalidDBURLFormat verifies that an unsupported URL scheme
// (e.g. mysql://) is caught by ParseDSN before any DB connection is attempted.
func TestExecute_InvalidDBURLFormat_ReturnsError(t *testing.T) {
	m, _ := validEnv(t)
	m["SPRING_DATASOURCE_URL"] = "mysql://host/db" // unsupported scheme

	err := execute(env(m), failDB) // failDB would error if called — it must not be
	if err == nil {
		t.Fatal("expected error for unsupported DB URL scheme, got nil")
	}
	if !strings.Contains(err.Error(), "invalid database URL") {
		t.Errorf("error should mention invalid database URL, got: %v", err)
	}
}

// TestExecute_ChangesetRemovedFromChangelog_ReturnsError covers the Decide
// error path: DB has more applied changesets than the changelog contains.
// This signals a changelog file was deleted — manual intervention required.
func TestExecute_ChangesetRemovedFromChangelog_ReturnsError(t *testing.T) {
	m, _ := validEnv(t)
	// changelogXML has 1 <changeSet; aheadDB returns 2 → Decide errors.
	if err := execute(env(m), aheadDB); err == nil {
		t.Fatal("expected error when DB is ahead of the changelog, got nil")
	} else if !strings.Contains(err.Error(), "failed to determine migration action") {
		t.Errorf("error should mention migration action decision failure, got: %v", err)
	}
}

// TestExecute_ActionFileOverwrite_OnRerun verifies that running execute twice
// on the same dest directory overwrites migration-action.txt with the updated
// decision — the file must never contain a stale value.
func TestExecute_ActionFileOverwrite_OnRerun(t *testing.T) {
	m, dest := validEnv(t)

	// First run: fresh DB → "update".
	if err := execute(env(m), freshDB); err != nil {
		t.Fatalf("first run: unexpected error: %v", err)
	}
	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "update")

	// Second run: same dest, now DB is up to date → "none".
	if err := execute(env(m), migratedDB); err != nil {
		t.Fatalf("second run: unexpected error: %v", err)
	}
	assertFileContent(t, filepath.Join(dest, "migration-action.txt"), "none")
}

func TestExecute_EmptyAllEnvVars(t *testing.T) {
	err := execute(env(map[string]string{}), freshDB)
	if err == nil {
		t.Fatal("expected error when all env vars are empty, got nil")
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

func assertMissingVar(t *testing.T, m map[string]string, varName string) {
	t.Helper()
	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatalf("expected error for missing %s, got nil", varName)
	}
	if !strings.Contains(err.Error(), varName) {
		t.Errorf("error should mention %q, got: %v", varName, err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %q: %v", path, err)
	}
	if got := string(data); got != want {
		t.Errorf("file %q:\n  got  %q\n  want %q", path, got, want)
	}
}
