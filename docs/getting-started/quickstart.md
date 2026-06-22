# Quickstart

Get a route in 30 seconds, production-ready in 5 minutes.

---

## 1. First request

```bash
curl "https://passbi-api.onrender.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

```javascript
const res = await fetch('https://passbi-api.onrender.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467');
const data = await res.json();
```

```python
import requests
data = requests.get('https://passbi-api.onrender.com/v2/route-search',
    params={'from': '14.7167,-17.4677', 'to': '14.6928,-17.4467'}).json()
```

---

## 2. Read the response

The API returns up to 4 route options, keyed by strategy:

```json
{
  "routes": {
    "no_transfer": { "duration_seconds": 1800, "transfers": 0, "steps": [] },
    "direct":      { "duration_seconds": 1500, "transfers": 1, "steps": [] },
    "simple":      { "duration_seconds": 1200, "transfers": 1, "steps": [] },
    "fast":        { "duration_seconds": 900,  "transfers": 2, "steps": [] }
  }
}
```

> [!TIP]
> Use `simple` as your default. It gives the best balance of time and comfort for most users.

Each route has:

| Field | Type | Description |
|-------|------|-------------|
| `duration_seconds` | `number` | Total trip time |
| `walk_distance_meters` | `number` | Total walking distance |
| `transfers` | `number` | Number of transfers |
| `steps` | `Step[]` | Turn-by-turn directions |

Each step has `type: "WALK" | "RIDE" | "TRANSFER"`.

---

## 3. Render directions

```javascript
function renderRoute(route) {
  const mins = Math.floor(route.duration_seconds / 60);
  console.log(`${mins} min — ${route.transfers} transfer(s) — ${route.walk_distance_meters}m walk`);

  for (const step of route.steps) {
    if (step.type === 'WALK')     console.log(`  Walk ${step.distance_meters}m to ${step.to_stop_name}`);
    if (step.type === 'RIDE')     console.log(`  Ride ${step.route_name} (${step.num_stops} stops)`);
    if (step.type === 'TRANSFER') console.log(`  Transfer at ${step.from_stop_name}`);
  }
}

const recommended = data.routes.simple ?? data.routes.direct ?? data.routes.fast;
renderRoute(recommended);
```

---

## 4. Handle errors

```javascript
async function searchRoute(from, to) {
  const url = new URL('https://passbi-api.onrender.com/v2/route-search');
  url.searchParams.set('from', from);
  url.searchParams.set('to', to);

  const res = await fetch(url);

  if (res.status === 404) throw new Error('NO_ROUTES_FOUND');
  if (res.status === 400) {
    const { error } = await res.json();
    throw new Error(`INVALID_PARAMS: ${error}`);
  }
  if (!res.ok) throw new Error(`HTTP_${res.status}`);

  return res.json();
}
```

> [!NOTE]
> `404` means no transit connection was found — not that the endpoint is wrong. Use `/v2/stops/nearby` to verify both locations are near a stop.

---

## 5. Add caching

Routes are GTFS-static. Cache aggressively.

```javascript
const cache = new Map();
const TTL = 10 * 60 * 1000; // 10 minutes

async function cachedSearch(from, to) {
  const key = `${from}|${to}`;
  const hit = cache.get(key);
  if (hit && Date.now() - hit.ts < TTL) return hit.data;

  const data = await searchRoute(from, to);
  cache.set(key, { data, ts: Date.now() });
  return data;
}
```

| Resource | Recommended TTL |
|----------|----------------|
| Route search | 10 min |
| Nearby stops | 1 hour |
| Routes list | 24 hours |
| Departures | 1 min |

---

## Find nearby stops

Use this to validate that a location is served by transit before searching routes.

```bash
curl "https://passbi-api.onrender.com/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"
```

```json
{
  "stops": [
    { "id": "stop_123", "name": "Gare Routière", "distance_meters": 120, "routes": ["D1LP", "D5TH"] }
  ]
}
```

---

## Next steps

- [Authentication](authentication.md) — API keys for partner access
- [Error reference](../api/reference/errors.md) — all error codes
- [Routing strategies](../guides/routing-strategies.md) — algorithm details
- [TypeScript types](../schemas/typescript/types.ts) — full type definitions
- [OpenAPI spec](../api/openapi.yaml) — generate client SDKs
