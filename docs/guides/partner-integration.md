# Partner Integration Guide

Everything you need to go from API key to production.

> [!NOTE]
> Don't have a key yet? See [Authentication](../getting-started/authentication.md) — keys are in the partner dashboard.

---

## Plans & limits

| Plan | req/s | req/day | req/month | Price |
|------|-------|---------|-----------|-------|
| Free | 2 | 1 000 | 30 000 | $0 |
| Starter | 10 | 10 000 | 300 000 | $49/mo |
| Business | 50 | 50 000 | 1 500 000 | $199/mo |
| Enterprise | 1 000 | unlimited | unlimited | custom |

Upgrade when you consistently hit 80% of your monthly quota or see frequent 429s.

---

## Rate limit handling

The API sets `Retry-After` (seconds) on `429` responses.

```javascript
async function fetchWithBackoff(url, options = {}, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    const res = await fetch(url, options);
    if (res.status !== 429) return res;

    const wait = parseInt(res.headers.get('Retry-After') ?? String(2 ** i), 10);
    await new Promise(r => setTimeout(r, wait * 1000));
  }
  throw new Error('RATE_LIMIT_MAX_RETRIES');
}
```

> [!TIP]
> Add client-side caching before adding retry logic — most 429s are caused by redundant requests that could be served from cache.

---

## Caching strategy

| Endpoint | TTL |
|----------|-----|
| `/v2/route-search` | 10 min |
| `/v2/stops/nearby` | 1 hour |
| `/v2/routes/list` | 24 hours |
| `/v2/stops/:id/departures` | 1 min |

```javascript
class CachedPassBiClient {
  #cache = new Map();

  async get(url, ttlMs) {
    const hit = this.#cache.get(url);
    if (hit && Date.now() - hit.ts < ttlMs) return hit.data;

    const res = await fetch(url, { headers: { Authorization: `Bearer ${process.env.PASSBI_API_KEY}` } });
    if (!res.ok) throw new Error(`HTTP_${res.status}`);

    const data = await res.json();
    this.#cache.set(url, { data, ts: Date.now() });
    return data;
  }

  searchRoutes(from, to) {
    const url = `https://passbi-api.onrender.com/v2/route-search?from=${from}&to=${to}`;
    return this.get(url, 10 * 60 * 1000);
  }

  nearbyStops(lat, lon, radius = 500) {
    const url = `https://passbi-api.onrender.com/v2/stops/nearby?lat=${lat}&lon=${lon}&radius=${radius}`;
    return this.get(url, 60 * 60 * 1000);
  }
}
```

---

## Security checklist

> [!WARNING]
> Violating these will get your key revoked.

**Do:**
- Store keys in environment variables (`PASSBI_API_KEY`)
- Use separate keys per environment (dev / staging / prod)
- Rotate keys immediately if compromised
- Proxy all API calls through your backend — never call PassBi directly from frontend

**Don't:**
- Commit keys to Git
- Share keys over email or chat
- Embed keys in mobile app binaries
- Use a single key across all environments

---

## Use cases

### Mobile app

```javascript
// Backend route (Node.js/Express)
app.get('/api/transit/routes', async (req, res) => {
  const { from, to } = req.query;
  const data = await passBiClient.searchRoutes(from, to);
  res.json(data);
});

// Mobile calls your backend, not PassBi
const routes = await fetch(`/api/transit/routes?from=${from}&to=${to}`).then(r => r.json());
```

### Commute scoring (Python backend)

```python
def score_commute(home: str, workplaces: list[dict]) -> list[dict]:
    results = []
    for wp in workplaces:
        route = client.search_routes(from_coords=home, to_coords=wp['coords'])
        best = route['routes']['simple']
        score = best['duration_seconds'] / 60 + best['transfers'] * 5 + best['walk_distance_meters'] / 100
        results.append({'workplace': wp['name'], 'score': score, **best})
    return sorted(results, key=lambda x: x['score'])
```

---

## Production checklist

- [ ] Production key created and stored in env vars
- [ ] Dev/staging keys separate from production
- [ ] Client-side caching implemented
- [ ] Rate limit retry with backoff implemented
- [ ] All API calls proxied through backend (no client-side key exposure)
- [ ] Error handling covers 400 / 404 / 429 / 500 / 503
- [ ] Health check integrated in startup / readiness probe
- [ ] Quota monitoring in place (alert at 80%)
- [ ] Load tested against your expected peak traffic

---

## Support

| Channel | Available |
|---------|-----------|
| Email `partners@passbi.com` | All plans |
| Live chat | Starter+ |
| Dedicated support | Enterprise |
| Status page `status.passbi.com` | All plans |

---

## See also

- [Authentication](../getting-started/authentication.md) — key creation, error codes
- [Error reference](../api/reference/errors.md) — full error catalog
- [Quickstart](../getting-started/quickstart.md) — first request in 30 seconds
