package changelog

import (
	"encoding/xml"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// CountChangeSets returns the total number of <changeSet elements found
// across all XML files in dir, searched recursively.
//
// Uses an XML tokenizer (not a plain text search) so that changeSet references
// inside XML comments or CDATA sections are not counted, and namespace-prefixed
// tags (e.g. <lb:changeSet>) are handled correctly via Name.Local.
//
// This is compared against the count of applied changesets in
// DATABASECHANGELOG to determine whether a migration is required:
//
//   - changelogCount > dbCount  → update   (new changesets to apply)
//   - changelogCount == dbCount → none     (database is up to date)
//   - changelogCount < dbCount  → error    (changeset removed — manual fix needed)
func CountChangeSets(dir string) (int, error) {
	total := 0
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("cannot walk %q: %w", path, err)
		}
		if info.IsDir() || !strings.HasSuffix(strings.ToLower(info.Name()), ".xml") {
			return nil
		}
		n, err := countChangeSetsInFile(path)
		if err != nil {
			return err
		}
		total += n
		return nil
	})
	return total, err
}

// countChangeSetsInFile opens a single XML file, tokenizes it, and returns
// the number of <changeSet start elements it contains. Extracting this into
// its own function ensures the file handle is closed as soon as the function
// returns — the defer does not accumulate across multiple Walk iterations.
func countChangeSetsInFile(path string) (int, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("cannot read changelog %q: %w", path, err)
	}
	defer f.Close()

	var count int
	decoder := xml.NewDecoder(f)
	for {
		tok, err := decoder.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return 0, fmt.Errorf("cannot parse changelog %q: %w", path, err)
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "changeSet" {
			count++
		}
	}
	return count, nil
}
