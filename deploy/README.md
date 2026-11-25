# Production deployment and recovery

Production uses PostgreSQL on a private Compose network, API and worker
containers, a scheduled retention process, and Caddy for automatic HTTPS. The
mirror directory is mounted read-only. PostgreSQL has no published host port.
Open inbound TCP 80/443 (and optionally UDP 443 for HTTP/3) at the host firewall;
restrict SSH separately. Point DNS `PUBLIC_DOMAIN` at the host before starting
Caddy.

## Configure and deploy

1. Copy `.env.production.example` to `.env.production` in this directory. Use a
   password manager or `go run ./cmd/admin-jwt -generate-secret` to create a
   random signing key, and create unique database credentials. Keep the file
   mode restricted and do not commit it.
   `DB_PASSWORD` must match `POSTGRES_PASSWORD`.
2. Set `PUBLIC_DOMAIN`, `MIRROR_DIR`, CORS origin, and database settings. The
   only published web service is Caddy. The API listens on the Compose network.
   The local database uses a private bridge network; for an external PostgreSQL
   service configure `DB_SSLMODE=verify-full` and its trusted certificate.
3. Build and start from the repository root:

   ```sh
   docker compose --env-file deploy/.env.production -f deploy/compose.production.yaml config --quiet
   docker compose --env-file deploy/.env.production -f deploy/compose.production.yaml up -d --build
   ```

   The one-shot `migrate` service applies additive migrations before API,
   worker, and retention start. Caddy obtains and renews certificates through
   ACME; DNS and inbound port 80/443 must work. Store GeoLite2 City and Nginx
   logs outside the application image.
4. Verify `https://$PUBLIC_DOMAIN/health/live`,
   `https://$PUBLIC_DOMAIN/health/ready`, and authenticated
   `/api/v1/admin/workers/analytics` with a JWT issued for an operator subject.
   Confirm the worker has a recent success
   and no accumulating pending events. Check Caddy/API/worker/retention logs.

For an operator-reviewed catalog, run `docker compose ... run --rm
catalog-import /manifest.json` after mounting/providing the manifest to that
one-shot importer. For Nginx data, set `NGINX_ACCESS_LOG_FILE` and
`GEOIP_DB_FILE`, then run `docker compose ... --profile import run --rm
nginx-import`. The access log is read-only; the importer checkpoint is stored
in a persistent named volume. This profile is a one-shot batch and should be
rerun after a new log interval or rotation.

## Backup and restore drill

Install `age` on the host and provision an offsite age recipient. Schedule the
encrypted dump at least daily (for example, systemd timer or cron at 02:15 UTC):

```sh
BACKUP_AGE_RECIPIENT='age1...' bash deploy/backup.sh deploy/.env.production /secure/local-backups
```

Copy encrypted output to an access-controlled offsite bucket with versioning
and a 30-day storage lifecycle. Keep the private age identity separate from
the database host. Alert on missing/failed backups and test the lifecycle rule.
The script fails if the encryption recipient is not configured. It writes to a
temporary file and only publishes the timestamped backup after both dump and
encryption succeed; interrupted/failed runs remove the partial file.

At least quarterly, restore the newest backup into a fresh isolated database:

```sh
BACKUP_AGE_IDENTITY=/secure/keys/backup.agekey \
  bash deploy/restore-test.sh /secure/local-backups/pinguin-....dump.age restore_drill_20261124 deploy/.env.production
docker compose --env-file deploy/.env.production -f deploy/compose.production.yaml \
  exec -T postgres psql -U pinguin_app -d restore_drill_20261124 \
  -c 'SELECT version FROM schema_migrations ORDER BY version'
```

Confirm the expected migration versions and record counts, then test the API
against the restored database in an isolated environment. Do not point production
traffic at the restore-test database. Delete it only after the drill is signed
off and evidence is retained. The restore script requires the `restore_` prefix
and refuses to overwrite an existing database.
If decryption or restore fails, the newly created `restore_` database is dropped
automatically. A successful restore is kept for inspection and must be deleted
only after the drill evidence is recorded.

To preflight both encryption and restore wrappers locally on Windows, install
Docker Desktop, Git Bash, `age`, and `age-keygen`, then run
`deploy/backup-age-smoke.ps1 -AgeToolDirectory <age-bin-directory>` from the
repository root. The smoke creates a uniquely named Compose database, encrypts
a seeded PostgreSQL dump, restores it under a separate `restore_` database, and
cleans up its temporary key, backup, database, and volume.

## Release, rollback, and migration policy

Before release: review OpenAPI/CI, build images, check migration checksums,
confirm backup age, inspect secrets and CORS, verify mirror mount permissions,
and verify Caddy's certificate. Deploy a reviewed commit and retain its source
revision/image tags. After release: check readiness, Caddy logs, admin worker
status, pending-event trend, and a representative listing/download.

Rollback application services by checking out/redeploying the prior reviewed
revision and rebuilding its API/worker images. Migrations are not automatically
rolled back: a newer app rollback is safe only while the applied schema remains
backward-compatible. Migration `000006` adds worker columns and indexes;
`000007` adds repository path prefixes. Their down migrations discard runtime
state/configuration and should not run during routine rollback. For a destructive or
incompatible migration, use a tested forward-fix or restore into a new database
and plan a controlled cutover; document downtime and data-loss point before
switching traffic. Never run a down migration against production as an
automated deployment step.

## Decisions and external checks still required before production sign-off

- Retention periods are approved: 90 days for aggregated raw events, 365 days
  for survey answers, 90 days for contacts, 365 days for audit logs, and
  indefinite retention for aggregates. Idempotency keys expire at `expires_at`.
- The current identity decision is individual subject/role JWTs issued by the
  operator CLI without SSO. Production owners should assign a CLI maintainer and
  document the secure token delivery and emergency key-rotation contact. For
  routine rotation, deploy a new `JWT_SECRET` with the former key in
  `JWT_SECRET_PREVIOUS`, restart the API, and issue replacement tokens using the
  new current key. Remove the previous key after the 24-hour maximum token TTL
  (plus clock leeway) has elapsed. Emergency rotation omits the previous key to
  revoke tokens signed by the compromised key immediately.
- Assign a maintainer and approved upstream source for catalog manifests,
  provide a sanitized Nginx sample from the mirror operator, and provision a
  licensed production GeoLite2 City database plus its update procedure.
- Provision the real DNS, SMTP-independent ACME reachability, offsite backup
  destination, and backup-age recipient/identity custody.
- Run and record the restore drill in the actual target environment. No live
  infrastructure, public log sample, or licensed GeoLite2 database is present
  in this repository, so those steps are documented but not claimed as done.
