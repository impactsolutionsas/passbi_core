# Changelog

All notable API changes follow [Semantic Versioning](https://semver.org).

---

## [2.0.0] — 2026-02-12

### Added
- `/v2/route-search` — ETAs per step (departure/arrival times)
- A* routing engine with 4 parallel strategies
- RAPTOR time-dependent engine (experimental, `?engine=raptor`)
- Climate impact metrics per route (`co2_grams`, `calories_burned`)
- `/v2/stops/search` — relevance-ranked stop name search
- Partner API — authentication, rate limiting, per-key quotas
- Audit log — all partner API calls recorded

### Changed
- Walk limit reduced to 200 m (was 500 m) for accuracy
- Stop search radius reduced to 500 m (was 1 km)
- Response format: `routes` object now keyed by strategy name

### Removed
- `/v1/` endpoints — use `/v2/` equivalents

---

## [1.0.0] — 2025-10-01

### Added
- Initial GTFS import pipeline (4 agencies, 2 855 stops, 134 routes)
- Basic route search with Dijkstra
- `/v1/stops/nearby` geospatial endpoint
- `/v1/routes/list` catalog endpoint
