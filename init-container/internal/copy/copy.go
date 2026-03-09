package copy

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
)

// CopyChangelogs copies all files from src directory into dest directory.
// This runs inside the Job pod init container using the service image.
// The service image has changelogs baked in at /db/changelog/ as loose files.
// Dest is an emptyDir volume shared with the liquibase runner main container.
//
// The second return value reports whether any copying was performed.
// (false, nil) means the source path did not exist — the caller should exit 0
// gracefully without querying the database.
func CopyChangelogs(src, dest string) (bool, error) {
	srcInfo, err := os.Stat(src)
	if err != nil {
		if os.IsNotExist(err) {
			slog.Info("changelog source path does not exist, skipping copy", "src", src)
			return false, nil
		}
		return false, fmt.Errorf("cannot access changelog source path %q: %w", src, err)
	}
	if !srcInfo.IsDir() {
		return false, fmt.Errorf("changelog source path %q is not a directory", src)
	}

	// Verify dest is writable by attempting to stat or create it.
	if err := os.MkdirAll(dest, 0o755); err != nil {
		return false, fmt.Errorf("changelog destination path %q is not writable (emptyDir not mounted?): %w", dest, err)
	}

	if err := filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return fmt.Errorf("error walking source path %q: %w", path, err)
		}

		// Compute relative path from src root.
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("cannot compute relative path for %q: %w", path, err)
		}

		destPath := filepath.Join(dest, rel)

		if info.IsDir() {
			if err := os.MkdirAll(destPath, info.Mode()); err != nil {
				return fmt.Errorf("cannot create directory %q: %w", destPath, err)
			}
			return nil
		}

		if err := copyFile(path, destPath, info.Mode()); err != nil {
			return err
		}

		slog.Info("copied changelog file", "src", path, "dest", destPath)
		return nil
	}); err != nil {
		return false, err
	}

	return true, nil
}

func copyFile(src, dest string, mode os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("cannot open source file %q: %w", src, err)
	}
	defer in.Close()

	out, err := os.OpenFile(dest, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return fmt.Errorf("cannot create destination file %q: %w", dest, err)
	}

	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return fmt.Errorf("cannot copy file %q to %q: %w", src, dest, err)
	}

	// Check Close error on the writer — a flush failure here means data loss.
	if err := out.Close(); err != nil {
		return fmt.Errorf("cannot close destination file %q: %w", dest, err)
	}

	return nil
}
