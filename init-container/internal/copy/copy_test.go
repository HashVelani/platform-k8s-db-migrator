package copy

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCopyChangelogs_Success(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	if err := os.MkdirAll(filepath.Join(src, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(src, "changelog-master.xml"), "<root/>")
	writeFile(t, filepath.Join(src, "subdir", "v1.xml"), "<change/>")

	srcExists, err := CopyChangelogs(src, dest)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !srcExists {
		t.Fatal("expected srcExists=true for existing src, got false")
	}

	assertFileContent(t, filepath.Join(dest, "changelog-master.xml"), "<root/>")
	assertFileContent(t, filepath.Join(dest, "subdir", "v1.xml"), "<change/>")
}

func TestCopyChangelogs_MissingSrc(t *testing.T) {
	src := filepath.Join(t.TempDir(), "does-not-exist")
	dest := t.TempDir()

	srcExists, err := CopyChangelogs(src, dest)
	if err != nil {
		t.Fatalf("expected no error for missing src (graceful exit), got: %v", err)
	}
	if srcExists {
		t.Fatal("expected srcExists=false when src does not exist, got true")
	}
}

func TestCopyChangelogs_SrcIsFile(t *testing.T) {
	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "file.xml")
	writeFile(t, src, "<root/>")
	dest := t.TempDir()

	_, err := CopyChangelogs(src, dest)
	if err == nil {
		t.Fatal("expected error when src is a file, not a directory")
	}
}

func TestCopyChangelogs_DestCreatedIfMissing(t *testing.T) {
	src := t.TempDir()
	writeFile(t, filepath.Join(src, "v1.xml"), "<change/>")

	dest := filepath.Join(t.TempDir(), "newdir")

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "v1.xml"), "<change/>")
}

func TestCopyChangelogs_EmptySrc(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error for empty src: %v", err)
	}
}

func TestCopyChangelogs_DeeplyNested(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	deep := filepath.Join(src, "a", "b", "c")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFile(t, filepath.Join(deep, "deep.xml"), "<deep/>")

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "a", "b", "c", "deep.xml"), "<deep/>")
}

func TestCopyChangelogs_OverwritesExistingFile(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	writeFile(t, filepath.Join(src, "v1.xml"), "new-content")
	writeFile(t, filepath.Join(dest, "v1.xml"), "stale-content")

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "v1.xml"), "new-content")
}

func TestCopyChangelogs_PreservesFileMode(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	srcFile := filepath.Join(src, "script.sh")
	writeFile(t, srcFile, "#!/bin/sh")
	if err := os.Chmod(srcFile, 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(dest, "script.sh"))
	if err != nil {
		t.Fatalf("cannot stat copied file: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o755 {
		t.Errorf("expected mode 0755, got %04o", got)
	}
}

func TestCopyChangelogs_MultipleFilesAtRoot(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	for _, name := range []string{"a.xml", "b.xml", "c.xml"} {
		writeFile(t, filepath.Join(src, name), name)
	}

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, name := range []string{"a.xml", "b.xml", "c.xml"} {
		assertFileContent(t, filepath.Join(dest, name), name)
	}
}

// TestCopyChangelogs_EmptyFile verifies that a zero-byte file is copied
// without error and the destination is also empty.
func TestCopyChangelogs_EmptyFile(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	writeFile(t, filepath.Join(src, "empty.xml"), "")

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error copying empty file: %v", err)
	}

	assertFileContent(t, filepath.Join(dest, "empty.xml"), "")
}

// TestCopyChangelogs_BinaryContent verifies that binary (non-UTF-8) data is
// copied byte-for-byte without corruption.
func TestCopyChangelogs_BinaryContent(t *testing.T) {
	src := t.TempDir()
	dest := t.TempDir()

	binary := []byte{0x00, 0xFF, 0xFE, 0x0D, 0x0A, 0xDE, 0xAD, 0xBE, 0xEF}
	if err := os.WriteFile(filepath.Join(src, "binary.xml"), binary, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, err := CopyChangelogs(src, dest); err != nil {
		t.Fatalf("unexpected error copying binary file: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dest, "binary.xml"))
	if err != nil {
		t.Fatalf("cannot read copied binary file: %v", err)
	}
	if string(got) != string(binary) {
		t.Errorf("binary content mismatch: got %v, want %v", got, binary)
	}
}

// TestCopyChangelogs_UnreadableSourceFile verifies that a source file which
// exists but cannot be opened causes CopyChangelogs to return an error.
func TestCopyChangelogs_UnreadableSourceFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root can read any file")
	}

	src := t.TempDir()
	dest := t.TempDir()

	unreadable := filepath.Join(src, "locked.xml")
	writeFile(t, unreadable, "<changeSet/>")
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(unreadable, 0o644) //nolint:errcheck

	_, err := CopyChangelogs(src, dest)
	if err == nil {
		t.Fatal("expected error for unreadable source file, got nil")
	}
}

// TestCopyChangelogs_UnwritableDest verifies that a destination directory
// with no write permission causes CopyChangelogs to return an error.
func TestCopyChangelogs_UnwritableDest(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root can write anywhere")
	}

	src := t.TempDir()
	writeFile(t, filepath.Join(src, "v1.xml"), "<changeSet/>")

	dest := t.TempDir()
	if err := os.Chmod(dest, 0o555); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(dest, 0o755) //nolint:errcheck

	_, err := CopyChangelogs(src, dest)
	if err == nil {
		t.Fatal("expected error for unwritable destination, got nil")
	}
}

// TestCopyChangelogs_InaccessibleSourcePath covers the second os.Stat error
// branch in CopyChangelogs — a path that exists but cannot be stat'd (e.g.
// inside a directory with no execute permission). This is distinct from the
// "does not exist" path.
func TestCopyChangelogs_InaccessibleSourcePath(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root can stat any path")
	}

	// Create a parent dir with no execute bit so that os.Stat on a child path
	// returns "permission denied" (not "not found").
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(parent, 0o755) //nolint:errcheck

	src := filepath.Join(parent, "nested")
	_, err := CopyChangelogs(src, t.TempDir())
	if err == nil {
		t.Fatal("expected error for inaccessible source path, got nil")
	}
	// Must NOT say "does not exist" — it should say "cannot access".
	if strings.Contains(err.Error(), "does not exist") {
		t.Errorf("expected 'cannot access' error not 'does not exist', got: %v", err)
	}
}

// ── helpers ───────────────────────────────────────────────────────────────────

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
