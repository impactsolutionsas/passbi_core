# PassBi Core API

Multimodal transit routing API — 4 strategies, real ETAs, GTFS-powered.

**Base URL:** `https://passbi-api.onrender.com` · **Dev:** `http://localhost:8080`

---

## Get Started

```bash
curl "https://passbi-api.onrender.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

→ [Quickstart (5 min)](getting-started/quickstart.md) · [Authentication](getting-started/authentication.md)

---

## Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/health` | Health check |
| `GET` | `/v2/route-search` | Route planning — returns 4 strategies |
| `GET` | `/v2/stops/nearby` | Stops within radius |
| `GET` | `/v2/stops/search` | Stop name search |
| `GET` | `/v2/routes/list` | Route catalog |
| `GET` | `/v2/stops/:id/departures` | Upcoming departures |
| `GET` | `/v2/routes/:id/schedule` | Full timetable |
| `GET` | `/v2/routes/:id/trips` | Trip details |

→ [OpenAPI spec](api/openapi.yaml) · [Error reference](api/reference/errors.md)

---

## Routing Strategies

Every `/v2/route-search` response includes up to 4 options:

| Key | Optimizes for | Use when |
|-----|--------------|----------|
| `no_transfer` | Zero transfers | Luggage, elderly, comfort |
| `direct` | Fewest transfers | First-time users |
| `simple` | Balance (**default**) | General use |
| `fast` | Shortest time | Commuters |

→ [Strategy deep dive](guides/routing-strategies.md)

---

## Documentation

| | |
|---|---|
| [Quickstart](getting-started/quickstart.md) | First request in 30 seconds |
| [Authentication](getting-started/authentication.md) | API keys & partner access |
| [Error reference](api/reference/errors.md) | All error codes |
| [Routing strategies](guides/routing-strategies.md) | Algorithm details |
| [Partner integration](guides/partner-integration.md) | Rate limits, quotas, security |
| [TypeScript types](schemas/typescript/types.ts) | Type definitions |
| [Architecture](architecture/partner-api-architecture.md) | System design |

---

## Changelog

→ [CHANGELOG.md](CHANGELOG.md)
