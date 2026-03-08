package main

import (
	"log/slog"
	"os"

	"github.com/your-org/platform-k8s-db-migrator/init-container/internal/copy"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	slog.Info("platform-k8s-db-migrator init container starting")

	// ── Phase 1: copy changelogs from service image into shared emptyDir ──────

	src := requireEnv("CHANGELOG_SOURCE_PATH")
	dest := requireEnv("CHANGELOG_DEST_PATH")
	serviceName := requireEnv("SERVICE_NAME")
	serviceImage := requireEnv("SERVICE_IMAGE")

	slog.Info("copying changelogs",
		"service", serviceName,
		"image", serviceImage,
		"src", src,
		"dest", dest,
	)

	if err := copy.CopyChangelogs(src, dest); err != nil {
		slog.Error("failed to copy changelogs", "error", err)
		os.Exit(1)
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
}

// requireEnv reads an environment variable and exits with a clear message if it is not set.
func requireEnv(key string) string {
	val := os.Getenv(key)
	if val == "" {
		slog.Error("required environment variable is not set", "var", key)
		os.Exit(1)
	}
	return val
}
