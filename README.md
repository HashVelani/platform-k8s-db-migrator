# platform-k8s-db-migrator

Platform-owned Kubernetes database migration framework used by 10,000+ microservices across multi-region multi-tenant EKS clusters. This repository owns the **init container** that runs inside the migration Job pod. It does **not** own the Liquibase runner — that is `liquibase/liquibase:latest` (official image, no customisation needed).

---

## Architecture

```
MIGRATION JOB POD  (ArgoCD PreSync hook — runs before every deployment)
──────────────────────────────────────────────────────────────────────
Init container: platform-k8s-db-migrator/init-container  (THIS REPO)
  Phase 1 (implemented): copies /db/changelog/ from service image
                         into emptyDir shared with liquibase runner
  Phase 2 (implemented): DB state check, writes migration-action.txt
                         (update | none) to the emptyDir volume
  Phase 3 (stubbed):     rollback detection
  Phase 4 (stubbed):     region-based deployment blocking

Main container: liquibase/liquibase  (NOT this repo)
  Reads changelogs from emptyDir
  Phase 1: runs liquibase update
  Phase 2+: runs update or rollback based on migration-action.txt
──────────────────────────────────────────────────────────────────────
       emptyDir volume (/changelogs) bridges the two containers
──────────────────────────────────────────────────────────────────────
```

---

## Phases

### Phase 1 — File copy (fully implemented)

The init container copies all files from the service image's `/db/changelog/`
directory into an emptyDir volume at `/changelogs`. The Liquibase runner then
reads from `/changelogs` and runs `liquibase update`.

Environment variables required in Phase 1:

| Variable               | Source                | Value                     |
|------------------------|-----------------------|---------------------------|
| `CHANGELOG_SOURCE_PATH`| Job spec (hardcoded)  | `/db/changelog`           |
| `CHANGELOG_DEST_PATH`  | Job spec (hardcoded)  | `/changelogs`             |
| `SERVICE_NAME`         | Job spec              | Name of the service       |
| `SERVICE_IMAGE`        | Job spec              | Full image ref            |

### Phase 2 — DB schema inspection and migration decision (implemented)

Phase 2 connects to PostgreSQL, counts applied changesets in `DATABASECHANGELOG`,
and writes the migration action (`update` or `none`) to `/changelogs/migration-action.txt`.

Additional variables for Phase 2: `SPRING_DATASOURCE_URL`, `SPRING_DATASOURCE_USERNAME`, `SPRING_DATASOURCE_PASSWORD`.

These use the same env var names as Spring Boot + Hikari so the init container can share
the service's existing ConfigMap/Secret via `envFrom` with zero remapping.

---

## App Team Contract

Service repos must do exactly two things:

1. Add `COPY src/main/resources/db/changelog/ /db/changelog/` to the service `Dockerfile`.
2. Set `spring.liquibase.enabled=false` (or equivalent) in `application.properties`.

See [docs/onboarding.md](docs/onboarding.md) for full onboarding steps and the migration Job template.

---

## Build & Test Locally

```bash
# Run all tests
make test

# Build the binary
make build

# Build the Docker image
make docker-build
```

Requirements: Go 1.22+, Docker (for image build).

---

## Docs

- [GitOps deployment contract](docs/gitops-deployment-contract.md) — what your GitOps manifests must provide
- [Reference-service gap analysis](docs/reference-service-migration-gaps.md) — worked example auditing an existing service against the contract
- [Onboarding guide](docs/onboarding.md)
- [Runbook: migration failed](docs/runbook-migration-failed.md)
- [Runbook: rollback](docs/runbook-rollback.md)
