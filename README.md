# PassBi Core — Multimodal Transit Routing Engine

**PassBi** is a fast, robust, and explainable multimodal routing engine designed for West African transit data. Built from scratch in Go with PostgreSQL + PostGIS.

## Features

✅ **Multi-Strategy Routing** — Returns 4 distinct routes: `no_transfer`, `direct`, `simple`, and `fast`
✅ **GTFS Import** — Handles incomplete and heterogeneous GTFS feeds
✅ **PostGIS-Powered** — Efficient geospatial queries and graph storage
✅ **Redis Caching** — Multi-level caching with mutex locks
✅ **Stateless & Scalable** — Horizontal scaling ready
✅ **Performance** — <500ms P95 response time

---

## Architecture

```
┌─────────────┐
│   Client    │
└──────┬──────┘
       │ HTTP
       ▼
┌─────────────────────────────────────┐
│         Fiber API Server            │
│  ┌────────────────────────────┐    │
│  │  Route Search Handler       │    │
│  │  ├─ Cache Check (Redis)     │    │
│  │  ├─ Parallel Strategy Exec  │    │
│  │  └─ Response Builder        │    │
│  └────────────────────────────┘    │
└──────────┬──────────────────────────┘
           │
           ▼
┌──────────────────────────────────────┐
│      Routing Engine (A*)             │
│  ┌─────────────────────────────┐    │
│  │ Direct Strategy             │    │
│  │ Simple Strategy             │    │
│  │ Fast Strategy               │    │
│  └─────────────────────────────┘    │
└──────────┬───────────────────────────┘
           │
           ▼
┌──────────────────────────────────────┐
│   PostgreSQL + PostGIS               │
│  ┌─────────────────────────────┐    │
│  │ Graph (nodes + edges)       │    │
│  │ Stops, Routes, Trips        │    │
│  │ Geospatial Indexes          │    │
│  └─────────────────────────────┘    │
└──────────────────────────────────────┘
```

---

## Quick Start

### Prerequisites

- **Go 1.22+**
- **PostgreSQL 15+** with **PostGIS 3+**
- **Redis 6+**

### 1. Clone and Install

```bash
git clone <repo-url>
cd passbi_core
go mod download
```

### 2. Configure Environment

```bash
cp .env.example .env
# Edit .env with your database and Redis credentials
```

### 3. Run Database Migrations

```bash
# Install golang-migrate
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@latest

# Run migrations
migrate -path migrations -database "postgres://user:pass@localhost:5432/passbi?sslmode=disable" up
```

### 4. Import GTFS Data

```bash
go run cmd/importer/main.go \
  --agency-id=dakar_dem_dikk \
  --gtfs=./gtfs_dakar.zip \
  --rebuild-graph
```

### 5. Start API Server

```bash
go run cmd/api/main.go
```

The server will start on `http://localhost:8080`

### 6. Test the API

```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

---

## 📖 API Documentation

**For developers integrating PassBi API into their applications:**

- **[🚀 5-Minute Quickstart](docs/guides/quickstart-integration.md)** - Get started in 5 minutes
- **[📘 Complete Integration Guide](docs/guides/integration-guide.md)** - Full integration tutorial
- **[🎯 Routing Strategies Explained](docs/guides/routing-strategies.md)** - Understanding the algorithms
- **[📚 Full API Documentation](docs/README.md)** - Complete documentation hub

**Quick Links:**
- [OpenAPI Specification](docs/api/openapi.yaml) - Machine-readable API spec
- [JavaScript Examples](docs/api/examples/javascript.md) - Code examples for web/Node.js
- [Python Examples](docs/api/examples/python.md) - Code examples for Python
- [cURL Examples](docs/api/examples/curl.md) - Command-line testing
- [Error Reference](docs/api/reference/errors.md) - Error codes and troubleshooting
- [Data Models](docs/api/reference/data-models.md) - Complete data structures

---

## API Reference

### `GET /v2/route-search`

Find routes between two coordinates.

**Query Parameters:**
- `from` (required): Origin coordinates as `lat,lon`
- `to` (required): Destination coordinates as `lat,lon`

**Example Request:**
```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

**Example Response:**
```json
{
  "routes": {
    "direct": {
      "duration_seconds": 1200,
      "walk_distance_meters": 150,
      "transfers": 0,
      "steps": [
        {
          "type": "WALK",
          "from_stop": "stop_123",
          "to_stop": "stop_124",
          "duration_seconds": 120,
          "distance_meters": 150
        },
        {
          "type": "RIDE",
          "from_stop": "stop_124",
          "to_stop": "stop_456",
          "route": "route_A",
          "mode": "BUS",
          "duration_seconds": 1080
        }
      ]
    },
    "simple": {
      "duration_seconds": 900,
      "walk_distance_meters": 300,
      "transfers": 1,
      "steps": [...]
    },
    "fast": {
      "duration_seconds": 720,
      "walk_distance_meters": 500,
      "transfers": 2,
      "steps": [...]
    }
  }
}
```

### `GET /health`

Health check endpoint.

**Example Response:**
```json
{
  "status": "healthy",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

### `GET /v2/stops/nearby` 🆕

Find stops within a radius of a location.

**Query Parameters:**
- `lat` (required): Latitude
- `lon` (required): Longitude
- `radius` (optional): Search radius in meters (default: 500)

**Example Request:**
```bash
curl "http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"
```

**Example Response:**
```json
{
  "stops": [
    {
      "id": "D_771",
      "name": "Face Eglise Temple Évangélique",
      "lat": 14.692267,
      "lon": -17.447672,
      "distance_meters": 120,
      "routes": ["D105CP", "D111LY", "D7OP"],
      "routes_count": 3
    }
  ]
}
```

### `GET /v2/routes/list` 🆕

List all available routes with filtering options.

**Query Parameters:**
- `mode` (optional): Filter by mode (BUS, BRT, TER)
- `agency` (optional): Filter by agency ID
- `limit` (optional): Limit results (default: 100, max: 1000)

**Example Request:**
```bash
curl "http://localhost:8080/v2/routes/list?mode=BUS&limit=10"
```

**Example Response:**
```json
{
  "routes": [
    {
      "id": "DDD_01",
      "name": "D1LP",
      "mode": "BUS",
      "agency_id": "dakar_dem_dikk",
      "stops_count": 75
    }
  ],
  "total": 134
}
```

---

## Routing Strategies

PassBi provides 4 distinct routing strategies:

### 1. **No Transfer** (`no_transfer`) 🆕
- **Goal**: Absolutely zero transfers - single line only
- **Best for**: Maximum comfort, passengers with luggage
- **Trade-off**: May be slower, moderate walking tolerance

### 2. **Direct** (`direct`)
- **Goal**: Minimize or eliminate transfers
- **Best for**: Users who prefer simplicity
- **Trade-off**: May take longer, heavy walk penalty

### 3. **Simple** (`simple`)
- **Goal**: Balance time, walking, and transfers
- **Best for**: General use (default strategy)
- **Trade-off**: Moderate in all aspects

### 4. **Fast** (`fast`)
- **Goal**: Minimize total travel time
- **Best for**: Users in a hurry
- **Trade-off**: May require more walking/transfers

---

## GTFS Import

### Import Command

```bash
go run cmd/importer/main.go \
  --agency-id=<agency_id> \
  --gtfs=<path_to_zip> \
  --rebuild-graph \
  --dedupe-threshold=30
```

**Flags:**
- `--agency-id` (required): Unique identifier for the agency
- `--gtfs` (required): Path to GTFS ZIP file
- `--rebuild-graph`: Rebuild routing graph after import
- `--dedupe-threshold`: Stop deduplication threshold in meters (default: 30)

### Import Process

1. **Parse** GTFS files (stops, routes, trips, stop_times)
2. **Validate** data (skip invalid entries)
3. **Normalize** (deduplicate stops, infer modes)
4. **Import** to database (transactional)
5. **Build Graph** (nodes and edges)
6. **Analyze** tables for query optimization

### Handling Incomplete GTFS

PassBi gracefully handles:
- Missing optional files (calendar.txt, shapes.txt)
- Stops without coordinates (skipped)
- Missing arrival/departure times (interpolated)
- Invalid route types (inferred from name)

---

## Configuration

### Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `DB_HOST` | `localhost` | PostgreSQL host |
| `DB_PORT` | `5432` | PostgreSQL port |
| `DB_NAME` | `passbi` | Database name |
| `DB_USER` | `postgres` | Database user |
| `DB_PASSWORD` | `` | Database password |
| `DB_SSLMODE` | `disable` | SSL mode |
| `REDIS_HOST` | `localhost` | Redis host |
| `REDIS_PORT` | `6379` | Redis port |
| `REDIS_PASSWORD` | `` | Redis password |
| `API_PORT` | `8080` | API server port |
| `CACHE_TTL` | `10m` | Route cache TTL |
| `MAX_WALK_DISTANCE` | `500` | Max walk distance (m) |
| `WALKING_SPEED` | `1.4` | Walking speed (m/s) |
| `TRANSFER_TIME` | `180` | Transfer time (s) |

---

## Performance

### Targets

- **P50**: <200ms
- **P95**: <500ms
- **P99**: <1s
- **Memory**: <100MB per instance

### Optimization Techniques

1. **Lazy Edge Loading** — Edges loaded on-demand during pathfinding
2. **PostGIS Indexes** — GIST indexes on geographies
3. **Redis Caching** — 10-minute TTL with mutex locks
4. **Parallel Strategy Execution** — All 3 routes computed concurrently
5. **Connection Pooling** — pgx pool (min=5, max=20)

### Load Testing

```bash
# Install hey
go install github.com/rakyll/hey@latest

# Run load test
hey -z 30s -c 50 'http://localhost:8080/v2/route-search?from=14.7,-17.4&to=14.8,-17.3'
```

---

## Development

### Project Structure

```
passbi_core/
├── cmd/
│   ├── api/              # HTTP server
│   └── importer/         # GTFS import CLI
├── internal/
│   ├── api/              # HTTP handlers
│   ├── cache/            # Redis caching
│   ├── db/               # Database connection
│   ├── graph/            # Graph builder
│   ├── gtfs/             # GTFS parser
│   ├── models/           # Data models
│   └── routing/          # Routing engine
├── migrations/           # SQL migrations
├── go.mod
└── README.md
```

### Running Tests

```bash
go test ./...
```

### Building Binaries

```bash
# Build API server
go build -o bin/passbi-api cmd/api/main.go

# Build importer
go build -o bin/passbi-import cmd/importer/main.go
```

---

## Deployment

### Docker

```bash
# Build image
docker build -t passbi-api .

# Run container
docker run -p 8080:8080 \
  -e DB_HOST=postgres \
  -e REDIS_HOST=redis \
  passbi-api
```

### Docker Compose

```bash
docker-compose up
```

---

## Troubleshooting

### No routes found

- **Check**: Are there nodes near origin/destination?
  ```sql
  SELECT COUNT(*) FROM node;
  SELECT COUNT(*) FROM edge;
  ```
- **Solution**: Re-import GTFS with `--rebuild-graph`

### Slow queries

- **Check**: Are indexes built?
  ```sql
  \d+ stop
  \d+ node
  \d+ edge
  ```
- **Solution**: Run `ANALYZE stop; ANALYZE node; ANALYZE edge;`

### Redis connection errors

- **Check**: Is Redis running?
  ```bash
  redis-cli PING
  ```
- **Solution**: Start Redis or update `REDIS_HOST`

---

## Roadmap

- [ ] GTFS-RT support
- [ ] Real-time vehicle tracking
- [ ] Fare calculation
- [ ] Multi-language support
- [ ] Mobile SDK

---

## License

MIT License

---

## Support

For issues and questions, please open an issue on GitHub.
