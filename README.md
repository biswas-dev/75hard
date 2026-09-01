# 75 Hard

A tracker for the 75 Hard challenge, deployed at **[75hard.biswas.me](https://75hard.biswas.me)**.

Sign up, start a program, and check off each day's tasks with one tap. Progress
photos and food photos are compressed in the browser before upload, calories and
training are logged per day, and the six classic rules are the *starting*
template rather than a fixed list — every task is editable.

## Features

- **Accounts** — email and password, plus optional Google and GitHub sign-in via
  [go-login](https://github.com/anchoo2kewl/go-login).
- **Customizable programs** — start from the canonical six tasks, then add,
  edit, reorder or remove any of them. Choose the length, the start date, a
  daily calorie target, and whether a missed day restarts the attempt.
- **Task kinds** — a tick, a count toward a target (128 oz of water), minutes
  toward a target (45 minutes of training), a photo, or a note. Metered tasks
  show partial progress and only complete at their target.
- **Timezone-correct days** — "today" is computed in the user's own zone, so an
  11pm check-off lands on the day they think it is.
- **Photos** — compressed to 1600px WebP in the browser, re-encoded and
  thumbnailed server-side, stored on disk and served only to their owner. A
  6.8MB phone photo lands at roughly 275KB.
- **Food and training** — meals with an optional itemised breakdown, macro
  totals, and workouts that can credit a duration task directly.
- **Progress** — a 75-day calendar, per-task consistency, streaks, and a weight
  series.
- **AI coach** — photograph a meal and get an itemised calorie and macro
  estimate; recipes that fit what's left of today's budget; a weekly plan and a
  short daily note, both written from your own logged record. Powered by
  [go-ai](https://github.com/anchoo2kewl/go-ai) with a primary provider and two
  backups.

## Stack

| | |
|---|---|
| Backend | Go 1.26, chi v5, SQLite (`modernc.org/sqlite`, pure Go — no CGO), zap |
| Frontend | React 18, TypeScript, Vite 5, Tailwind 3.4, React Router 6 |
| Auth | JWT (HS256) + bcrypt, OAuth via `go-login` |
| Deploy | One container behind host nginx; Harbor registry, GitHub Actions |

One binary serves the API, the photos and the built SPA. State lives entirely
under `/data` — the SQLite database and the photo tree — so a backup is a copy
of one directory.

## Layout

```
api/
  cmd/api/main.go            router, route registration, graceful shutdown
  internal/
    api/                     handlers, middleware, response helpers, SPA server
    auth/                    JWT + bcrypt
    config/                  env-driven config and the logger
    db/                      SQLite connection, migrations, the go-login store
    photo/                   decode, downscale, thumbnail, disk layout
    program/                 the challenge rules — day rollover, streaks, restart
    version/                 build metadata from ldflags
web/
  src/
    components/              TaskRow (the check-off), Confetti, PhotoUpload, …
    routes/                  Today, Calendar, DayDetail, Photos, Stats, Settings
    lib/                     API client, types, browser-side image compression
    state/                   auth context
deployment/nginx-ssl.conf    host nginx site
```

`internal/program` holds every rule decision and is free of HTTP and database
concerns, so the tricky parts — what "today" means, when a day is missed, what a
miss does to the attempt — are unit-tested directly.

## Running locally

```bash
# API plus the built SPA on one port
cd api && go run ./cmd/api          # http://localhost:8087

# or with the frontend hot-reloading
cd web && npm install && npm run dev  # http://localhost:5175, proxies /api
```

With the [`me`](https://github.com/anchoo2kewl/me) runner:

```bash
./.me/scripts/server local dev 75hard
./.me/scripts/server local test 75hard
./.me/scripts/server prod health 75hard
```

## Tests

```bash
cd api && go vet ./... && go test ./...
cd web && npx tsc --noEmit && npx vite build
```

## Configuration

Everything is environment-driven; the defaults suit local development.

| Variable | Default | Notes |
|---|---|---|
| `ENV` | `development` | `production` enforces a real `JWT_SECRET` |
| `PORT` | `8087` | 8080 in the container |
| `APP_URL` | `http://localhost:8087` | Used to build OAuth redirect URLs |
| `DB_PATH` | `./data/75hard.db` | |
| `PHOTOS_DIR` | `./data/photos` | |
| `FRONTEND_DIST` | — | Path to `web/dist`; unset serves the API alone |
| `JWT_SECRET` | dev value | **Required** in production |
| `JWT_EXPIRY_HOURS` | `720` | |
| `MAX_UPLOAD_MB` | `15` | |
| `MAX_PHOTO_EDGE` / `THUMB_EDGE` | `1600` / `320` | Longest side, in pixels |
| `ALLOW_SIGNUP` | `true` | Set false to close registration |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | — | Seeds an admin on first boot only |
| `OAUTH_STATE_SECRET` | — | Must differ from `JWT_SECRET` |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` | — | Optional |
| `LOGIN_GITHUB_CLIENT_ID` / `LOGIN_GITHUB_CLIENT_SECRET` | — | Optional |

### AI providers

The AI features are off unless a provider chain is configured. Slots are read
in order and the scan stops at the first gap, so slot 1 is the primary and 2
and 3 are backups:

```bash
AI_1_PROVIDER=anthropic
AI_1_MODEL=claude-sonnet-5
AI_1_API_KEY=sk-ant-...

AI_2_PROVIDER=openai          # backup 1
AI_2_MODEL=gpt-5.2
AI_2_API_KEY=sk-...

AI_3_PROVIDER=ollama          # backup 2, self-hosted
AI_3_MODEL=llama3.1
AI_3_BASE_URL=http://localhost:11434/v1
```

The chain falls through on a rate limit, a 5xx, a timeout or a dead key, but
not on a 400 — a malformed request would be rejected identically by the backup.
Every call is recorded in `ai_runs`, which is both the audit trail and the
quota counter (40 calls per user per rolling 24 hours), and identical inputs
reuse the stored result rather than paying twice.

Anthropic is worth putting first for the food-photo feature: it's the vision
path the library implements natively.

## Deployment

Push to `main` and GitHub Actions tests, builds, pushes to
`harbor.biswas.me/biswas/75hard` and deploys on the production host, which runs
the self-hosted runner. See `.github/workflows/ci-cd.yml`.

First-time setup on a new host:

1. **Cloudflare** — add an `A` record for `75hard` pointing at the prod host,
   proxied, with SSL mode Full (Strict).
2. **Origin certificate** — Cloudflare dashboard → the `biswas.me` zone →
   SSL/TLS → Origin Server → Create Certificate for `75hard.biswas.me`. Save
   the PEM blocks to `/etc/ssl/75hard.biswas.me/origin-cert.pem` and
   `origin-key.pem`, then `chmod 600` the key.
3. **nginx** — `sudo cp deployment/nginx-ssl.conf
   /etc/nginx/sites-enabled/75hard.biswas.me && sudo nginx -t && sudo nginx -s reload`
4. **Secrets** — add the repository secrets listed at the top of the workflow.
5. Push to `main`.

The container binds `127.0.0.1:13431`; nginx terminates TLS in front of it.

## Backups

Everything is under `/opt/75hard/data`. Stop the container (or use
`sqlite3 .backup` for a hot copy) and archive the directory — that is the
database and every photo.

## License

MIT
