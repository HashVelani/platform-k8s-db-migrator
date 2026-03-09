# GitOps Deployment Contract — platform-k8s-db-migrator

This document is the authoritative reference for what your GitOps manifests must supply in order to use the platform database migration framework. Keep your manifests in your own GitOps repo; this document tells you exactly what to put in them.

---

## 1. Init container image

| Field   | Value                                      |
|---------|--------------------------------------------|
| Image   | `hashvelani/platform-k8s-db-migrator`      |
| Tag     | A pinned semver tag, e.g. `v1.2.3`        |
| Registry| Docker Hub (public)                        |

**Do not use `latest`.** Pin to an immutable semver tag. Available tags are published at:
`https://hub.docker.com/r/hashvelani/platform-k8s-db-migrator/tags`

---

## 2. Required environment variables

All seven variables below are **mandatory**. The init container exits with an error if any is missing or empty.

### Hardcoded in your Job manifest

| Variable               | Value                  | Description                                      |
|------------------------|------------------------|--------------------------------------------------|
| `CHANGELOG_SOURCE_PATH`| `/db/changelog`        | Path inside the service image where Liquibase changelogs live |
| `CHANGELOG_DEST_PATH`  | `/changelogs`          | Mount path of the shared emptyDir volume         |
| `SERVICE_NAME`         | `<your-service-name>`  | Human-readable identifier used in structured logs |
| `SERVICE_IMAGE`        | `<registry/name:tag>`  | Full image reference of the service being deployed |

### Provided by your existing ConfigMap and Secret (via `envFrom`)

| Variable                    | Source       | Description                                                      |
|-----------------------------|--------------|------------------------------------------------------------------|
| `SPRING_DATASOURCE_URL`     | ConfigMap    | PostgreSQL JDBC URL, e.g. `jdbc:postgresql://host:5432/mydb?sslmode=require` |
| `SPRING_DATASOURCE_USERNAME`| ConfigMap    | Database username                                                |
| `SPRING_DATASOURCE_PASSWORD`| Secret       | Database password                                                |

The init container reuses the **same** ConfigMap and Secret your service already uses with Spring Boot / Hikari. No key remapping is required — mount them with `envFrom` and the variables are available automatically.

**ConfigMap minimum required keys:**
```
SPRING_DATASOURCE_URL
SPRING_DATASOURCE_USERNAME
```

**Secret minimum required key:**
```
SPRING_DATASOURCE_PASSWORD
```

---

## 3. Required Kubernetes RBAC resources

Reference the platform Kustomize base from your GitOps `kustomization.yaml`. This creates the `migration-trigger-sa` ServiceAccount plus the `migration-job-creator` Role and RoleBinding in your namespace.

```yaml
# your-gitops-repo/kustomization.yaml
resources:
  - github.com/HashVelani/platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>
```

The Role grants permission to:
- `batch/jobs` — `create`, `get`, `watch`, `list`
- `pods`, `pods/log` — `get`, `list`, `watch`

---

## 4. Service image contract

Before the migration Job can run, your service Docker image **must** contain the changelogs at the path the init container expects.

Add this line to your service `Dockerfile` **before** the final `ENTRYPOINT`:

```dockerfile
COPY src/main/resources/db/changelog/ /db/changelog/
```

The source path (`src/main/resources/db/changelog/`) is an example for a Maven/Gradle Spring Boot layout. Adjust to wherever your Liquibase XML files live. The destination path (`/db/changelog/`) must match `CHANGELOG_SOURCE_PATH` set in your Job manifest.

---

## 5. Disable service-side Liquibase auto-migration

The platform migration framework owns all Liquibase runs. Your service must **not** run Liquibase at startup.

| Framework   | Setting                                  |
|-------------|------------------------------------------|
| Spring Boot | `spring.liquibase.enabled=false`         |
| Quarkus     | `quarkus.liquibase.migrate-at-start=false` |
| Micronaut   | `micronaut.liquibase.enabled=false`      |

---

## 6. Migration Job manifest

Create `migration-job.yaml` in your GitOps repo. Replace every `<placeholder>` with your values.

```yaml
apiVersion: batch/v1
kind: Job
metadata:
  name: <service-name>-db-migration
  namespace: <your-namespace>
  annotations:
    argocd.argoproj.io/hook: PreSync
    argocd.argoproj.io/hook-delete-policy: BeforeHookCreation
spec:
  backoffLimit: 0       # fail the Job immediately on first error — do not retry
  template:
    metadata:
      labels:
        app: <service-name>-db-migration
    spec:
      serviceAccountName: migration-trigger-sa   # created by the kustomize-base above
      restartPolicy: Never
      initContainers:
        - name: init-changelogs
          image: hashvelani/platform-k8s-db-migrator:<version-tag>
          env:
            - name: CHANGELOG_SOURCE_PATH
              value: /db/changelog        # must match the COPY destination in your service Dockerfile
            - name: CHANGELOG_DEST_PATH
              value: /changelogs          # must match the emptyDir mountPath below
            - name: SERVICE_NAME
              value: <service-name>
            - name: SERVICE_IMAGE
              value: <your-registry>/<service-name>:<image-tag>
          envFrom:
            - configMapRef:
                name: <service-name>-config    # provides SPRING_DATASOURCE_URL and SPRING_DATASOURCE_USERNAME
            - secretRef:
                name: <service-name>-db-secret # provides SPRING_DATASOURCE_PASSWORD
          volumeMounts:
            - name: changelogs
              mountPath: /changelogs
      containers:
        - name: liquibase
          # Pin to an immutable version tag — do not use 'latest'.
          # Find stable releases at https://hub.docker.com/r/liquibase/liquibase/tags
          image: liquibase/liquibase:<version>
          args:
            - --changeLogFile=/changelogs/changelog-master.xml
            - --url=$(SPRING_DATASOURCE_URL)
            - --username=$(SPRING_DATASOURCE_USERNAME)
            - --password=$(SPRING_DATASOURCE_PASSWORD)
            - update
          envFrom:
            - configMapRef:
                name: <service-name>-config
            - secretRef:
                name: <service-name>-db-secret
          volumeMounts:
            - name: changelogs
              mountPath: /changelogs
      volumes:
        - name: changelogs
          emptyDir: {}    # shared between init container (write) and liquibase container (read)
```

---

## 7. Volume contract

| Volume name  | Type      | Init container mount | Liquibase container mount | Purpose                             |
|--------------|-----------|----------------------|---------------------------|-------------------------------------|
| `changelogs` | `emptyDir`| `/changelogs` (write)| `/changelogs` (read)      | Pass changelog files between containers |

The init container **writes** into `/changelogs`. The Liquibase runner **reads** from `/changelogs`. Both must mount the same `emptyDir` volume.

### Files written by the init container

| File                              | Content                        | When written                              |
|-----------------------------------|--------------------------------|-------------------------------------------|
| `/changelogs/*.xml` (and subdirs) | Copied from service image      | Always (Phase 1)                          |
| `/changelogs/migration-action.txt`| `update` or `none`            | Only when changelog files are found (Phase 2) |

---

## 8. Database URL format

The init container accepts the standard Spring Boot JDBC URL format:

```
jdbc:postgresql://<host>:<port>/<database>[?param=value&...]
```

Example with TLS:
```
jdbc:postgresql://postgres.example.com:5432/myapp?sslmode=require
```

The `sslmode` parameter (and any other query parameters) is passed through to the Go PostgreSQL driver. PostgreSQL on TLS is fully supported — the init container image ships CA certificates for server certificate verification.

---

## 9. Checklist for your GitOps PR

Use this checklist when adding migration support for a new service:

- [ ] Service `Dockerfile` includes `COPY ... /db/changelog/`
- [ ] Service config disables Liquibase auto-migration (`spring.liquibase.enabled=false` or equivalent)
- [ ] `kustomization.yaml` references `platform-k8s-db-migrator/kustomize-base/base?ref=<version-tag>`
- [ ] `migration-job.yaml` exists with all `<placeholder>` values filled in
- [ ] `CHANGELOG_SOURCE_PATH` in Job matches the `COPY` destination in the service `Dockerfile`
- [ ] `CHANGELOG_DEST_PATH` in Job matches the `emptyDir` mount path (`/changelogs`)
- [ ] `envFrom` references the correct ConfigMap and Secret names
- [ ] Both `SPRING_DATASOURCE_URL` and `SPRING_DATASOURCE_USERNAME` are keys in the referenced ConfigMap
- [ ] `SPRING_DATASOURCE_PASSWORD` is a key in the referenced Secret
- [ ] Init container image tag and Liquibase image tag are both pinned semver values (not `latest`)

---

## Troubleshooting

If the migration Job fails, see [runbook-migration-failed.md](runbook-migration-failed.md) for a guided diagnosis.

To check Job status and logs after a deployment:

```bash
kubectl get jobs -n <your-namespace>
kubectl logs job/<service-name>-db-migration -c init-changelogs
kubectl logs job/<service-name>-db-migration -c liquibase
```

Logs from the init container are JSON-structured. Each log line includes `time`, `level`, and `msg` fields plus context keys such as `service`, `src`, `dest`, `changelogCount`, and `dbCount`.
