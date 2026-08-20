# Developing Silo

This document covers building, running, and contributing to the Silo server. If
you just want to run Silo, see the [README](README.md).

See [CONTRIBUTING.md](CONTRIBUTING.md) for contribution expectations, pull
request guidance, and the policy for AI-assisted submissions.

## Prerequisites

- **Git**, **Make**, and **OpenSSL**
- **Docker Engine** or **Docker Desktop** with Docker Compose 2.24+ (local services and testcontainers)
- **Go** 1.26.4+
- **Node.js** 22+ with **pnpm** 10.32.1
- **PostgreSQL** 18 with pgvector
- **Redis**
- **FFmpeg** (for transcoding support)
- A **C compiler and build toolchain** (for CGO dependencies)
- **pkg-config** and the **libvips development headers** (for image processing through bimg)

## Local Development

Local development remains intentionally separate from the deploy-oriented
Compose setup. Use [docker-compose.yml](docker-compose.yml) for local services
and the source-build workflow below.

```sh
# Create the local bootstrap configuration
cp .env.example .env
chmod 600 .env
printf '\nSECRET_KEY=%s\nDATABASE_URL=%s\nREDIS_URL=%s\n' \
  "$(openssl rand -base64 48)" \
  'postgres://silo:silo@localhost:5432/silo?sslmode=disable' \
  'redis://localhost:6379' >> .env

# Start local PostgreSQL and Redis
docker compose up -d postgres redis

# Install frontend dependencies and create the embedded-frontend test stub
cd web
pnpm install --frozen-lockfile
cd ..
make embed-stub
```

Run the backend and frontend in separate terminals. Start the backend first:

```sh
make dev-backend
```

Then start the frontend dev server with its proxy pointed at the source backend:

```sh
VITE_API_PROXY_TARGET=http://localhost:8080 make dev-frontend
```

The template supplies a non-empty `MEDIA_ROOT` because Compose validates the whole file even when
you start only PostgreSQL and Redis. Change it before testing libraries against real media.

If you are developing `Silo` and `silo-plugin-sdk` together, use an untracked local `go.work`
workspace. `go.work` and `go.work.sum` are intentionally ignored and are developer conveniences
only. CI uses a clean checkout that does not contain them, and release builds explicitly set
`GOWORK=off`. Any new SDK helper used here must be pushed and tagged in `silo-plugin-sdk` before
this repository can merge or release the change.

Plugin authors should start with the `silo-plugin-sdk` repository, usually
checked out beside this one. It owns the RPC plugin package format, protobuf
contracts, generated plugin API, SDK import paths, and manifest helpers.

## Build and Run from Source

After creating `.env` and starting PostgreSQL and Redis as described above,
build the production frontend and Go binary:

```sh
make build
./silo
```

The source-built server listens at <http://localhost:8080> by default. Complete
onboarding and manage the remaining application settings through the web
interface.

## Make Targets

| Target | Description |
|---|---|
| `make build` | Build frontend + Go binary |
| `make frontend` | Build frontend only |
| `make dev-frontend` | Vite dev server with HMR |
| `make dev-backend` | Run Go backend (integrated mode) |
| `make dev-proxy` | Run a standalone proxy node |
| `make dev-transcode` | Run a standalone transcode node |
| `make migrate-create NAME=add_thing` | Create a timestamped Goose SQL migration |
| `make migrate-validate` | Validate Goose migration files without touching a database |
| `make migrate-status` | Show Goose migration status using Silo's bootstrapping runner |
| `make migrate-up` | Apply pending Goose migrations using Silo's bootstrapping runner |
| `make clean` | Remove build artifacts |

## Database Migrations

PostgreSQL schema migrations are managed by Goose. Migration SQL files live in
`migrations/sql/` and use Goose annotations. Converted legacy migrations keep
their original numeric versions so existing `schema_versions` rows can bootstrap
cleanly into Goose without replaying old SQL. New migrations should be created
with timestamped filenames:

```sh
make migrate-create NAME=add_thing
make migrate-validate
```

Do not run `goose fix`; timestamped migrations are the repository policy because
they avoid version collisions across parallel PRs. The existing `001`-style
files are historical compatibility records, not the naming pattern for new work.
Runtime migrations are applied by the integrated/API server only. Proxy and
transcode modes never mutate schema.
For existing installs, use `make migrate-status` and `make migrate-up` rather
than invoking the Goose CLI directly; those targets copy legacy
`schema_versions` rows into `public.goose_db_version` under the migration lock
before reading or applying migrations. Set `ENV_FILE=path/to/.env` when the
database URL should be read from a non-default env file.

## Running Tests

```sh
# Go tests (uses testcontainers — Docker must be running)
make test-go

# Frontend tests (uses the repository's current known-failure exclusions)
make test-web
```

## Linting

```sh
# Go formatting and vet
make embed-stub
gofmt -l .
go vet ./...

# Frontend
cd web
pnpm run lint
pnpm run format:check
cd ..
```

`gofmt -l .` must produce no output. Full-tree `golangci-lint` currently reports inherited
findings; pull-request CI runs it against changed lines from the target branch. See
[CONTRIBUTING.md](CONTRIBUTING.md#validate-your-change) for the complete pre-submission gate.

## Project Structure

```
cmd/silo/       Entry point
internal/
  api/               HTTP router, handlers, middleware
  auth/              JWT authentication and sessions
  catalog/           Media item, episode, season repositories
  config/            YAML + env var configuration
  jellycompat/       Jellyfin/Emby protocol compatibility
  metadata/          Plugin-driven metadata matching and enrichment
  playback/          Direct play, remux, transcode session management
  scanner/           Media file discovery and FFProbe
  worker/            Background jobs (scan, match, reconcile)
web/                 React + TypeScript frontend (Vite, Tailwind, shadcn/ui)
migrations/sql/      Goose-managed PostgreSQL schema migrations
```
