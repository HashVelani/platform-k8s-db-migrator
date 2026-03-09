# Gap Analysis — reference-service GitOps manifests

This document audits the `reference-service` GitOps layout against the
[GitOps Deployment Contract](gitops-deployment-contract.md) and lists every
change required to wire up the platform database migration framework.

---

## Current state of the manifests

The following files are present in your GitOps repo after the most recent
update:

```
products/liquibase-job/apps/reference-service/
  kustomization.yaml       (lists configmap.yaml + patchesStrategicMerge)
  configmap.yaml           (reference-service-db-config — DB_URL key only)
  deployment-patch.yaml    (adds DB_URL / DB_USERNAME / DB_PASSWORD to service container)
  deployment.yaml
  service.yaml
  gateway.yaml
  virtual-service.yaml
  db.yaml
```

### What the most recent update fixed

The addition of `configmap.yaml` and `deployment-patch.yaml` wires the app
container to the database correctly. The ConfigMap and the app are now
connected. However, neither of those changes is what the **init container**
needs — the migrator has different requirements for variable names and a
separate admin-password secret, none of which exist yet.

---

## Summary of remaining gaps

| # | Gap | Status | Severity |
|---|-----|--------|----------|
| 1 | No platform RBAC (kustomize-base not referenced) | ❌ Not started | Blocks migration Job from scheduling |
| 2 | ConfigMap has wrong keys for the init container (`DB_URL` ≠ `SPRING_DATASOURCE_URL`; `SPRING_DATASOURCE_USERNAME` missing) | ⚠️ Partial — ConfigMap exists but keys don't match | Init container exits immediately |
| 3 | No secret with `SPRING_DATASOURCE_PASSWORD` key | ❌ Not started | Init container exits immediately |
| 4 | No `migration-job.yaml` (the ArgoCD PreSync hook Job) | ❌ Not started | Migrations never run |
| 5 | `kustomization.yaml` missing kustomize-base, liquibase-secret, migration-job entries | ⚠️ Partial — configmap.yaml is listed, the rest are not | ArgoCD will not apply the new resources |
| 6 | Service `Dockerfile` does not copy changelogs into the image | ❌ Not started | Init container copy phase produces no files; migrations silently skipped |
| 7 | Service has Liquibase auto-migration enabled | ❌ Not started | Migrations run twice if not disabled |

---

## Gap 1 — No platform RBAC

**What's missing:** The `migration-trigger-sa` ServiceAccount, the
`migration-job-creator` Role, and the RoleBinding that binds them do not exist
in the `projects` namespace. The migration Job pod references
`serviceAccountName: migration-trigger-sa`, so it will fail to schedule without
this.

**Fix:** Add the platform kustomize-base as the first entry in `resources` in
your `kustomization.yaml` (see also Gap 5 for the complete updated file):

```yaml
resources:
  - github.com/HashVelani/platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>  # check https://github.com/HashVelani/platform-k8s-db-migrator/releases for available tags
```

---

## Gap 2 — ConfigMap has wrong key names for the init container

**What's partially done:** `configmap.yaml` exists and creates
`reference-service-db-config`. It contains:

```yaml
data:
  DB_URL: jdbc:postgresql://reference-service-db-rw.projects.svc.cluster.local:5432/reference_service
```

**What's still missing:** The init container reads two specific variable names
that are not present in this ConfigMap:

- `SPRING_DATASOURCE_URL` — the JDBC URL (note: must include `?sslmode=require`)
- `SPRING_DATASOURCE_USERNAME` — the admin/Liquibase user (`reference_service_admin`)

The `DB_URL` key is correct for your service container (as wired up in
`deployment-patch.yaml`), but the init container will not find it under that
name and will exit immediately with:

```
required environment variable "SPRING_DATASOURCE_URL" is not set
```

**Fix:** Add the two missing keys to the existing `configmap.yaml`. Keep
`DB_URL` as-is so the service container continues to work:

```yaml
# products/liquibase-job/apps/reference-service/configmap.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: reference-service-db-config
  namespace: projects
data:
  # Used by the service container (via deployment-patch.yaml)
  DB_URL: jdbc:postgresql://reference-service-db-rw.projects.svc.cluster.local:5432/reference_service

  # Used by the migration init container and Liquibase Job container
  SPRING_DATASOURCE_URL: "jdbc:postgresql://reference-service-db-rw.projects.svc.cluster.local:5432/reference_service?sslmode=require"
  SPRING_DATASOURCE_USERNAME: "reference_service_admin"
```

> **Why `sslmode=require` on the migrator URL but not `DB_URL`?** CloudNative-PG
> enables TLS by default. The init container image ships CA certificates so it
> can verify the server certificate — `sslmode=require` is the correct setting.
> Your service's JDBC driver (HikariCP/Spring) may handle TLS separately via its
> own configuration; adjust `DB_URL` if your service also needs TLS.

> **Why `reference_service_admin` as username?** The init container connects to
> PostgreSQL to count applied changesets before copying files. It must connect as
> the role that owns the schema (the Liquibase admin user), not the read-write
> application user. `reference_service_admin` is the CNPG cluster owner and is
> used by Liquibase to apply migrations.

---

## Gap 3 — No secret with `SPRING_DATASOURCE_PASSWORD`

**What's missing:** The init container requires a variable named
`SPRING_DATASOURCE_PASSWORD` exposed via `envFrom: secretRef`. None of your
existing secrets provide a key with that name.

Your `reference-service-db-admin` secret (type `kubernetes.io/basic-auth`)
holds the admin password under the key `password`. When mounted via
`envFrom: secretRef`, that yields an env var named `password` — not
`SPRING_DATASOURCE_PASSWORD`. The init container exits immediately.

**Do not rename or retype `reference-service-db-admin`.** CloudNative-PG reads
that secret's `username` and `password` keys at bootstrap; changing the secret
type or key names causes the CNPG operator to reject the cluster resource.

**Fix:** Create a separate `Opaque` secret, `liquibase-secret.yaml`, that
exposes the same admin password under the key the init container expects:

```yaml
# products/liquibase-job/apps/reference-service/liquibase-secret.yaml
apiVersion: v1
kind: Secret
metadata:
  name: reference-service-liquibase-secret
  namespace: projects
type: Opaque
stringData:
  SPRING_DATASOURCE_PASSWORD: "CHANGE_ME_ADMIN"  # same value as reference-service-db-admin → password
```

The value must stay in sync with `reference-service-db-admin` → `password`.
Both refer to the `reference_service_admin` PostgreSQL role.

> **Tip:** In production, use an external secrets manager (e.g. External Secrets
> Operator or Vault Agent Injector) to project the same upstream secret into
> both Kubernetes secrets from a single source of truth, eliminating the
> duplication.

---

## Gap 4 — No `migration-job.yaml`

**What's missing:** There is no Kubernetes Job resource that runs the
migration. Without it, ArgoCD has nothing to execute as a PreSync hook and
Liquibase never runs.

**Fix:** Create `migration-job.yaml`:

```yaml
# products/liquibase-job/apps/reference-service/migration-job.yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: reference-service-db-migration
  namespace: projects
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
spec:
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: reference-service-db-migration
    spec:
      serviceAccountName: migration-trigger-sa
      restartPolicy: Never
      initContainers:
        - name: init-changelogs
          image: hashvelani/platform-k8s-db-migrator:<version-tag>  # e.g. v1.0.0 — see https://github.com/HashVelani/platform-k8s-db-migrator/releases
          env:
            - name: CHANGELOG_SOURCE_PATH
              value: /db/changelog
            - name: CHANGELOG_DEST_PATH
              value: /changelogs
            - name: SERVICE_NAME
              value: reference-service
            - name: SERVICE_IMAGE
              value: hashvelani/reference-service-internal:v0.1.1  # must match the image tag in deployment.yaml — update both together
          envFrom:
            - configMapRef:
                name: reference-service-db-config       # provides SPRING_DATASOURCE_URL + SPRING_DATASOURCE_USERNAME (see Gap 2 fix)
            - secretRef:
                name: reference-service-liquibase-secret # provides SPRING_DATASOURCE_PASSWORD (see Gap 3 fix)
          volumeMounts:
            - name: changelogs
              mountPath: /changelogs
      containers:
        - name: liquibase
          # Pin to an immutable version tag — do not use 'latest'.
          # Find stable releases at https://hub.docker.com/r/liquibase/liquibase/tags
          image: liquibase/liquibase:<version-tag>  # e.g. 4.29.2 — check https://hub.docker.com/r/liquibase/liquibase/tags for current stable
          args:
            - --changeLogFile=/changelogs/changelog-master.xml
            - --url=$(SPRING_DATASOURCE_URL)
            - --username=$(SPRING_DATASOURCE_USERNAME)
            - --password=$(SPRING_DATASOURCE_PASSWORD)
            - update
          envFrom:
            - configMapRef:
                name: reference-service-db-config
            - secretRef:
                name: reference-service-liquibase-secret
          volumeMounts:
            - name: changelogs
              mountPath: /changelogs
      volumes:
        - name: changelogs
          emptyDir: {}
```

**Key decisions in this manifest:**

- `serviceAccountName: migration-trigger-sa` — provisioned by the kustomize-base (Gap 1)
- `backoffLimit: 0` — migration failures block the deployment rather than retrying silently
- `hook-delete-policy: BeforeHookCreation` — stale Job from the previous sync is deleted before ArgoCD creates a new one
- Both containers mount the same `emptyDir` volume: the init container writes changelog files into it, and the Liquibase container reads from it
- `SERVICE_IMAGE` must always match the image tag in `deployment.yaml`. There is no automated enforcement — update both files together whenever you bump the service image.

---

## Gap 5 — `kustomization.yaml` incomplete

**What's partially done:** `kustomization.yaml` already lists `configmap.yaml`
and uses `patchesStrategicMerge` for `deployment-patch.yaml`. Those entries are
correct and should stay.

**What's still missing:** The three new resources from Gaps 1–4 are not listed:

```yaml
resources:
  - github.com/HashVelani/platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>  # ← missing
  - liquibase-secret.yaml  # ← missing
  - migration-job.yaml     # ← missing
```

**Fix:** The complete updated `kustomization.yaml`:

```yaml
# products/liquibase-job/apps/reference-service/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: projects

resources:
  - github.com/HashVelani/platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>  # ADD — check https://github.com/HashVelani/platform-k8s-db-migrator/releases
  - configmap.yaml                                                                          # already present; keys updated in Gap 2
  - deployment.yaml
  - service.yaml
  - gateway.yaml
  - virtual-service.yaml
  - db.yaml
  - liquibase-secret.yaml  # ADD (see Gap 3)
  - migration-job.yaml     # ADD (see Gap 4)

patchesStrategicMerge:
  - deployment-patch.yaml  # already present; unchanged
```

---

## Gap 6 — Service Dockerfile does not include changelogs (service repo change)

**What's missing:** This change belongs in your **service repository**, not the
GitOps repo. The init container's Phase 1 copies changelog XML files from
`/db/changelog/` inside the **service container image** into the shared emptyDir
volume. If that path is absent or empty, the init container logs a warning and
skips the database check — Liquibase never runs.

**Fix:** Add the following line to `reference-service`'s `Dockerfile`, **before**
the `ENTRYPOINT` instruction:

```dockerfile
# Dockerfile (reference-service service repo)
COPY src/main/resources/db/changelog/ /db/changelog/
```

Adjust the source path to wherever your Liquibase XML files live in the build
context. The destination path `/db/changelog/` must match the
`CHANGELOG_SOURCE_PATH` value in `migration-job.yaml`.

Verify after building:

```bash
docker run --rm hashvelani/reference-service-internal:v0.1.1 ls /db/changelog/
```

You should see `changelog-master.xml` and any `changesets/` subdirectory.

---

## Gap 7 — Liquibase auto-migration must be disabled (service repo change)

**What's missing:** If `reference-service` runs Liquibase at startup (i.e.
`spring.liquibase.enabled` is not explicitly `false`), migrations will execute
twice — once via the platform PreSync Job and again when the service pod starts.
The second run is a no-op for forward migrations but will cause errors if the
Job and the service disagree on changelog state.

**Fix:** In `reference-service`'s `application.properties` (or
`application.yaml`):

```properties
spring.liquibase.enabled=false
```

---

## Complete list of changes required

### In the GitOps repo

| File | Change |
|------|--------|
| `kustomization.yaml` | Add kustomize-base URL, `liquibase-secret.yaml`, `migration-job.yaml` to `resources` |
| `configmap.yaml` | Add `SPRING_DATASOURCE_URL` and `SPRING_DATASOURCE_USERNAME` keys (keep `DB_URL` as-is) |
| `liquibase-secret.yaml` | **Create new** — Opaque secret with `SPRING_DATASOURCE_PASSWORD` |
| `migration-job.yaml` | **Create new** — ArgoCD PreSync Job |

### In the service repository

| File | Change |
|------|--------|
| `Dockerfile` | Add `COPY .../db/changelog/ /db/changelog/` before `ENTRYPOINT` |
| `application.properties` | Add `spring.liquibase.enabled=false` |

---

## Verification after deploying

```bash
# Check that the RBAC resources exist
kubectl get serviceaccount migration-trigger-sa -n projects
kubectl get role migration-job-creator -n projects

# Check that the updated ConfigMap and new Secret are present
kubectl get configmap reference-service-db-config -n projects -o yaml | grep SPRING_DATASOURCE
kubectl get secret reference-service-liquibase-secret -n projects

# After ArgoCD syncs (PreSync hook triggers the Job)
kubectl get jobs -n projects
kubectl logs job/reference-service-db-migration -c init-changelogs
kubectl logs job/reference-service-db-migration -c liquibase
```

See [runbook-migration-failed.md](runbook-migration-failed.md) if the Job fails.
