# Error Reference

All errors return JSON with a single `error` field:

```json
{ "error": "human-readable message" }
```

---

## Status codes

| Code | Meaning | Retryable |
|------|---------|-----------|
| `200` | Success | — |
| `400` | Invalid request parameters | No — fix the request |
| `404` | No results / resource not found | No — check inputs |
| `500` | Internal server error | Yes — exponential backoff |
| `503` | Dependency unavailable (DB/Redis) | Yes — wait |

---

## 400 — Bad Request

### Missing parameters

```json
{ "error": "missing required parameters: from and to" }
{ "error": "missing required parameters: lat and lon" }
```

**Fix:** ensure all required query params are present before sending the request.

---

### Invalid coordinate format

```json
{ "error": "invalid 'from' coordinates: expected format: lat,lon" }
```

Coordinates must be `"lat,lon"` — comma-separated, no spaces, numeric only.

```javascript
// Validate before sending
const coordPattern = /^-?\d+(\.\d+)?,-?\d+(\.\d+)?$/;
if (!coordPattern.test(from)) throw new Error('Invalid coordinate format');
```

---

### Coordinates out of range

```json
{ "error": "invalid 'from' coordinates: latitude must be between -90 and 90" }
{ "error": "invalid 'from' coordinates: longitude must be between -180 and 180" }
```

```javascript
function validateCoords(lat, lon) {
  if (lat < -90 || lat > 90)   throw new Error('Latitude out of range');
  if (lon < -180 || lon > 180) throw new Error('Longitude out of range');
}
```

---

### Invalid radius

```json
{ "error": "invalid radius (must be between 0 and 5000 meters)" }
```

**Valid range:** 0–5 000 m. Default: 500 m.

---

## 404 — Not Found

### No routes found

```json
{ "error": "no routes found between the specified locations" }
```

> [!NOTE]
> This is not a server error — it means no transit path connects the two points.

**Diagnose:**

```bash
# Check if origin has nearby stops
curl "https://passbi-api.onrender.com/v2/stops/nearby?lat=14.7167&lon=-17.4677&radius=1000"

# Check if destination has nearby stops
curl "https://passbi-api.onrender.com/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=1000"
```

If either returns 0 stops, the location is outside the service area.

---

### Invalid endpoint path

Caused by a missing `/v2/` prefix or a typo.

```bash
# Wrong
curl "https://passbi-api.onrender.com/route-search?from=..."

# Correct
curl "https://passbi-api.onrender.com/v2/route-search?from=..."
```

---

## 500 — Internal Server Error

```json
{ "error": "internal server error" }
```

Transient — retry with exponential backoff:

```javascript
async function fetchWithRetry(url, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    const res = await fetch(url);
    if (res.status !== 500 || i === maxRetries - 1) return res;
    await new Promise(r => setTimeout(r, 1000 * Math.pow(2, i)));
  }
}
```

Check health to confirm dependencies are up:

```bash
curl "https://passbi-api.onrender.com/health"
# { "status": "healthy", "checks": { "database": "ok", "redis": "ok" } }
```

---

## 503 — Service Unavailable

```json
{ "status": "unhealthy", "checks": { "database": "connection refused", "redis": "ok" } }
```

> [!WARNING]
> Returned by `/health` when a dependency is down. Do not send business requests until the health check passes.

```javascript
async function waitForHealthy(maxAttempts = 5, intervalMs = 3000) {
  for (let i = 0; i < maxAttempts; i++) {
    const res = await fetch('https://passbi-api.onrender.com/health');
    const { status } = await res.json();
    if (status === 'healthy') return true;
    await new Promise(r => setTimeout(r, intervalMs));
  }
  return false;
}
```

---

## Complete error handler

```javascript
async function searchRoute(from, to) {
  const url = new URL('https://passbi-api.onrender.com/v2/route-search');
  url.searchParams.set('from', from);
  url.searchParams.set('to', to);

  let res;
  try {
    res = await fetch(url);
  } catch {
    throw new Error('NETWORK_ERROR');
  }

  if (res.ok)          return res.json();
  if (res.status === 404) throw new Error('NO_ROUTES_FOUND');
  if (res.status === 400) {
    const { error } = await res.json();
    throw new Error(`INVALID_PARAMS: ${error}`);
  }
  if (res.status === 500) throw new Error('SERVER_ERROR');
  if (res.status === 503) throw new Error('SERVICE_UNAVAILABLE');
  throw new Error(`HTTP_${res.status}`);
}

const USER_MESSAGES = {
  NO_ROUTES_FOUND:     'No routes found. Check that both locations are near transit stops.',
  INVALID_PARAMS:      'Invalid search parameters.',
  SERVER_ERROR:        'Server error — please try again.',
  SERVICE_UNAVAILABLE: 'Service temporarily unavailable.',
  NETWORK_ERROR:       'No internet connection.',
};

function userMessage(err) {
  return USER_MESSAGES[err.message] ?? USER_MESSAGES[err.message?.split(':')[0]] ?? 'Unexpected error.';
}
```

---

## Debugging checklist

1. **Health first** — `GET /health` confirms DB and Redis are up
2. **cURL first** — isolate app bugs from API bugs with a raw request
3. **Coordinate format** — `"lat,lon"` comma-separated, no spaces
4. **Content-Type** — responses are always `application/json`
5. **URL encoding** — use `URLSearchParams` or equivalent; don't hand-build query strings

---

## See also

- [OpenAPI spec](../openapi.yaml)
- [Quickstart](../../getting-started/quickstart.md)
- [Authentication errors](../../getting-started/authentication.md#error-codes)
