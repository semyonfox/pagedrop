# PageDrop operations handoff

## Service

- Public URL: <https://pagedrop.semyon.ie>
- Local origin: `http://127.0.0.1:8788`
- Application container: `pagedrop`
- Tunnel container: `pagedrop-tunnel`
- Persistent Docker volume: `pagedrop-data`
- Restart policy: `unless-stopped`

The application serves static uploads only. Publishing is anonymous.
Management requests require `PAGEDROP_TOKEN`; the Cloudflare connector uses a
separate tunnel token. Keep both out of the repository and command output.

The checked-in `compose.yaml` is the preferred reproducible deployment. The
current live tunnel was initially started manually with host networking and
forwards to the local origin.

## Routine checks

```bash
curl --fail http://127.0.0.1:8788/health
curl --fail https://pagedrop.semyon.ie/health
docker ps --filter name=pagedrop
docker logs --tail 100 pagedrop
docker logs --tail 100 pagedrop-tunnel
```

With the CLI configured and the admin token available:

```bash
pagedrop stats --server https://pagedrop.semyon.ie
pagedrop list --server https://pagedrop.semyon.ie
```

`pagedrop stats --json` is suitable for scripts. It reports active, expired,
and deleted page counts, stored file and byte totals, and the nearest active
expiry.

## Upgrade

Before changing the live service:

1. Run `make check`.
2. Build a versioned image; do not replace the rollback image tag.
3. Preserve `PAGEDROP_TOKEN`, `PAGEDROP_PUBLIC_BASE_URL`, expiry settings, and
   the upload rate/proxy settings and `pagedrop-data:/data` mount.
4. Start the replacement with host port `127.0.0.1:8788` mapped to container
   port `8080`.
5. Verify local health, public health, and an authenticated `pagedrop stats`.
6. Keep the previous stopped container until the new version has settled.

For a clean installation, copy `.env.example` to `.env`, set both tokens and
the public base URL, then use:

```bash
docker compose --profile tunnel up --build -d
```

## Rollback

The deployment made on 2026-07-23 retained these stopped containers:

- `pagedrop-landing-20260723`: landing-page release immediately before stats
- `pagedrop-previous-20260723`: earlier application release

To roll back, stop the current `pagedrop`, give it a unique diagnostic name,
rename the selected stopped container to `pagedrop`, and start it. Confirm
`/health` locally and publicly afterward. Do not delete `pagedrop-data`; all
releases share it.

## Backup

The `pagedrop-data` volume contains both the SQLite database and uploaded page
files. Back up the complete volume consistently. The simplest safe procedure is
to stop `pagedrop`, snapshot or copy the volume, and then restart it. The tunnel
may remain running during this short maintenance window.

## CI and management scope

Jenkins is intentionally not part of this deployment. The repository's
`make check`, versioned image build, health checks, and rollback container are
enough for the present single-owner service. Add CI only when releases become
frequent enough that the manual checklist is a recurring source of mistakes.

There is deliberately no web admin dashboard. The authenticated CLI provides
the useful management surface without adding sessions, cookies, or another
privileged UI to secure.

## Abuse control

Anonymous publishing is protected by the application limits (10 MiB uploaded,
50 MiB extracted, 500 archive entries, one-day default expiry, seven-day
maximum) and a built-in upload rate limit. Production sets
`PAGEDROP_UPLOADS_PER_MINUTE=5` and `PAGEDROP_TRUST_PROXY_HEADERS=true`, so
PageDrop groups requests by Cloudflare's validated `CF-Connecting-IP` value.
Enable trusted proxy headers only while the origin remains private behind the
tunnel.

An equivalent Cloudflare rate-limiting rule for `POST /api/v1/pages` is useful
defense in depth when the zone plan supports method matching. Keep public page
reads and authenticated management requests outside that edge rule. The
application limit remains authoritative because Cloudflare rate-limit matching
capabilities vary by plan.
