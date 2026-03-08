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
  Phase 2 (stubbed):     region detection, DB state check,
                         forward/rollback direction decision

Main container: liquibase/liquibase:latest  (NOT this repo)
  Reads changelogs from emptyDir
  Phase 1: runs liquibase update
  Phase 2: runs update or rollback based on init container decision
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
| `REGION_ROLE`          | Kustomize overlay     | `primary` or `replica`    |
| `REGION`               | Kustomize overlay     | e.g. `eu-west-1`          |

### Phase 2 — Region-aware migration direction (stubbed — TODOs in code)

Phase 2 will add:
- Region detection (EUW1 primary vs EUW2 replica)
- DB state check via `DATABASECHANGELOG` table
- Forward/rollback decision written to `/changelogs/migration-action.txt`

Additional variables for Phase 2: `DB_URL`, `DB_USERNAME`, `DB_PASSWORD`.

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

- [Onboarding guide](docs/onboarding.md)
- [Runbook: migration failed](docs/runbook-migration-failed.md)
- [Runbook: rollback](docs/runbook-rollback.md)
