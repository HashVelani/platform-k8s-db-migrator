package changelog

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// CountChangeSets returns the total number of <changeSet elements found
// across all XML files in dir, searched recursively.
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
		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("cannot read changelog %q: %w", path, err)
		}
		total += strings.Count(string(data), "<changeSet")
		return nil
	})
	return total, err
}
