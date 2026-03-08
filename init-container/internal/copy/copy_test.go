package copy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCopyChangelogs_Success(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	// Create a nested file structure in src.
	if err := os.MkdirAll(filepath.Join(src, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "changelog-master.xml"), "<root/>")
	writeFile(t, filepath.Join(src, "subdir", "v1.xml"), "<change/>")

	if err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "changelog-master.xml"), "<root/>")
	assertFileContent(t, filepath.Join(dest, "subdir", "v1.xml"), "<change/>")
}

func TestCopyChangelogs_MissingSrc(t *testing.T) {
	src := filepath.Join(t.TempDir(), "does-not-exist")
	dest := t.TempDir()

	err := CopyChangelogs(src, dest)
	if err == nil {
		t.Fatal("expected error for missing src, got nil")
	}
}

func TestCopyChangelogs_SrcIsFile(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "file.xml")
	writeFile(t, src, "<root/>")
	dest := t.TempDir()

	err := CopyChangelogs(src, dest)
	if err == nil {
		t.Fatal("expected error when src is a file, not a directory")
	}
}

func TestCopyChangelogs_DestCreatedIfMissing(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "v1.xml"), "<change/>")

	// dest does not exist yet; CopyChangelogs should create it.
	dest := filepath.Join(t.TempDir(), "newdir")

	if err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "v1.xml"), "<change/>")
}

func TestCopyChangelogs_EmptySrc(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	// Should succeed even when src is empty.
	if err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error for empty src: %v", err)
	}
}

// helpers

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("cannot read %q: %v", path, err)
	}
	if got := string(data); got != want {
		t.Errorf("file %q: got %q, want %q", path, got, want)
	}
}
