# Authentication

PassBi uses API keys for partner access. Public endpoints currently work without authentication.

---

## Public access (current)

All `/v2/` endpoints are accessible without a key during the beta period. No `Authorization` header needed.

```bash
curl "https://passbi-api.onrender.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

> [!NOTE]
> Authentication will be enforced when the partner portal launches. Build your integration now without keys — migration will be additive (one header to add).

---

## Partner API keys

Partners receive keys in the format `pk_live_...` (production) or `pk_test_...` (sandbox).

### Send the key

All authenticated requests use the `Authorization` header:

```bash
curl "https://passbi-api.onrender.com/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" \
  -H "Authorization: Bearer pk_live_YOUR_KEY"
```

```javascript
const res = await fetch(url, {
  headers: { 'Authorization': `Bearer ${process.env.PASSBI_API_KEY}` }
});
```

```python
headers = {'Authorization': f'Bearer {os.environ["PASSBI_API_KEY"]}'}
res = requests.get(url, headers=headers)
```

---

## Store keys securely

> [!WARNING]
> Never expose API keys in frontend code or commit them to Git.

**Backend (Node.js)**
```bash
# .env
PASSBI_API_KEY=pk_live_abc123...
```
```javascript
import 'dotenv/config';
const client = new PassBiClient(process.env.PASSBI_API_KEY);
```

**Frontend → proxy via your backend**
```javascript
// Your backend endpoint
app.get('/api/routes', async (req, res) => {
  const data = await passBiClient.searchRoutes(req.query);
  res.json(data);
});

// Frontend calls your backend, never PassBi directly
const data = await fetch('/api/routes?from=...&to=...').then(r => r.json());
```

---

## Error codes

| Code | Error | Cause |
|------|-------|-------|
| `401` | `invalid_api_key` | Key is missing, malformed, or revoked |
| `403` | `insufficient_permissions` | Key doesn't have the required scope |
| `429` | `rate_limit_exceeded` | Too many requests per second |
| `429` | `daily_quota_exceeded` | Daily request limit reached |

---

## Rate limits by plan

| Plan | req/s | req/day | req/month |
|------|-------|---------|-----------|
| Free | 2 | 1 000 | 30 000 |
| Starter | 10 | 10 000 | 300 000 |
| Business | 50 | 50 000 | 1 500 000 |
| Enterprise | 1 000 | unlimited | unlimited |

### Retry on 429

```javascript
async function fetchWithRetry(url, options, retries = 3) {
  for (let i = 0; i < retries; i++) {
    const res = await fetch(url, options);
    if (res.status !== 429) return res;

    const retryAfter = parseInt(res.headers.get('Retry-After') ?? '1', 10);
    await new Promise(r => setTimeout(r, retryAfter * 1000));
  }
  throw new Error('Rate limit — max retries exceeded');
}
```

---

## Next steps

- [Partner integration guide](../guides/partner-integration.md) — quotas, security, production checklist
- [Error reference](../api/reference/errors.md) — full error catalog
