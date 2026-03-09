# Gap Analysis — reference-service GitOps manifests

This document audits the `reference-service` GitOps layout against the
[GitOps Deployment Contract](gitops-deployment-contract.md) and lists every
change required to wire up the platform database migration framework.

The analysis is based on the following files already present in your GitOps
repo:

```
products/liquibase-job/apps/reference-service/
  kustomization.yaml
  deployment.yaml
  service.yaml
  gateway.yaml
  virtual-service.yaml
  db.yaml
```

---

## Summary of gaps

| # | Gap | Severity |
|---|-----|----------|
| 1 | No platform RBAC (kustomize-base not referenced) | ❌ Blocks migration Job from running |
| 2 | No ConfigMap with `SPRING_DATASOURCE_URL` / `SPRING_DATASOURCE_USERNAME` | ❌ Init container will exit immediately |
| 3 | Existing DB secrets use wrong key names for the init container | ❌ Init container will exit immediately |
| 4 | No `migration-job.yaml` (the PreSync Job does not exist) | ❌ Migrations will never run |
| 5 | `kustomization.yaml` does not list the new files | ❌ ArgoCD will not apply them |
| 6 | Service `Dockerfile` does not copy changelogs into the image | ❌ Init container copy phase will fail |
| 7 | Service has Liquibase auto-migration enabled | ⚠️ Migrations will run twice — once by the service, once by the Job |

---

## Gap 1 — No platform RBAC

**What's missing:** The `migration-trigger-sa` ServiceAccount, the
`migration-job-creator` Role, and the RoleBinding that binds them do not exist
in the `projects` namespace. The migration Job references
`serviceAccountName: migration-trigger-sa`, so the Job pod will fail to
schedule without it.

**Fix:** Add the platform kustomize-base as a resource in your
`kustomization.yaml`:

```yaml
# products/liquibase-job/apps/reference-service/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: projects

resources:
  - github.com/HashVelani/platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>  # e.g. v1.0.0 — see https://github.com/HashVelani/platform-k8s-db-migrator/releases
  - deployment.yaml
  - service.yaml
  - gateway.yaml
  - virtual-service.yaml
  - db.yaml
  - db-config.yaml        # ADD (see Gap 2)
  - liquibase-secret.yaml # ADD (see Gap 3)
  - migration-job.yaml    # ADD (see Gap 4)
```

Replace `<version-tag>` with the platform release you want to pin to, e.g.
`v1.0.0`. Check available tags at:
`https://github.com/HashVelani/platform-k8s-db-migrator/releases`

---

## Gap 2 — No ConfigMap with `SPRING_DATASOURCE_*` keys

**What's missing:** The init container requires two values from a ConfigMap:

- `SPRING_DATASOURCE_URL` — the JDBC URL for the PostgreSQL primary endpoint
- `SPRING_DATASOURCE_USERNAME` — the database user that Liquibase runs as

Your `db.yaml` provisions a CloudNative-PG cluster whose primary (read-write)
endpoint is:

```
reference-service-db-rw.projects.svc.cluster.local:5432
```

The database name is `reference_service` and the Liquibase user is
`reference_service_admin` (the cluster owner/admin role).

**Fix:** Create `db-config.yaml` in your GitOps app directory:

```yaml
# products/liquibase-job/apps/reference-service/db-config.yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: reference-service-db-config
  namespace: projects
data:
  SPRING_DATASOURCE_URL: "jdbc:postgresql://reference-service-db-rw.projects.svc.cluster.local:5432/reference_service?sslmode=require"
  SPRING_DATASOURCE_USERNAME: "reference_service_admin"
```

> **Why `sslmode=require`?** CloudNative-PG enables TLS by default. The init
> container image ships CA certificates (`/etc/ssl/certs/ca-certificates.crt`)
> so it can verify the server certificate. Do not use `sslmode=disable`.

---

## Gap 3 — Existing DB secrets have wrong key names

**What's missing:** Your `reference-service-db-admin` secret (type
`kubernetes.io/basic-auth`) exposes its password under the key `password`. The
init container reads `SPRING_DATASOURCE_PASSWORD` — a different key name. When
you mount the existing secret via `envFrom: secretRef`, the variable
`SPRING_DATASOURCE_PASSWORD` will not be set, and the init container will exit
with:

```
required environment variable "SPRING_DATASOURCE_PASSWORD" is not set
```

**Do not rename or retype** `reference-service-db-admin`. CloudNative-PG reads
that secret's `username` and `password` keys to set the DB owner password; if
you change the type or rename the keys the CNPG operator will reject it.

**Fix:** Create a separate `Opaque` secret that exposes the same admin password
under the name the init container expects:

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

Replace `CHANGE_ME_ADMIN` with the actual admin password. The value must stay
in sync with `reference-service-db-admin` → `password` — both refer to the
`reference_service_admin` role.

> **Tip:** In production, use an external secrets manager (e.g. External Secrets
> Operator or Vault Agent) to keep both secrets in sync from a single source of
> truth rather than duplicating the value in two YAML files.

---

## Gap 4 — No `migration-job.yaml`

**What's missing:** There is no Kubernetes Job that runs the migration. Without
it, ArgoCD will never trigger Liquibase before deployments.

**Fix:** Create `migration-job.yaml` in your GitOps app directory:

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
              value: hashvelani/reference-service-internal:v0.1.1  # update to match your deployment image
          envFrom:
            - configMapRef:
                name: reference-service-db-config      # provides SPRING_DATASOURCE_URL, SPRING_DATASOURCE_USERNAME
            - secretRef:
                name: reference-service-liquibase-secret  # provides SPRING_DATASOURCE_PASSWORD
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

- `serviceAccountName: migration-trigger-sa` — provided by Gap 1 fix (kustomize-base)
- `configMapRef: reference-service-db-config` — created in Gap 2 fix
- `secretRef: reference-service-liquibase-secret` — created in Gap 3 fix
- `backoffLimit: 0` — migration failures must block the deployment, not be retried silently
- `hook-delete-policy: BeforeHookCreation` — the old Job is deleted before a new sync creates a fresh one

---

## Gap 5 — `kustomization.yaml` not updated

**What's missing:** Even after creating the files above, ArgoCD will not apply
them unless they are listed in `kustomization.yaml`.

**Fix:** The complete updated `kustomization.yaml`:

```yaml
# products/liquibase-job/apps/reference-service/kustomization.yaml
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization

namespace: projects

resources:
  - github.com/HashVelani/platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>  # e.g. v1.0.0
  - deployment.yaml
  - service.yaml
  - gateway.yaml
  - virtual-service.yaml
  - db.yaml
  - db-config.yaml
  - liquibase-secret.yaml
  - migration-job.yaml
```

---

## Gap 6 — Service Dockerfile does not include changelogs (service repo change)

**What's missing:** This change is in your **service repository**, not the
GitOps repo. The init container's Phase 1 copies changelog files from
`/db/changelog/` inside the **service container image** into the shared
emptyDir volume. If that path is empty, the init container logs a warning and
skips the database check — migrations never run.

**Fix:** Add the following line to `reference-service`'s `Dockerfile`, **before**
the `ENTRYPOINT` instruction:

```dockerfile
# Dockerfile (reference-service repo)
COPY src/main/resources/db/changelog/ /db/changelog/
```

Adjust the source path to wherever your Liquibase XML files live in the build
context. The destination path `/db/changelog/` must match `CHANGELOG_SOURCE_PATH`
in the migration Job manifest.

Verify the path is correct:

```bash
docker run --rm hashvelani/reference-service-internal:v0.1.1 ls /db/changelog/
```

You should see your `changelog-master.xml` and any `changesets/` subdirectory
in the output.

---

## Gap 7 — Liquibase auto-migration must be disabled (service repo change)

**What's missing:** If `reference-service` runs Liquibase at startup
(`spring.liquibase.enabled` is not explicitly `false`), migrations will execute
twice — once via the platform Job (before the deployment) and again when the
service pod starts. The second run is a no-op in the forward direction but will
cause errors if the Job and the service disagree on the changelog state.

**Fix:** In `reference-service`'s `application.properties` (or `application.yaml`):

```properties
spring.liquibase.enabled=false
```

---

## Final file list after all fixes

```
products/liquibase-job/apps/reference-service/
  kustomization.yaml         ← UPDATED (add kustomize-base, 3 new files)
  deployment.yaml            (unchanged)
  service.yaml               (unchanged)
  gateway.yaml               (unchanged)
  virtual-service.yaml       (unchanged)
  db.yaml                    (unchanged)
  db-config.yaml             ← NEW
  liquibase-secret.yaml      ← NEW
  migration-job.yaml         ← NEW
```

Plus two changes in the **service repository** (not GitOps):

```
reference-service/
  Dockerfile                 ← ADD: COPY .../db/changelog/ /db/changelog/
  src/main/resources/
    application.properties   ← ADD: spring.liquibase.enabled=false
```

---

## Verification after deploying

```bash
# Check that the RBAC resources exist
kubectl get serviceaccount migration-trigger-sa -n projects
kubectl get role migration-job-creator -n projects

# Check that the ConfigMap and Secret are present
kubectl get configmap reference-service-db-config -n projects
kubectl get secret reference-service-liquibase-secret -n projects

# After ArgoCD syncs (PreSync hook triggers the Job)
kubectl get jobs -n projects
kubectl logs job/reference-service-db-migration -c init-changelogs
kubectl logs job/reference-service-db-migration -c liquibase
```

See [runbook-migration-failed.md](runbook-migration-failed.md) if the Job fails.
