package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// env returns a getenv func that reads from the supplied map.
func env(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

// validEnv returns a minimal set of env vars that point at real temp dirs.
// Only Phase 1 vars are included — REGION_ROLE and REGION are Phase 2 and
// must not be required by Phase 1.
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
	}, dest
}

func TestRun_Success(t *testing.T) {
	m, dest := validEnv(t)

	if err := run(env(m)); err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	// Verify the file was actually copied into dest.
	data, err := os.ReadFile(filepath.Join(dest, "changelog-master.xml"))
	if err != nil {
		t.Fatalf("expected copied file in dest, got error: %v", err)
	}
	if string(data) != "<root/>" {
		t.Errorf("unexpected file content: %q", string(data))
	}
}

// TestRun_Phase1DoesNotRequireRegionVars asserts that Phase 1 completes
// successfully when REGION_ROLE and REGION are absent. Region detection
// is a Phase 2 concern and must not block Phase 1.
func TestRun_Phase1DoesNotRequireRegionVars(t *testing.T) {
	m, _ := validEnv(t)

	// Explicitly confirm neither region var is present.
	delete(m, "REGION_ROLE")
	delete(m, "REGION")

	if err := run(env(m)); err != nil {
		t.Fatalf("Phase 1 must not require REGION_ROLE or REGION, got error: %v", err)
	}
}

func TestRun_MissingChangelogSourcePath(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "CHANGELOG_SOURCE_PATH")

	err := run(env(m))
	if err == nil {
		t.Fatal("expected error for missing CHANGELOG_SOURCE_PATH, got nil")
	}
	if !strings.Contains(err.Error(), "CHANGELOG_SOURCE_PATH") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestRun_MissingChangelogDestPath(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "CHANGELOG_DEST_PATH")

	err := run(env(m))
	if err == nil {
		t.Fatal("expected error for missing CHANGELOG_DEST_PATH, got nil")
	}
	if !strings.Contains(err.Error(), "CHANGELOG_DEST_PATH") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestRun_MissingServiceName(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SERVICE_NAME")

	err := run(env(m))
	if err == nil {
		t.Fatal("expected error for missing SERVICE_NAME, got nil")
	}
	if !strings.Contains(err.Error(), "SERVICE_NAME") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestRun_MissingServiceImage(t *testing.T) {
	m, _ := validEnv(t)
	delete(m, "SERVICE_IMAGE")

	err := run(env(m))
	if err == nil {
		t.Fatal("expected error for missing SERVICE_IMAGE, got nil")
	}
	if !strings.Contains(err.Error(), "SERVICE_IMAGE") {
		t.Errorf("error should mention the missing var, got: %v", err)
	}
}

func TestRun_SrcDoesNotExist(t *testing.T) {
	m, _ := validEnv(t)
	m["CHANGELOG_SOURCE_PATH"] = filepath.Join(t.TempDir(), "nonexistent")

	err := run(env(m))
	if err == nil {
		t.Fatal("expected error when src does not exist, got nil")
	}
}

func TestRun_EmptyAllEnvVars(t *testing.T) {
	err := run(env(map[string]string{}))
	if err == nil {
		t.Fatal("expected error when all env vars are empty, got nil")
	}
}
