# Copilot instructions for PassBi Core

Quick, focused guidance to help AI agents contribute productively.

## Purpose
- Help with features, fixes and tests for the PassBi routing engine (Go + Postgres/PostGIS + Redis).

## Big picture
- HTTP server: `cmd/api/main.go` — Fiber-based API that exposes `/v2/route-search`, `/v2/stops/nearby`, `/health`.
- GTFS importer CLI: `cmd/importer/main.go` — parses GTFS, deduplicates stops, imports DB rows, optionally rebuilds graph.
- Routing engine: `internal/routing` — A*-style search with pluggable strategies in `strategy.go` (names: `no_transfer`, `direct`, `simple`, `fast`).
- Storage: PostgreSQL + PostGIS (see `internal/db/connection.go` and `migrations/`), graph tables are `node` and `edge`.
- Cache & coordination: Redis singleton in `internal/cache/redis.go` (route keys, lock/wait pattern, TTLs controlled by env).

## Key workflows (what to run)
- Dev server: `go run cmd/api/main.go` (or `make run`).
- Build binaries: `make build` or `go build -o bin/passbi-api cmd/api/main.go`.
- GTFS import: `go run cmd/importer/main.go --agency-id=<id> --gtfs=path.zip --rebuild-graph` (also `make import GTFS=... AGENCY=...`).
- Migrations: use `migrate -path migrations -database "postgres://user:pass@host:5432/passbi?sslmode=disable" up` (see `Makefile` targets).
- Docker local: `./scripts/deploy-local.sh` or `docker-compose up --build` (Dockerfile builds two binaries: `passbi-api` and `passbi-import`).
- Tests: `go test ./...`.

## Project-specific conventions & patterns
- Configuration is environment-driven: consult `.env.example` and `getEnv` wrappers used across `internal/*` packages.
- Singletons: DB and Redis use `sync.Once` singletons (`internal/db`, `internal/cache`). Avoid reinitializing in normal code; use `InitPoolWithConfig` for tests.
- Caching pattern: route results are stored by a hashed coordinate key (`cache.RouteKey`), with `lock:` keys used to implement a wait-for-result pattern to avoid thundering herd.
- Routing model: `internal/models` uses `Node` as a (stop, route) pair and `Edge` with typed costs (`EdgeWalk`, `EdgeRide`, `EdgeTransfer`). When adding strategy behavior, modify `internal/routing/strategy.go`.
- GTFS import: import does stop deduplication (default threshold 30m). If changing import semantics, update `internal/gtfs` and ensure `graph.Builder` remains compatible.

## Integration points & important files
- API handlers: [internal/api/handlers.go](internal/api/handlers.go#L1-L1)
- Routing strategies: [internal/routing/strategy.go](internal/routing/strategy.go#L1-L1)
- DB pool: [internal/db/connection.go](internal/db/connection.go#L1-L1)
- Redis cache and lock logic: [internal/cache/redis.go](internal/cache/redis.go#L1-L1)
- GTFS parser/normalizer: [internal/gtfs/normalize.go](internal/gtfs/normalize.go#L1-L1) and parser in `internal/gtfs`.
- Graph builder: [internal/graph/builder.go](internal/graph/builder.go#L1-L1)

## Small, actionable examples for edits
- Add a new routing strategy:
  - Implement `Strategy` in `internal/routing/strategy.go` with `Name()`, `EdgeCost()` and `ShouldStop()`.
  - Ensure `GetAllStrategies()` includes the new strategy and it follows cost conventions (time in seconds, walk in meters, transfer penalty in seconds).

- Change cache TTL: update `CACHE_TTL` in env or `internal/cache.LoadConfigFromEnv()` defaults.

- Add DB migration: put SQL in `migrations/` and document the change in README; CI/deploy runs `migrate` according to Makefile.

## Testing & debugging notes
- Use `InitPoolWithConfig` to create isolated DB pools in unit tests.
- The importer runs large transactions — use small test GTFS files and `--rebuild-graph` locally when iterating.
- Health endpoints: `GET /health` checks PostgreSQL (and PostGIS) and Redis; use this when orchestrating integration tests.

## Non-goals / things not to change lightly
- Do not change the `Node`/`Edge` schema without updating SQL in `migrations/` and the graph builder.
- Avoid changing cache key format (`cache.RouteKey`) — this will invalidate existing cached results.

If anything above is unclear or you want more examples (tests, a sample GTFS import flow, or strategy tuning notes), tell me which section to expand.
