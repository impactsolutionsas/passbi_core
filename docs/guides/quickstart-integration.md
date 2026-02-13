# 5-Minute Integration Quickstart

Get PassBi routing into your app in 5 minutes.

## Overview

This guide will walk you through the absolute basics to get you up and running with the PassBi API.

**What you'll learn:**
1. Making your first API request (30 seconds)
2. Understanding the response (1 minute)
3. Displaying routes in your app (2 minutes)
4. Adding basic error handling (1 minute)
5. Implementing simple caching (30 seconds)

**Prerequisites:**
- Basic understanding of HTTP APIs
- An HTTP client (fetch, axios, requests, etc.)

---

## Step 1: Make Your First Request (30 seconds)

### Test with cURL

The simplest way to verify the API works:

```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

### JavaScript / Browser

```javascript
const response = await fetch(
  'http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467'
);
const data = await response.json();
console.log(data);
```

### Python

```python
import requests

response = requests.get(
    'http://localhost:8080/v2/route-search',
    params={
        'from': '14.7167,-17.4677',
        'to': '14.6928,-17.4467'
    }
)
data = response.json()
print(data)
```

**That's it!** You've made your first PassBi API call.

---

## Step 2: Understand the Response (1 minute)

You get **4 route options** optimized for different user preferences:

```json
{
  "routes": {
    "no_transfer": { ... },    // Zero transfers (single line only)
    "direct": { ... },         // Minimal transfers (simplicity)
    "simple": { ... },         // Balanced (recommended default)
    "fast": { ... }            // Fastest time (may need more transfers)
  }
}
```

Each route contains:

```json
{
  "duration_seconds": 1200,           // Total trip time (20 minutes)
  "walk_distance_meters": 350,        // How far to walk (350 meters)
  "transfers": 1,                     // Number of transfers
  "steps": [...]                      // Step-by-step directions
}
```

**Pro tip:** Start with the `simple` strategy - it's the recommended default for most users.

---

## Step 3: Display Routes (2 minutes)

### Basic Display

```javascript
async function displayRoute(fromLat, fromLon, toLat, toLon) {
  // 1. Fetch routes
  const response = await fetch(
    `http://localhost:8080/v2/route-search?from=${fromLat},${fromLon}&to=${toLat},${toLon}`
  );
  const data = await response.json();

  // 2. Get the recommended route (simple strategy)
  const recommended = data.routes.simple;

  if (!recommended) {
    console.log('No routes found');
    return;
  }

  // 3. Display summary
  const durationMins = Math.floor(recommended.duration_seconds / 60);
  console.log(`
    ⏱️  Duration: ${durationMins} minutes
    🚶 Walking: ${recommended.walk_distance_meters}m
    🔄 Transfers: ${recommended.transfers}
  `);

  // 4. Display step-by-step directions
  recommended.steps.forEach(step => {
    if (step.type === 'WALK') {
      console.log(`🚶 Walk ${step.distance_meters}m to ${step.to_stop_name}`);
    } else if (step.type === 'RIDE') {
      console.log(`🚌 Take ${step.route_name} (${step.num_stops} stops)`);
    } else if (step.type === 'TRANSFER') {
      console.log(`🔄 Transfer at ${step.from_stop_name}`);
    }
  });
}

// Usage
displayRoute(14.7167, -17.4677, 14.6928, -17.4467);
```

**Output:**
```
⏱️  Duration: 20 minutes
🚶 Walking: 350m
🔄 Transfers: 1

🚶 Walk 200m to Gare Routière
🚌 Take D1LP (12 stops)
🔄 Transfer at Place de l'Indépendance
🚌 Take D5TH (8 stops)
🚶 Walk 150m to Destination
```

### Show All 4 Options

Let users choose their preferred strategy:

```javascript
function displayAllOptions(data) {
  const routes = data.routes;

  // Display each available strategy
  if (routes.no_transfer) {
    console.log('🛋️  NO TRANSFERS:', formatRoute(routes.no_transfer));
  }
  if (routes.direct) {
    console.log('➡️  DIRECT:', formatRoute(routes.direct));
  }
  if (routes.simple) {
    console.log('✅ RECOMMENDED:', formatRoute(routes.simple));
  }
  if (routes.fast) {
    console.log('⚡ FASTEST:', formatRoute(routes.fast));
  }
}

function formatRoute(route) {
  const mins = Math.floor(route.duration_seconds / 60);
  return `${mins} min, ${route.transfers} transfers, ${route.walk_distance_meters}m walking`;
}
```

---

## Step 4: Add Error Handling (1 minute)

Handle the most common errors:

```javascript
async function searchRoute(fromLat, fromLon, toLat, toLon) {
  const url = new URL('http://localhost:8080/v2/route-search');
  url.searchParams.set('from', `${fromLat},${fromLon}`);
  url.searchParams.set('to', `${toLat},${toLon}`);

  try {
    const response = await fetch(url);

    // No routes found (404)
    if (response.status === 404) {
      alert('No routes found. Try different locations or check if they\'re within the service area.');
      return null;
    }

    // Invalid parameters (400)
    if (response.status === 400) {
      const error = await response.json();
      alert(`Invalid parameters: ${error.error}`);
      return null;
    }

    // Server error (500)
    if (response.status === 500) {
      alert('Server error. Please try again in a moment.');
      return null;
    }

    // Success (200)
    if (response.ok) {
      return await response.json();
    }

    // Unknown error
    alert('An unexpected error occurred.');
    return null;

  } catch (error) {
    // Network error
    alert('Network error. Please check your internet connection.');
    console.error(error);
    return null;
  }
}

// Usage with error handling
const routes = await searchRoute(14.7167, -17.4677, 14.6928, -17.4467);
if (routes) {
  displayRoute(routes);
}
```

---

## Step 5: Implement Caching (30 seconds)

Routes are static (no real-time updates yet), so cache results for 10 minutes:

```javascript
const routeCache = new Map();
const CACHE_TTL = 10 * 60 * 1000; // 10 minutes

function getCacheKey(fromLat, fromLon, toLat, toLon) {
  return `${fromLat},${fromLon}|${toLat},${toLon}`;
}

async function searchRouteWithCache(fromLat, fromLon, toLat, toLon) {
  // Check cache
  const key = getCacheKey(fromLat, fromLon, toLat, toLon);
  const cached = routeCache.get(key);

  if (cached && Date.now() - cached.timestamp < CACHE_TTL) {
    console.log('🎯 Cache hit!');
    return cached.data;
  }

  // Fetch from API
  console.log('🌐 Fetching from API...');
  const data = await searchRoute(fromLat, fromLon, toLat, toLon);

  // Store in cache
  if (data) {
    routeCache.set(key, {
      data,
      timestamp: Date.now()
    });
  }

  return data;
}

// Usage
const routes = await searchRouteWithCache(14.7167, -17.4677, 14.6928, -17.4467);
```

**Benefits:**
- Faster response for repeated searches
- Reduces server load
- Better user experience

---

## Complete Example

Putting it all together:

```javascript
// Simple PassBi client with caching and error handling
class PassBiClient {
  constructor(baseURL = 'http://localhost:8080') {
    this.baseURL = baseURL;
    this.cache = new Map();
    this.cacheTTL = 10 * 60 * 1000; // 10 minutes
  }

  async searchRoute(fromLat, fromLon, toLat, toLon) {
    // Check cache
    const cacheKey = `${fromLat},${fromLon}|${toLat},${toLon}`;
    const cached = this.cache.get(cacheKey);

    if (cached && Date.now() - cached.timestamp < this.cacheTTL) {
      return cached.data;
    }

    // Build URL
    const url = new URL(`${this.baseURL}/v2/route-search`);
    url.searchParams.set('from', `${fromLat},${fromLon}`);
    url.searchParams.set('to', `${toLat},${toLon}`);

    try {
      const response = await fetch(url);

      if (response.status === 404) {
        throw new Error('NO_ROUTES_FOUND');
      }

      if (response.status === 400) {
        const error = await response.json();
        throw new Error(`INVALID_PARAMS: ${error.error}`);
      }

      if (!response.ok) {
        throw new Error(`HTTP_${response.status}`);
      }

      const data = await response.json();

      // Cache result
      this.cache.set(cacheKey, {
        data,
        timestamp: Date.now()
      });

      return data;

    } catch (error) {
      if (error.message === 'Failed to fetch') {
        throw new Error('NETWORK_ERROR');
      }
      throw error;
    }
  }

  getRecommendedRoute(data) {
    // Return the simple strategy (recommended default)
    return data.routes.simple || data.routes.direct || data.routes.fast || data.routes.no_transfer;
  }

  formatSteps(steps) {
    return steps.map(step => {
      if (step.type === 'WALK') {
        return `🚶 Walk ${step.distance_meters}m to ${step.to_stop_name}`;
      } else if (step.type === 'RIDE') {
        return `🚌 Take ${step.route_name} (${step.num_stops} stops)`;
      } else if (step.type === 'TRANSFER') {
        return `🔄 Transfer at ${step.from_stop_name}`;
      }
    }).join('\n');
  }
}

// Usage
async function main() {
  const client = new PassBiClient();

  try {
    const routes = await client.searchRoute(14.7167, -17.4677, 14.6928, -17.4467);
    const recommended = client.getRecommendedRoute(routes);

    if (recommended) {
      const durationMins = Math.floor(recommended.duration_seconds / 60);

      console.log(`
⏱️  Duration: ${durationMins} minutes
🚶 Walking: ${recommended.walk_distance_meters}m
🔄 Transfers: ${recommended.transfers}

${client.formatSteps(recommended.steps)}
      `);
    }
  } catch (error) {
    if (error.message === 'NO_ROUTES_FOUND') {
      console.error('❌ No routes found between these locations.');
    } else if (error.message.startsWith('INVALID_PARAMS')) {
      console.error('❌ Invalid parameters:', error.message);
    } else if (error.message === 'NETWORK_ERROR') {
      console.error('❌ Network error. Please check your connection.');
    } else {
      console.error('❌ Unexpected error:', error.message);
    }
  }
}

main();
```

---

## Bonus: Find Nearby Stops

Discover transit stops near a location:

```javascript
async function findNearbyStops(lat, lon, radius = 500) {
  const url = new URL('http://localhost:8080/v2/stops/nearby');
  url.searchParams.set('lat', lat);
  url.searchParams.set('lon', lon);
  url.searchParams.set('radius', radius);

  const response = await fetch(url);
  const data = await response.json();

  data.stops.forEach(stop => {
    console.log(`
📍 ${stop.name}
   ${stop.distance_meters}m away
   ${stop.routes_count} routes: ${stop.routes.join(', ')}
    `);
  });
}

// Usage
findNearbyStops(14.6928, -17.4467, 500);
```

---

## Next Steps

Congratulations! You've integrated PassBi in 5 minutes.

### Learn More:

- **[Integration Guide](integration-guide.md)** - Complete integration guide with best practices
- **[Data Models Reference](../api/reference/data-models.md)** - Full data structure documentation
- **[Error Reference](../api/reference/errors.md)** - Complete error handling guide
- **[Code Examples](../api/examples/)** - More examples in JavaScript, Python, cURL, etc.
- **[Routing Strategies](routing-strategies.md)** - Deep dive into the 4 routing algorithms

### Additional Features:

1. **Routing Strategies** - Learn when to use each of the 4 strategies
2. **Performance Optimization** - Debouncing, request cancellation, advanced caching
3. **Production Considerations** - HTTPS, rate limiting, monitoring
4. **Client Libraries** - TypeScript types, Python SDK, etc.

---

## Common Questions

**Q: Which strategy should I use?**
A: Use `simple` as the default. It's a balanced approach that works for most users. You can offer all 4 options and let users choose their preference.

**Q: How often do routes update?**
A: Routes are currently static (based on GTFS data). Cache for 10-15 minutes is safe. Real-time updates are coming in a future version.

**Q: What if I get a 404 "no routes found"?**
A: The locations may be too far apart or not served by transit. Use the `/v2/stops/nearby` endpoint to check if there are stops near your origin and destination.

**Q: Do I need an API key?**
A: Not currently. Authentication may be added in the future.

**Q: What's the rate limit?**
A: No rate limiting is currently enforced, but please be respectful and implement client-side caching.

---

## Quick Reference

### Endpoints

| Endpoint | Method | Purpose |
|----------|--------|---------|
| `/health` | GET | Check API health |
| `/v2/route-search` | GET | Find routes between two points (with ETAs) |
| `/v2/stops/nearby` | GET | Find stops near a location |
| `/v2/stops/search` | GET | Search stops by name |
| `/v2/routes/list` | GET | List all available routes |
| `/v2/stops/:id/departures` | GET | Upcoming departures at a stop |
| `/v2/routes/:id/schedule` | GET | Route timetable |
| `/v2/routes/:id/trips` | GET | Route trip details |

### Parameters

**Route Search:**
- `from`: Origin coordinates as `"lat,lon"`
- `to`: Destination coordinates as `"lat,lon"`
- `time`: Departure time as `"HH:MM"` (optional, default: current UTC time)

**Nearby Stops:**
- `lat`: Latitude (number)
- `lon`: Longitude (number)
- `radius`: Search radius in meters (optional, default: 500, max: 5000)

### Error Codes

- **400**: Invalid parameters
- **404**: No routes found
- **500**: Server error (retry)
- **503**: Service unavailable

---

**Happy routing! 🚌🚶⚡**
