# Architecture: PreSync Hook and Sync-Wave Deployment Orchestration

This document explains how `kustomize-base/base/liquibase-migrator.yml` is designed to work, with a focus on how ArgoCD PreSync hooks and sync-waves are used to guarantee the correct deployment order.

---

## Background: ArgoCD sync phases and waves

ArgoCD divides every sync operation into three ordered phases:

| Phase | When it runs | Typical resources |
|---|---|---|
| **PreSync** | Before the main sync starts | Migration Jobs, readiness checks |
| **Sync** | The main apply phase | Deployments, Services, ConfigMaps, … |
| **PostSync** | After all Sync resources are Healthy | Smoke-test Jobs, notification hooks |

Within each phase, **sync-waves** (integer labels) control ordering. Resources at wave `N` must all be Healthy or Succeeded before ArgoCD advances to wave `N+1`. Wave numbers are evaluated globally across all resources in the Application, so a Deployment at Sync-wave `1` and a PreSync Job at PreSync-wave `0` participate in the same ordering sequence.

The two annotations that make a Job a PreSync hook are:

```yaml
argocd.argoproj.io/hook: PreSync
argocd.argoproj.io/sync-wave: "0"
```

---

## Wave ordering for a reference-service deployment

```
wave -2  DB Secrets (db-admin, db-user, db-ro-user)
         ↓  applied first; CNPG reads these to bootstrap credentials
wave -1  CNPG Cluster + reference-service-db-config ConfigMap
         ↓  ArgoCD polls until Cluster is Healthy (primary elected, pods ready)
         ↓  the database is now accepting connections
wave  0  migration Job  ← liquibase-migrator.yml  [PreSync hook]
         ↓  runs after DB is confirmed healthy
         ↓  applies pending Liquibase changesets
         ↓  ArgoCD waits until Job Succeeds
wave  1  reference-service Deployment  [Sync phase]
         first Pod starts only after the migration Job has Succeeded
         → schema is guaranteed to be ready before any application code runs
```

Because the migration Job has `argocd.argoproj.io/hook: PreSync`, it runs before the Sync phase starts for any wave. The `sync-wave: "0"` places it after wave `-1` resources (CNPG Cluster) have become Healthy, so the database is already accepting connections when Liquibase connects.

---

## The BeforeHookCreation delete policy

```yaml
argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
```

On every sync ArgoCD deletes the previous Job object before creating a new one. This:

- Prevents `AlreadyExists` errors when the image tag is unchanged between syncs.
- Keeps the namespace clean (no accumulation of old Job objects).
- Ensures the migration always runs from a fresh Job, even if the previous run Succeeded.

---

## Inside the migration Job: container ordering

Kubernetes runs init containers **sequentially** (each must exit 0 before the next starts). Main containers start only after all init containers have exited 0. The migration Job exploits this to build a three-stage pipeline:

```
┌─────────────────────────────────────────────────────────────────┐
│  Pod lifecycle (sequential left → right)                        │
│                                                                 │
│  Init 1           Init 2              Main                      │
│  copy-changelogs  init-changelogs     liquibase                 │
│  ───────────────  ─────────────────   ─────────────────────     │
│  cp /db/changelog → /changelogs       liquibase update          │
│  (service image)   (platform image)  (official Liquibase image) │
│                                                                 │
│       emptyDir volume /changelogs bridges all three containers  │
└─────────────────────────────────────────────────────────────────┘
```

### Init 1 — `copy-changelogs`

```yaml
- name: copy-changelogs
  image: hashvelani/reference-service-internal:v0.1.6   # the SERVICE image
  command: ["cp", "-r", "/db/changelog/.", "/changelogs/"]
  volumeMounts:
    - name: changelogs
      mountPath: /changelogs
```

- Runs the **service image** (the same image the Deployment uses) with its entrypoint replaced by a plain `cp`.
- The JVM never starts; no Spring context; no application code executes.
- Copies `/db/changelog/` (baked into the service image at build time via `COPY src/main/resources/db/changelog/ /db/changelog/`) into the shared `emptyDir` volume at `/changelogs`.
- Exits 0 immediately after the copy.

### Init 2 — `init-changelogs`

```yaml
- name: init-changelogs
  image: hashvelani/platform-k8s-db-migrator:0.1.4     # THIS REPO
  env:
    - name: CHANGELOG_SOURCE_PATH
      value: /changelogs   # same as DEST — copy was already done above
    - name: CHANGELOG_DEST_PATH
      value: /changelogs
```

- Runs after `copy-changelogs` has exited 0 and the files are in the `emptyDir`.
- `CHANGELOG_SOURCE_PATH` and `CHANGELOG_DEST_PATH` are **both set to `/changelogs`** because the file copy was already handled by Init 1. The platform container detects `src == dest` and activates its self-copy guard, skipping the copy step entirely.
- Counts `<changeSet>` elements in the XML files under `/changelogs` — the "desired" schema state.
- Queries the `DATABASECHANGELOG` table to count how many changesets have already been applied — the "current" schema state.
- Compares the two counts and writes one of the following to `/changelogs/migration-action.txt`:
  - `update` — new changesets exist that have not been applied yet.
  - `none` — the database is already up to date; no migration needed.

DB credentials come from the same ConfigMap and Secret the service already uses (via `envFrom`), so no credential duplication is needed.

### Main — `liquibase`

```yaml
- name: liquibase
  image: liquibase/liquibase:4.29.2
  args:
    - --searchPath=/changelogs
    - --changeLogFile=db.changelog-master.xml
    - update
```

- Starts only after both init containers have exited 0.
- Reads `/changelogs/migration-action.txt`:
  - `update` → calls `liquibase update` to apply pending changesets.
  - `none` → logs "already up to date" and exits 0 without touching the database.
- Connects using admin (DDL-privileged) credentials from the Secret.
- The image is pinned to an explicit immutable tag (`4.29.2`). **Never use `latest`** — unpinned images are a supply-chain risk and break reproducible deployments.

---

## What happens on every sync

### New deployment (image tag bumped, new changesets added)

1. ArgoCD detects a diff.
2. `BeforeHookCreation` deletes the old migration Job.
3. Wave -2 / -1: Secrets and CNPG Cluster are reconciled (fast no-op if already healthy).
4. Wave 0: Migration Job is created.
   - `copy-changelogs` copies the new changelogs into emptyDir.
   - `init-changelogs` detects `changelogCount > dbCount`, writes `update`.
   - `liquibase` runs `liquibase update`, applies new changesets, exits 0.
5. Job Succeeds → ArgoCD advances to the Sync phase.
6. Wave 1: Deployment rolls out with the new image — schema is already in place.

### Re-sync with no schema changes (image tag unchanged or only non-DB changes)

1. ArgoCD detects a diff (or is triggered manually).
2. Wave 0: Migration Job runs again.
   - `init-changelogs` detects `changelogCount == dbCount`, writes `none`.
   - `liquibase` reads `none`, skips Liquibase, exits 0.
3. Job Succeeds → sync continues. No database traffic from Liquibase.

---

## Relationship to the Kustomize base

`kustomize-base/base/kustomization.yaml` provides shared RBAC resources that every service references:

- `serviceaccount.yaml` — `migration-trigger-sa`
- `role.yaml` — `migration-job-creator` Role
- `rolebinding.yaml` — binds the Role to `migration-trigger-sa`

`liquibase-migrator.yml` is the **reference implementation** for the migration Job. Because the Job spec is inherently service-specific (image tags, service name, namespace, ConfigMap and Secret names), each service copies and customises this file rather than inheriting it via a Kustomize overlay.

See [onboarding.md](onboarding.md) for the step-by-step guide on wiring this into your service.
