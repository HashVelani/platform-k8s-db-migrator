# Onboarding Guide — platform-k8s-db-migrator

This guide describes what application teams must do to use the platform database migration framework.

---

## Prerequisites

- Your service uses Liquibase changelogs stored at `src/main/resources/db/changelog/` (or equivalent).
- Your GitOps repo uses Kustomize.
- ArgoCD is configured to sync your application.

---

## Step 1: Add changelogs to your service Docker image

Add the following line to your service `Dockerfile` **before** the final `ENTRYPOINT` instruction:

```dockerfile
COPY src/main/resources/db/changelog/ /db/changelog/
```

The init container expects changelogs at `/db/changelog/` inside the service image.

---

## Step 2: Disable Liquibase auto-migration in your service

The platform migration framework owns all migrations. Your service must **not** run Liquibase at startup.

**Spring Boot** (`application.properties` or `application.yaml`):
```properties
spring.liquibase.enabled=false
```

**Quarkus**:
```properties
quarkus.liquibase.migrate-at-start=false
```

**Micronaut**:
```properties
micronaut.liquibase.enabled=false
```

> The `check-liquibase-disabled` pre-commit hook (in `ci-tools/pre-commit-hooks/`) enforces this automatically.

---

## Step 3: Reference the platform Kustomize base in your GitOps repo

In your GitOps repo's `kustomization.yaml`, add:

```yaml
resources:
  - github.com/your-org/platform-k8s-db-migrator/kustomize-base/base?ref=main
```

This provides the `migration-trigger-sa` ServiceAccount and the RBAC Role/RoleBinding your migration Job needs.

---

## Step 4: Add `migration-job.yaml` to your GitOps repo

Create the following file in your GitOps repo. Replace placeholders (`<...>`) with your values.

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
  backoffLimit: 0
  template:
    metadata:
      labels:
        app: <service-name>-db-migration
    spec:
      serviceAccountName: migration-trigger-sa
      restartPolicy: Never
      initContainers:
        - name: init-changelogs
          image: hashvelani/platform-k8s-db-migrator:<version>
          env:
            - name: CHANGELOG_SOURCE_PATH
              value: /db/changelog
            - name: CHANGELOG_DEST_PATH
              value: /changelogs
            - name: SERVICE_NAME
              value: <service-name>
            - name: SERVICE_IMAGE
              value: <your-registry>/<service-name>:<image-tag>
          # DB credentials come from the same ConfigMap and Secret your service
          # already uses. envFrom mounts all keys as env vars automatically, so
          # SPRING_DATASOURCE_URL, SPRING_DATASOURCE_USERNAME and
          # SPRING_DATASOURCE_PASSWORD are available without any extra mapping.
          envFrom:
            - configMapRef:
                name: <service-name>-config   # provides SPRING_DATASOURCE_URL, SPRING_DATASOURCE_USERNAME
            - secretRef:
                name: <service-name>-db-secret # provides SPRING_DATASOURCE_PASSWORD
          volumeMounts:
            - name: changelogs
              mountPath: /changelogs
      containers:
        - name: liquibase
          image: liquibase/liquibase:latest
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
          emptyDir: {}
```

---

## Verification

After deploying, check the ArgoCD UI for the PreSync hook status, or run:

```bash
kubectl get jobs -n <your-namespace>
kubectl logs job/<service-name>-db-migration -c init-changelogs
kubectl logs job/<service-name>-db-migration -c liquibase
```

See [runbook-migration-failed.md](runbook-migration-failed.md) if the Job fails.
