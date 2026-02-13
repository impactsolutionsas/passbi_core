# PassBi Core API Documentation

Welcome to the PassBi Core API documentation! This guide will help you integrate multimodal transit routing into your application.

## 🚀 Quick Start

**New to PassBi?** Start here:

1. **[5-Minute Quickstart](guides/quickstart-integration.md)** - Get up and running in 5 minutes
2. **[Integration Guide](guides/integration-guide.md)** - Complete integration tutorial
3. **[Code Examples](api/examples/)** - Copy-paste examples in your language

## 📚 Documentation Structure

### Getting Started

- **[Quickstart Integration](guides/quickstart-integration.md)** - 5-minute integration tutorial
- **[Integration Guide](guides/integration-guide.md)** - Complete guide with best practices
- **[Routing Strategies](guides/routing-strategies.md)** - Deep dive into the 4 routing algorithms

### API Reference

- **[OpenAPI Specification](api/openapi.yaml)** - Machine-readable API spec
- **[Data Models](api/reference/data-models.md)** - Complete data structure reference
- **[Error Reference](api/reference/errors.md)** - Error codes and troubleshooting

### Code Examples

- **[JavaScript Examples](api/examples/javascript.md)** - Browser, Node.js, React hooks
- **[Python Examples](api/examples/python.md)** - Sync and async examples
- **[cURL Examples](api/examples/curl.md)** - Command-line testing

### Additional Resources

- **[JSON Schemas](schemas/json-schema/)** - Validation schemas
- **[TypeScript Types](schemas/typescript/types.ts)** - Type definitions
- **[Postman Collection](api/examples/postman-collection.json)** - Interactive API testing

---

## 🎯 Key Features

### Multi-Strategy Routing

PassBi returns **4 different route options**, each optimized for different user preferences:

| Strategy | Goal | Best For |
|----------|------|----------|
| **no_transfer** | Zero transfers (single line) | Maximum comfort, luggage, elderly |
| **direct** | Minimal transfers | Simplicity, first-time users |
| **simple** | Balanced (recommended) | General use, default option |
| **fast** | Fastest time | Commuters, time-sensitive trips |

See [Routing Strategies Guide](guides/routing-strategies.md) for details.

### Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Check API health |
| `GET /v2/route-search` | Find routes between two points (with ETAs) |
| `GET /v2/stops/nearby` | Find stops near a location |
| `GET /v2/stops/search` | Search stops by name |
| `GET /v2/routes/list` | List all available routes |
| `GET /v2/stops/:id/departures` | Upcoming departures at a stop |
| `GET /v2/routes/:id/schedule` | Route timetable |
| `GET /v2/routes/:id/trips` | Route trip details |

---

## 📖 Common Use Cases

### Find Routes Between Two Points

```javascript
const response = await fetch(
  'http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467'
);
const data = await response.json();

// Get recommended route (simple strategy)
const recommended = data.routes.simple;
console.log(`Duration: ${recommended.duration_seconds / 60} minutes`);
console.log(`Transfers: ${recommended.transfers}`);
```

[Full Example →](api/examples/javascript.md#basic-route-search)

### Find Nearby Stops

```javascript
const response = await fetch(
  'http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500'
);
const data = await response.json();

data.stops.forEach(stop => {
  console.log(`${stop.name} - ${stop.distance_meters}m away`);
});
```

[Full Example →](api/examples/javascript.md#nearby-stops)

### List Available Routes

```bash
curl "http://localhost:8080/v2/routes/list?mode=BUS&limit=10"
```

[Full Example →](api/examples/curl.md#list-routes)

---

## 🔧 Integration Checklist

- [ ] Read the [Quickstart Guide](guides/quickstart-integration.md)
- [ ] Test API with health check: `curl http://localhost:8080/health`
- [ ] Make your first route search
- [ ] Understand the [4 routing strategies](guides/routing-strategies.md)
- [ ] Implement [error handling](api/reference/errors.md)
- [ ] Add [client-side caching](guides/integration-guide.md#caching-strategies)
- [ ] Review [production considerations](guides/integration-guide.md#production-considerations)

---

## 🎓 Learn More

### For Developers

- **[Integration Guide](guides/integration-guide.md)** - Complete integration walkthrough
- **[Error Handling](api/reference/errors.md)** - Handle errors gracefully
- **[Performance Tips](guides/integration-guide.md#performance-optimization)** - Optimize your integration

### For Technical Leads

- **[Routing Strategies](guides/routing-strategies.md)** - Algorithm details and trade-offs
- **[OpenAPI Spec](api/openapi.yaml)** - Generate client SDKs
- **[Data Models](api/reference/data-models.md)** - Understand the data contracts

### For Architects

- **[System Overview](../README.md#architecture)** - PassBi architecture
- **[Performance Characteristics](guides/routing-strategies.md#performance-characteristics)** - Response times, caching
- **[Production Guide](guides/integration-guide.md#production-considerations)** - HTTPS, monitoring, privacy

---

## 🌐 API Endpoints Summary

### Base URL

**Development:** `http://localhost:8080`

**Production:** `https://passbi-api.onrender.com`

### Quick Reference

#### Health Check
```bash
GET /health
```

Returns API health status and dependency checks.

#### Route Search
```bash
GET /v2/route-search?from=LAT,LON&to=LAT,LON&time=HH:MM
```

Find routes between two coordinates. Returns up to 4 strategies with ETAs.

#### Nearby Stops
```bash
GET /v2/stops/nearby?lat=LAT&lon=LON&radius=METERS
```

Find transit stops within radius (max 20 results).

#### Stop Search
```bash
GET /v2/stops/search?q=QUERY&limit=10
```

Search stops by name (case-insensitive, relevance-ranked).

#### Routes List
```bash
GET /v2/routes/list?mode=MODE&limit=LIMIT
```

List available transit routes with optional filtering.

#### Stop Departures
```bash
GET /v2/stops/:id/departures?time=HH:MM&date=YYYY-MM-DD
```

Upcoming departures at a stop (2-hour window).

#### Route Schedule
```bash
GET /v2/routes/:id/schedule?direction=0
```

Full timetable for a route.

#### Route Trips
```bash
GET /v2/routes/:id/trips?direction=0&limit=20
```

Individual trip details with stop-by-stop times.

---

## 💡 Best Practices

### 1. Always Use the `simple` Strategy as Default

```javascript
const recommended = data.routes.simple || data.routes.direct || data.routes.fast;
```

### 2. Show All 4 Options

Let users choose their preferred strategy:

```javascript
if (data.routes.simple) showOption('Recommended', data.routes.simple);
if (data.routes.fast) showOption('Fastest', data.routes.fast);
if (data.routes.no_transfer) showOption('No Transfers', data.routes.no_transfer);
if (data.routes.direct) showOption('Direct', data.routes.direct);
```

### 3. Implement Caching

```javascript
// Cache for 10 minutes
const CACHE_TTL = 10 * 60 * 1000;
```

### 4. Handle Errors Gracefully

```javascript
if (response.status === 404) {
  showMessage('No routes found. Try different locations.');
}
```

### 5. Validate Input

```javascript
if (lat < -90 || lat > 90 || lon < -180 || lon > 180) {
  throw new Error('Invalid coordinates');
}
```

See [Integration Guide](guides/integration-guide.md#error-handling) for complete error handling.

---

## 📦 SDK & Tools

### TypeScript Types

```typescript
import { RouteSearchResponse, RouteResult, Step } from './docs/schemas/typescript/types';

const routes: RouteSearchResponse = await searchRoute(from, to);
```

[View Types →](schemas/typescript/types.ts)

### Postman Collection

Import the [Postman collection](api/examples/postman-collection.json) for interactive API testing.

### JSON Schemas

Validate responses using [JSON schemas](schemas/json-schema/).

---

## 🆘 Support & Resources

### Documentation

- **[Quickstart](guides/quickstart-integration.md)** - Get started quickly
- **[Integration Guide](guides/integration-guide.md)** - Complete tutorial
- **[Error Reference](api/reference/errors.md)** - Troubleshooting
- **[Code Examples](api/examples/)** - Copy-paste examples

### Common Questions

**Q: Which routing strategy should I use?**
A: Use `simple` as the default. Show all 4 options and let users choose. See [Routing Strategies](guides/routing-strategies.md).

**Q: How do I handle "no routes found" errors?**
A: Check if locations are near transit stops using `/v2/stops/nearby`. See [Error Reference](api/reference/errors.md#no-routes-found).

**Q: How often should I cache results?**
A: Cache route searches for 10 minutes, nearby stops for 1 hour. See [Caching Guide](guides/integration-guide.md#caching-strategies).

**Q: Do I need an API key?**
A: Not currently. Authentication may be added in future versions.

### Project Links

- **[Main README](../README.md)** - Project overview
- **[Quickstart](../QUICKSTART.md)** - Local setup guide
- **[GitHub Issues](https://github.com/passbi/passbi_core/issues)** - Report bugs

---

## 📄 License

PassBi Core is released under the MIT License. See [LICENSE](../LICENSE) for details.

---

## 🎉 Ready to Build?

Start with the [5-Minute Quickstart](guides/quickstart-integration.md) or dive into the [Integration Guide](guides/integration-guide.md).

**Happy routing! 🚌🚶⚡**
