# Runbook: Migration Job Failed

This runbook covers how to diagnose and recover from a failed database migration Job.

---

## 1. Identify the failed Job

```bash
kubectl get jobs -n <namespace>
kubectl describe job <job-name> -n <namespace>
```

Look for `Status: Failed` and the failure reason in the `Events` section.

---

## 2. Check init container logs

The init container (`init-changelogs`) runs first. If it fails, Liquibase never starts.

```bash
kubectl logs job/<job-name> -c init-changelogs -n <namespace>
```

### Common init container failures

| Error message | Root cause | Fix |
|---|---|---|
| `changelog source path "/db/changelog" does not exist` | Service image missing `COPY /db/changelog/` line | Add the `COPY` instruction to the service `Dockerfile` and rebuild |
| `changelog destination path "/changelogs" is not writable` | emptyDir volume not mounted | Check `volumeMounts` in the Job spec |
| `required environment variable is not set` | Missing env var in Job spec | Add the missing variable to `migration-job.yaml` |

---

## 3. Check Liquibase runner logs

If the init container succeeded, check the Liquibase runner:

```bash
kubectl logs job/<job-name> -c liquibase -n <namespace>
```

### Common Liquibase failures

| Error | Root cause | Fix |
|---|---|---|
| `Connection refused` | DB not reachable from the Job pod | Check DB_URL, network policies, security groups |
| `liquibase.exception.LockException` | Stale lock in DATABASECHANGELOCKLOG | See "Clear a stale lock" below |
| `Validation failed` | Changelog checksum mismatch | A committed changeset was modified — restore original or clear checksums |
| `ChangeSet already ran` | Duplicate changeset ID | Fix duplicate IDs in changelog files |

---

## 4. Clear a stale Liquibase lock

If Liquibase failed mid-run, it may leave a lock. Connect to the database:

```sql
UPDATE DATABASECHANGELOGLOCK SET LOCKED = FALSE, LOCKGRANTED = NULL, LOCKEDBY = NULL WHERE ID = 1;
```

Or use Liquibase CLI:
```bash
liquibase --url=<db-url> --username=<user> --password=<pass> releaseLocks
```

---

## 5. Check the DATABASECHANGELOG table state

Connect to the PostgreSQL database and run:

```sql
SELECT ID, AUTHOR, FILENAME, DATEEXECUTED, EXECTYPE, MD5SUM
FROM DATABASECHANGELOG
ORDER BY DATEEXECUTED DESC
LIMIT 20;
```

This shows which changesets have been applied and in what order.

---

## 6. Manually rerun the Job

Delete the failed Job and re-create it to trigger another run:

```bash
kubectl delete job <job-name> -n <namespace>
kubectl apply -f migration-job.yaml -n <namespace>
```

Or trigger a fresh ArgoCD sync:

```bash
argocd app sync <app-name> --force
```

---

## 7. Escalation

If you cannot resolve the issue:

1. Check the platform team's on-call Slack channel.
2. Open an incident in PagerDuty with tag `platform-db-migrator`.
3. Attach the output of `kubectl describe job` and `kubectl logs` for both containers.
