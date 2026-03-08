package changelog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// helper writes a file and fatals on error.
func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestCountChangeSets_EmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for empty dir, got %d", count)
	}
}

func TestCountChangeSets_SingleChangeSet(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.xml"), `
<databaseChangeLog>
  <changeSet id="1" author="dev">
    <createTable tableName="users"/>
  </changeSet>
</databaseChangeLog>`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1, got %d", count)
	}
}

func TestCountChangeSets_MultipleChangeSetsInOneFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.xml"), `
<databaseChangeLog>
  <changeSet id="1" author="dev"><createTable tableName="a"/></changeSet>
  <changeSet id="2" author="dev"><createTable tableName="b"/></changeSet>
  <changeSet id="3" author="dev"><createTable tableName="c"/></changeSet>
</databaseChangeLog>`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestCountChangeSets_MultipleFiles(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.xml"), `<changeSet id="1" author="dev"/>`)
	writeFile(t, filepath.Join(dir, "v2.xml"), `<changeSet id="2" author="dev"/>`)
	writeFile(t, filepath.Join(dir, "v3.xml"), `<changeSet id="3" author="dev"/>`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestCountChangeSets_NestedDirectories(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "changelog-master.xml"), `<changeSet id="1" author="dev"/>`)
	writeFile(t, filepath.Join(dir, "releases", "v2.xml"), `
<changeSet id="2" author="dev"/>
<changeSet id="3" author="dev"/>`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 3 {
		t.Errorf("expected 3, got %d", count)
	}
}

func TestCountChangeSets_NonXMLFilesIgnored(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.xml"), `<changeSet id="1" author="dev"/>`)
	// These should NOT be counted.
	writeFile(t, filepath.Join(dir, "README.md"), `<changeSet id="fake" author="test"/>`)
	writeFile(t, filepath.Join(dir, "script.sql"), `<changeSet id="fake" author="test"/>`)
	writeFile(t, filepath.Join(dir, "config.yaml"), `changeSet: notxml`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 (non-XML files ignored), got %d", count)
	}
}

func TestCountChangeSets_DirectoryDoesNotExist(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	_, err := CountChangeSets(dir)
	if err == nil {
		t.Fatal("expected error for non-existent directory, got nil")
	}
}

// TestCountChangeSets_XMLWithNoChangeSets verifies that a valid XML changelog
// file containing no <changeSet elements counts as zero.
func TestCountChangeSets_XMLWithNoChangeSets(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "empty-changelog.xml"), `<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <!-- placeholder: changesets will be added in a future release -->
</databaseChangeLog>`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 0 {
		t.Errorf("expected 0 for changelog with no changeSets, got %d", count)
	}
}

// TestCountChangeSets_LargeCount verifies accuracy when many changesets exist
// across a file — important for services with a long migration history.
func TestCountChangeSets_LargeCount(t *testing.T) {
	const n = 50
	dir := t.TempDir()

	var sb strings.Builder
	sb.WriteString(`<databaseChangeLog>` + "\n")
	for i := 1; i <= n; i++ {
		fmt.Fprintf(&sb, `  <changeSet id="%d" author="dev"><sql>select 1</sql></changeSet>`+"\n", i)
	}
	sb.WriteString(`</databaseChangeLog>`)
	writeFile(t, filepath.Join(dir, "changelog.xml"), sb.String())

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != n {
		t.Errorf("expected %d, got %d", n, count)
	}
}

// TestCountChangeSets_MixedDepthWithNonXML tests the common real-world
// layout: a master XML at the root, versioned XMLs in a subdirectory, and
// non-XML files scattered throughout.
func TestCountChangeSets_MixedDepthWithNonXML(t *testing.T) {
	dir := t.TempDir()
	// Root changelog — 1 changeSet (include directive only, no changeSets itself).
	writeFile(t, filepath.Join(dir, "changelog-master.xml"),
		`<databaseChangeLog><include file="releases/v1.xml"/></databaseChangeLog>`)
	// Release file — 2 changeSets.
	writeFile(t, filepath.Join(dir, "releases", "v1.xml"), `
<databaseChangeLog>
  <changeSet id="1" author="dev"><createTable tableName="a"/></changeSet>
  <changeSet id="2" author="dev"><createTable tableName="b"/></changeSet>
</databaseChangeLog>`)
	// Non-XML noise files at every level.
	writeFile(t, filepath.Join(dir, "releases", "README.md"), "notes")
	writeFile(t, filepath.Join(dir, ".gitignore"), "*.class")

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// master (0) + v1.xml (2) = 2
	if count != 2 {
		t.Errorf("expected 2, got %d", count)
	}
}

// TestCountChangeSets_UnreadableFile verifies that an unreadable XML file
// causes CountChangeSets to return an error rather than silently skip it.
func TestCountChangeSets_UnreadableFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping permission test: running as root can read any file")
	}

	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.xml"), `<changeSet id="1" author="dev"/>`)

	unreadable := filepath.Join(dir, "v2.xml")
	writeFile(t, unreadable, `<changeSet id="2" author="dev"/>`)
	if err := os.Chmod(unreadable, 0o000); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(unreadable, 0o644)

	_, err := CountChangeSets(dir)
	if err == nil {
		t.Fatal("expected error for unreadable changelog file, got nil")
	}
}

// TestCountChangeSets_MalformedXML verifies that an XML file containing
// invalid markup returns an error (not a silent zero count). This covers the
// encoding/xml tokenizer error path introduced to replace the string-search
// approach.
func TestCountChangeSets_MalformedXML(t *testing.T) {
	dir := t.TempDir()
	// Unclosed tag — the XML decoder will fail on this.
	writeFile(t, filepath.Join(dir, "bad.xml"), `<databaseChangeLog><changeSet id="1" author="dev">`)

	_, err := CountChangeSets(dir)
	if err == nil {
		t.Fatal("expected error for malformed XML, got nil")
	}
	if !strings.Contains(err.Error(), "cannot parse changelog") {
		t.Errorf("error should mention parse failure, got: %v", err)
	}
}

// TestCountChangeSets_ChangeSetInComment verifies that a <changeSet string
// inside an XML comment is NOT counted — this was impossible to guarantee
// with the old string-search approach but the XML tokenizer handles it
// correctly because comments are returned as xml.Comment tokens, not
// xml.StartElement tokens.
func TestCountChangeSets_ChangeSetInComment(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "v1.xml"), `<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog>
  <!-- <changeSet id="disabled" author="dev"/> this should not be counted -->
  <changeSet id="1" author="dev"><createTable tableName="a"/></changeSet>
</databaseChangeLog>`)

	count, err := CountChangeSets(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if count != 1 {
		t.Errorf("expected 1 (comment <changeSet must not be counted), got %d", count)
	}
}

