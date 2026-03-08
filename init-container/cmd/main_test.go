package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// env returns a getenv func that reads from the supplied map.
func env(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

// freshDB simulates a brand-new database with no applied changesets.
// Returned latestChangeset is "" which causes Decide to return ActionUpdate.
func freshDB(_ context.Context, _ string) (string, error) {
	return "", nil
}

// migratedDB simulates a database that already has changesets applied.
// Returned latestChangeset causes Decide to return ActionNone.
func migratedDB(_ context.Context, _ string) (string, error) {
	return "create-users-table", nil
}

// failDB simulates a database that cannot be reached.
func failDB(_ context.Context, _ string) (string, error) {
	return "", fmt.Errorf("connection refused")
}

// validEnv returns the minimal set of env vars needed by execute.
// Only Phase 1 + Phase 2 vars are included.
// REGION_ROLE and REGION are Phase 4 and must not be required here.
func validEnv(t *testing.T) (map[string]string, string) {
	t.Helper()
	src := t.TempDir()
	dest := t.TempDir()

	// Populate src with at least one changelog file so the copy has work to do.
	if err := os.WriteFile(filepath.Join(src, "changelog-master.xml"), []byte("<root/>"), 0o644); err != nil {
		t.Fatal(err)
	}

	return map[string]string{
		"CHANGELOG_SOURCE_PATH": src,
		"CHANGELOG_DEST_PATH":   dest,
		"SERVICE_NAME":          "test-service",
		"SERVICE_IMAGE":         "test-image:latest",
		"DB_URL":                "jdbc:postgresql://localhost:5432/testdb",
		"DB_USERNAME":           "testuser",
		"DB_PASSWORD":           "testpass",
	}, dest
}

// ── Phase 1 + 2 happy paths ───────────────────────────────────────────────

func TestExecute_FreshDB_CopiesChangelogsAndWritesUpdateAction(t *testing.T) {
	m, dest := validEnv(t)

	if err := execute(env(m), freshDB); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Changelog file must be copied.
	data, err := os.ReadFile(filepath.Join(dest, "changelog-master.xml"))
	if err != nil {
		t.Fatalf("expected copied file in dest, got error: %v", err)
	}
	if string(data) != "<root/>" {
		t.Errorf("unexpected file content: %q", string(data))
	}

	// migration-action.txt must contain "update" for a fresh database.
	action, err := os.ReadFile(filepath.Join(dest, "migration-action.txt"))
	if err != nil {
		t.Fatalf("expected migration-action.txt in dest, got error: %v", err)
	}
	if string(action) != "update" {
		t.Errorf("expected action %q for fresh DB, got %q", "update", string(action))
	}
}

func TestExecute_AlreadyMigrated_WritesNoneAction(t *testing.T) {
	m, dest := validEnv(t)

	if err := execute(env(m), migratedDB); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	action, err := os.ReadFile(filepath.Join(dest, "migration-action.txt"))
	if err != nil {
		t.Fatalf("expected migration-action.txt in dest, got error: %v", err)
	}
	if string(action) != "none" {
		t.Errorf("expected action %q for already-migrated DB, got %q", "none", string(action))
	}
}

// TestExecute_NoRegionVarsRequired asserts Phase 4 vars are not required now.
func TestExecute_NoRegionVarsRequired(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "REGION_ROLE")
	delete(m, "REGION")

	if err := execute(env(m), freshDB); err != nil {
		t.Fatalf("REGION_ROLE and REGION must not be required (Phase 4), got: %v", err)
	}
}

// ── Missing Phase 1 env vars ──────────────────────────────────────────────

func TestExecute_MissingChangelogSourcePath(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "CHANGELOG_SOURCE_PATH")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing CHANGELOG_SOURCE_PATH, got nil")
	}
	if !strings.Contains(err.Error(), "CHANGELOG_SOURCE_PATH") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestExecute_MissingChangelogDestPath(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "CHANGELOG_DEST_PATH")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing CHANGELOG_DEST_PATH, got nil")
	}
	if !strings.Contains(err.Error(), "CHANGELOG_DEST_PATH") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestExecute_MissingServiceName(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SERVICE_NAME")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing SERVICE_NAME, got nil")
	}
	if !strings.Contains(err.Error(), "SERVICE_NAME") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestExecute_MissingServiceImage(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SERVICE_IMAGE")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing SERVICE_IMAGE, got nil")
	}
	if !strings.Contains(err.Error(), "SERVICE_IMAGE") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

// ── Missing Phase 2 env vars ──────────────────────────────────────────────

func TestExecute_MissingDBURL(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "DB_URL")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing DB_URL, got nil")
	}
	if !strings.Contains(err.Error(), "DB_URL") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestExecute_MissingDBUsername(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "DB_USERNAME")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing DB_USERNAME, got nil")
	}
	if !strings.Contains(err.Error(), "DB_USERNAME") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestExecute_MissingDBPassword(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "DB_PASSWORD")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error for missing DB_PASSWORD, got nil")
	}
	if !strings.Contains(err.Error(), "DB_PASSWORD") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

// ── Other failure modes ────────────────────────────────────────────────────

func TestExecute_SrcDoesNotExist(t *testing.T) {
	m, _ := validEnv(t)
	m["CHANGELOG_SOURCE_PATH"] = filepath.Join(t.TempDir(), "nonexistent")

	err := execute(env(m), freshDB)
	if err == nil {
		t.Fatal("expected error when src does not exist, got nil")
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

func TestExecute_EmptyAllEnvVars(t *testing.T) {
	err := execute(env(map[string]string{}), freshDB)
	if err == nil {
		t.Fatal("expected error when all env vars are empty, got nil")
	}
}
