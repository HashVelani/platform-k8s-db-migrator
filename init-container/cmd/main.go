package main

import (
	"fmt"
	"log/slog"
	"os"

	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/copy"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	if err := run(os.Getenv); err != nil {
		slog.Error("init container failed", "error", err)
		os.Exit(1)
	}
}

// run is the testable entry point. getenv is injected so tests can supply
// controlled environment variables without modifying the real process env.
func run(getenv func(string) string) error {
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

	if err := copy.CopyChangelogs(src, dest); err != nil {
		return fmt.Errorf("failed to copy changelogs: %w", err)
	}

	slog.Info("changelogs copied successfully",
		"service", serviceName,
		"src", src,
		"dest", dest,
	)

	// ── Phase 2 stub ──────────────────────────────────────────────────────────
	// TODO Phase 2: implement region-aware logic
	// 1. call region.Detect() → returns RegionRole (primary | replica)
	// 2. call db.GetLatestChangeset() → returns current DB state
	// 3. call decision.Decide(regionRole, dbState, changelogPath)
	//    → returns Action (update | rollback | none | block)
	// 4. if Action == block → log clear message, exit 1
	// 5. if Action == none  → log up to date, exit 0
	// 6. write Action to CHANGELOG_DEST_PATH/migration-action.txt
	//    so liquibase runner main container can read it
	// 7. exit 0

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
