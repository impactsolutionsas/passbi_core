# PassBi Core API Integration Guide

Complete guide for integrating the PassBi routing API into your application.

## Table of Contents

1. [Introduction](#introduction)
2. [Prerequisites](#prerequisites)
3. [Getting Started](#getting-started)
4. [Understanding Routing Strategies](#understanding-routing-strategies)
5. [Making API Requests](#making-api-requests)
6. [Error Handling](#error-handling)
7. [Caching Strategies](#caching-strategies)
8. [Performance Optimization](#performance-optimization)
9. [Production Considerations](#production-considerations)
10. [Advanced Topics](#advanced-topics)

---

## Introduction

PassBi Core is a multimodal transit routing engine that provides optimal routes between two locations using multiple strategies. This guide will help you integrate the API into your web, mobile, or backend application.

**What PassBi Provides:**
- **Multi-strategy routing**: 4 different route options optimized for different user preferences
- **Geospatial search**: Find nearby transit stops
- **Route catalog**: Browse available transit routes
- **High performance**: Sub-500ms P95 response times

**Use Cases:**
- Transit apps and journey planners
- Mobility-as-a-Service (MaaS) platforms
- Navigation applications
- Urban planning and analytics tools

---

## Prerequisites

### Technical Requirements
- Basic understanding of RESTful APIs
- HTTP client library for your programming language
- Understanding of JSON data format

### Knowledge Requirements
- Familiarity with geographic coordinates (latitude/longitude)
- Basic understanding of transit concepts (routes, stops, transfers)

### Optional
- Experience with caching strategies
- Understanding of async/await patterns
- Knowledge of error handling best practices

---

## Getting Started

### Base URL

**Development:**
```
http://localhost:8080
```

**Production:**
```
https://api.passbi.com  # Example - replace with your actual URL
```

All API endpoints are prefixed with `/v2/` to indicate the API version.

### Authentication

Currently, no authentication is required. Future versions may introduce API keys or OAuth.

### First Steps

1. **Check API health:**
```bash
curl http://localhost:8080/health
```

Expected response:
```json
{
  "status": "healthy",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

2. **Make your first route search:**
```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

3. **Find nearby stops:**
```bash
curl "http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"
```

See [Quickstart Guide](quickstart-integration.md) for a 5-minute integration tutorial.

---

## Understanding Routing Strategies

PassBi computes **4 different routing strategies** in parallel, each optimized for different user preferences:

### 1. No Transfer Strategy (`no_transfer`)

**Goal:** Absolutely zero transfers - single transit line only

**Best For:**
- Users with heavy luggage
- Elderly passengers
- Those seeking maximum comfort
- When simplicity is more important than speed

**Trade-offs:**
- May take longer than other strategies
- Moderate walking tolerance
- Limited route options

**Use Case Example:**
```javascript
// For a user profile indicating "accessibility needs"
const route = routeData.routes.no_transfer;
if (route) {
  showRoute(route, "Most Comfortable - No Transfers");
}
```

**Cost Function:**
- Transfer cost: 999,999,999 (effectively infinite)
- Walk cost: Time × 5
- Ride cost: Time only

---

### 2. Direct Strategy (`direct`)

**Goal:** Minimize or eliminate transfers (simplicity focused)

**Best For:**
- Users who prefer simplicity
- First-time transit users
- Quick trips where simplicity matters

**Trade-offs:**
- May take longer than faster options
- Heavy penalty on walking distance
- Prioritizes ease of use over speed

**Use Case Example:**
```javascript
// For users marked as "prefer simple routes"
const route = routeData.routes.direct;
if (route) {
  showRoute(route, "Simplest Route");
}
```

**Cost Function:**
- Transfer cost: 999,999 (very high)
- Walk cost: Time × 10 (heavy penalty)
- Ride cost: Time only
- Max transfers: 0

---

### 3. Simple Strategy (`simple`) - **RECOMMENDED DEFAULT**

**Goal:** Balance time, walking distance, and transfers

**Best For:**
- General-purpose routing
- Most users
- Default recommendation
- Balanced approach

**Trade-offs:**
- Moderate in all aspects
- Best overall user experience
- Good compromise between speed and comfort

**Use Case Example:**
```javascript
// Default recommended route
const route = routeData.routes.simple;
if (route) {
  showRoute(route, "Recommended", { badge: "✓ Best Balance" });
}
```

**Cost Function:**
- Cost = Time + (Walk distance × 2) + (Transfers × 180 seconds)
- Max transfers: 2
- Balanced weighting of all factors

---

### 4. Fast Strategy (`fast`)

**Goal:** Minimize total travel time

**Best For:**
- Users in a hurry
- Time-sensitive trips
- Commuters prioritizing speed
- When arriving quickly is critical

**Trade-offs:**
- May require more walking
- May involve more transfers
- Less comfortable but faster

**Use Case Example:**
```javascript
// For "express" or "fastest" mode
const route = routeData.routes.fast;
if (route) {
  showRoute(route, "Fastest Route", { icon: "⚡" });
}
```

**Cost Function:**
- Cost = Time only (ignores walking and transfers)
- Max transfers: 3
- Pure time optimization

---

### Strategy Selection Guide

**Recommendation:** Always fetch all 4 strategies and present them to users with clear labels:

```javascript
function presentRouteOptions(data) {
  const options = [];

  if (data.routes.simple) {
    options.push({
      strategy: 'simple',
      route: data.routes.simple,
      label: 'Recommended',
      badge: '✓ Best Balance',
      description: 'Good balance of time and comfort'
    });
  }

  if (data.routes.fast) {
    options.push({
      strategy: 'fast',
      route: data.routes.fast,
      label: 'Fastest',
      badge: '⚡',
      description: 'Quickest journey time'
    });
  }

  if (data.routes.no_transfer) {
    options.push({
      strategy: 'no_transfer',
      route: data.routes.no_transfer,
      label: 'No Transfers',
      badge: '🛋️',
      description: 'Most comfortable, single line'
    });
  }

  if (data.routes.direct) {
    options.push({
      strategy: 'direct',
      route: data.routes.direct,
      label: 'Direct',
      badge: '➡️',
      description: 'Simplest route'
    });
  }

  return options;
}
```

See [Routing Strategies Deep Dive](routing-strategies.md) for algorithm details.

---

## Making API Requests

### Endpoint: Route Search

**Purpose:** Find optimal routes between two coordinates

**Endpoint:** `GET /v2/route-search`

**Parameters:**
- `from` (required): Origin coordinates as `"lat,lon"`
- `to` (required): Destination coordinates as `"lat,lon"`

**Example Request:**
```javascript
async function searchRoute(fromLat, fromLon, toLat, toLon) {
  const url = new URL('http://localhost:8080/v2/route-search');
  url.searchParams.set('from', `${fromLat},${fromLon}`);
  url.searchParams.set('to', `${toLat},${toLon}`);

  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return await response.json();
}

// Usage
const routes = await searchRoute(14.7167, -17.4677, 14.6928, -17.4467);
```

**Response Structure:**
```json
{
  "routes": {
    "no_transfer": { ... },
    "direct": { ... },
    "simple": { ... },
    "fast": { ... }
  }
}
```

**Processing Steps:**
```javascript
function processRouteResponse(data) {
  const routes = data.routes;

  // Check if any routes were found
  if (Object.keys(routes).length === 0) {
    throw new Error('No routes available');
  }

  // Get recommended route (simple strategy)
  const recommended = routes.simple || routes.direct || routes.fast || routes.no_transfer;

  // Extract journey details
  const durationMinutes = Math.floor(recommended.duration_seconds / 60);
  const walkingMeters = recommended.walk_distance_meters;
  const transferCount = recommended.transfers;

  // Process step-by-step directions
  const directions = recommended.steps.map(step => {
    if (step.type === 'WALK') {
      return {
        type: 'walk',
        instruction: `Walk ${step.distance_meters}m to ${step.to_stop_name}`,
        duration: step.duration_seconds,
        distance: step.distance_meters
      };
    } else if (step.type === 'RIDE') {
      return {
        type: 'ride',
        instruction: `Take ${step.route_name} towards ${step.to_stop_name}`,
        duration: step.duration_seconds,
        route: step.route_name,
        mode: step.mode,
        stops: step.num_stops
      };
    } else if (step.type === 'TRANSFER') {
      return {
        type: 'transfer',
        instruction: `Transfer at ${step.from_stop_name}`,
        duration: step.duration_seconds
      };
    }
  });

  return {
    durationMinutes,
    walkingMeters,
    transferCount,
    directions,
    allStrategies: routes
  };
}
```

---

### Endpoint: Nearby Stops

**Purpose:** Find transit stops within a radius of a location

**Endpoint:** `GET /v2/stops/nearby`

**Parameters:**
- `lat` (required): Latitude (number)
- `lon` (required): Longitude (number)
- `radius` (optional): Search radius in meters (default: 500, max: 5000)

**Example Request:**
```javascript
async function findNearbyStops(lat, lon, radius = 500) {
  const url = new URL('http://localhost:8080/v2/stops/nearby');
  url.searchParams.set('lat', lat.toString());
  url.searchParams.set('lon', lon.toString());
  url.searchParams.set('radius', radius.toString());

  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return await response.json();
}

// Usage
const stops = await findNearbyStops(14.6928, -17.4467, 500);
```

**Response Structure:**
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

**Use Cases:**
- "Where is the nearest bus stop?"
- Suggesting alternative start/end points for routing
- Displaying transit coverage on a map
- Finding stops in a neighborhood

---

### Endpoint: Routes List

**Purpose:** Get a catalog of all available transit routes

**Endpoint:** `GET /v2/routes/list`

**Parameters:**
- `mode` (optional): Filter by mode (BUS, BRT, TER, FERRY, TRAM)
- `agency` (optional): Filter by agency ID
- `limit` (optional): Limit results (default: 100, max: 1000)

**Example Request:**
```javascript
async function listRoutes(filters = {}) {
  const url = new URL('http://localhost:8080/v2/routes/list');

  if (filters.mode) {
    url.searchParams.set('mode', filters.mode);
  }
  if (filters.agency) {
    url.searchParams.set('agency', filters.agency);
  }
  if (filters.limit) {
    url.searchParams.set('limit', filters.limit.toString());
  }

  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return await response.json();
}

// Usage examples
const allRoutes = await listRoutes();
const busRoutes = await listRoutes({ mode: 'BUS', limit: 50 });
const agencyRoutes = await listRoutes({ agency: 'dakar_dem_dikk' });
```

**Use Cases:**
- Browse all available routes
- Filter routes by mode (show only buses, trains, etc.)
- Display route information to users
- Build route selection UI

---

## Error Handling

### Error Response Format

All errors return JSON:
```json
{
  "error": "human-readable error message"
}
```

### HTTP Status Codes

| Status | Meaning | Action |
|--------|---------|--------|
| 200 | Success | Process response |
| 400 | Bad Request | Fix parameters |
| 404 | Not Found | Adjust search or inform user |
| 500 | Server Error | Retry with backoff |
| 503 | Service Unavailable | Wait and retry |

### Comprehensive Error Handling

```javascript
async function makeAPIRequest(url) {
  try {
    const response = await fetch(url, {
      timeout: 15000  // 15 second timeout
    });

    // Success (200)
    if (response.ok) {
      return await response.json();
    }

    // Client errors (400)
    if (response.status === 400) {
      const error = await response.json();
      throw new APIError('INVALID_PARAMS', error.error);
    }

    // Not found (404)
    if (response.status === 404) {
      const error = await response.json();
      throw new APIError('NOT_FOUND', error.error);
    }

    // Server errors (500)
    if (response.status === 500) {
      throw new APIError('SERVER_ERROR', 'Internal server error');
    }

    // Service unavailable (503)
    if (response.status === 503) {
      throw new APIError('SERVICE_UNAVAILABLE', 'Service temporarily unavailable');
    }

    // Other errors
    throw new APIError('UNKNOWN', `HTTP ${response.status}`);

  } catch (error) {
    // Network errors
    if (error.name === 'AbortError') {
      throw new APIError('TIMEOUT', 'Request timeout');
    }
    if (error.message === 'Failed to fetch') {
      throw new APIError('NETWORK_ERROR', 'Network connection failed');
    }

    // Re-throw API errors
    if (error instanceof APIError) {
      throw error;
    }

    // Unknown errors
    throw new APIError('UNKNOWN', error.message);
  }
}

class APIError extends Error {
  constructor(code, message) {
    super(message);
    this.code = code;
    this.name = 'APIError';
  }
}

// Usage
try {
  const routes = await makeAPIRequest(url);
  displayRoutes(routes);
} catch (error) {
  if (error instanceof APIError) {
    switch (error.code) {
      case 'NOT_FOUND':
        showMessage('No routes found. Try different locations.');
        break;
      case 'INVALID_PARAMS':
        showMessage('Invalid search parameters. Please check your input.');
        break;
      case 'NETWORK_ERROR':
        showMessage('Network error. Please check your connection.');
        break;
      case 'SERVER_ERROR':
      case 'SERVICE_UNAVAILABLE':
        showMessage('Service temporarily unavailable. Please try again.');
        break;
      default:
        showMessage('An unexpected error occurred.');
    }
  }
}
```

See [Error Reference](../api/reference/errors.md) for complete error catalog.

---

## Caching Strategies

### Why Cache?

- **Performance**: Faster response times for repeated searches
- **Cost**: Reduce server load and API calls
- **UX**: Better user experience with instant results

### Recommended Cache TTL

| Endpoint | TTL | Reason |
|----------|-----|--------|
| `/v2/route-search` | 10 minutes | Routes are static (no real-time yet) |
| `/v2/stops/nearby` | 1 hour | Stop locations rarely change |
| `/v2/routes/list` | 24 hours | Route list is very stable |

### Client-Side Caching

```javascript
class PassBiCache {
  constructor() {
    this.cache = new Map();
    this.ttls = {
      'route-search': 10 * 60 * 1000,      // 10 minutes
      'stops-nearby': 60 * 60 * 1000,      // 1 hour
      'routes-list': 24 * 60 * 60 * 1000   // 24 hours
    };
  }

  generateKey(endpoint, params) {
    const sortedParams = Object.keys(params)
      .sort()
      .map(k => `${k}=${params[k]}`)
      .join('&');
    return `${endpoint}?${sortedParams}`;
  }

  get(endpoint, params) {
    const key = this.generateKey(endpoint, params);
    const cached = this.cache.get(key);

    if (!cached) {
      return null;
    }

    const ttl = this.ttls[endpoint] || 600000; // Default 10 min
    const age = Date.now() - cached.timestamp;

    if (age > ttl) {
      this.cache.delete(key);
      return null;
    }

    return cached.data;
  }

  set(endpoint, params, data) {
    const key = this.generateKey(endpoint, params);
    this.cache.set(key, {
      data,
      timestamp: Date.now()
    });
  }

  clear() {
    this.cache.clear();
  }

  clearEndpoint(endpoint) {
    for (const [key, _] of this.cache) {
      if (key.startsWith(endpoint)) {
        this.cache.delete(key);
      }
    }
  }
}

// Usage
const cache = new PassBiCache();

async function searchRouteWithCache(fromLat, fromLon, toLat, toLon) {
  const params = {
    from: `${fromLat},${fromLon}`,
    to: `${toLat},${toLon}`
  };

  // Check cache
  const cached = cache.get('route-search', params);
  if (cached) {
    console.log('Cache hit!');
    return cached;
  }

  // Fetch from API
  console.log('Cache miss - fetching from API');
  const data = await searchRoute(fromLat, fromLon, toLat, toLon);

  // Store in cache
  cache.set('route-search', params, data);

  return data;
}
```

### Cache Invalidation

Invalidate cache when:
- User manually refreshes
- App is restarted
- User changes significant preferences
- Known data updates occur

```javascript
// Manual refresh
function handleRefresh() {
  cache.clear();
  reloadData();
}

// Preference change
function handlePreferenceChange() {
  cache.clearEndpoint('route-search');
  recalculateRoutes();
}
```

---

## Performance Optimization

### 1. Debounce User Input

Don't make requests on every keystroke:

```javascript
import { debounce } from 'lodash';

const searchRouteDebounced = debounce(async (from, to) => {
  const routes = await searchRoute(from.lat, from.lon, to.lat, to.lon);
  displayRoutes(routes);
}, 500); // Wait 500ms after last keystroke

// Usage
searchInput.addEventListener('input', () => {
  searchRouteDebounced(origin, destination);
});
```

### 2. Cancel Outdated Requests

```javascript
let currentRequest = null;

async function searchWithCancellation(fromLat, fromLon, toLat, toLon) {
  // Cancel previous request
  if (currentRequest) {
    currentRequest.abort();
  }

  // Create new abort controller
  const controller = new AbortController();
  currentRequest = controller;

  try {
    const url = new URL('http://localhost:8080/v2/route-search');
    url.searchParams.set('from', `${fromLat},${fromLon}`);
    url.searchParams.set('to', `${toLat},${toLon}`);

    const response = await fetch(url, {
      signal: controller.signal
    });

    return await response.json();

  } catch (error) {
    if (error.name === 'AbortError') {
      console.log('Request cancelled');
      return null;
    }
    throw error;
  } finally {
    if (currentRequest === controller) {
      currentRequest = null;
    }
  }
}
```

### 3. Parallel Requests

Fetch multiple independent resources simultaneously:

```javascript
async function loadDashboard(userLat, userLon) {
  // Fetch nearby stops and routes list in parallel
  const [nearbyStops, routesList] = await Promise.all([
    findNearbyStops(userLat, userLon, 500),
    listRoutes({ limit: 20 })
  ]);

  displayDashboard(nearbyStops, routesList);
}
```

### 4. Progressive Enhancement

Show partial results while loading:

```javascript
async function searchWithProgress(fromLat, fromLon, toLat, toLon) {
  // Show loading state
  showLoadingIndicator();

  try {
    // Fetch routes
    const routes = await searchRoute(fromLat, fromLon, toLat, toLon);

    // Show first result immediately
    if (routes.routes.simple) {
      displayRoute(routes.routes.simple, 'Recommended');
    }

    // Show other results
    setTimeout(() => {
      displayAllRoutes(routes);
    }, 100);

  } finally {
    hideLoadingIndicator();
  }
}
```

---

## Production Considerations

### 1. Use HTTPS

Always use HTTPS in production to protect user location data:

```javascript
const BASE_URL = process.env.NODE_ENV === 'production'
  ? 'https://api.passbi.com'
  : 'http://localhost:8080';
```

### 2. Handle Offline Mode

Implement graceful degradation:

```javascript
window.addEventListener('online', () => {
  console.log('Connection restored');
  retryFailedRequests();
});

window.addEventListener('offline', () => {
  console.log('Connection lost');
  showOfflineMessage();
  useCachedData();
});

function useCachedData() {
  // Use cached routes if available
  const lastSearch = cache.get('route-search', lastParams);
  if (lastSearch) {
    displayRoutes(lastSearch, { cached: true });
    showMessage('Showing cached results (offline)');
  }
}
```

### 3. Monitor API Health

Periodically check health endpoint:

```javascript
setInterval(async () => {
  try {
    const health = await fetch('http://localhost:8080/health');
    const status = await health.json();

    if (status.status !== 'healthy') {
      console.warn('API unhealthy:', status.checks);
      showWarningBanner('Service may be experiencing issues');
    }
  } catch (error) {
    console.error('Health check failed:', error);
  }
}, 5 * 60 * 1000); // Every 5 minutes
```

### 4. Respect User Privacy

- Don't store location data longer than necessary
- Anonymize logs
- Provide clear privacy policy
- Allow users to clear cached data

```javascript
function clearUserData() {
  cache.clear();
  localStorage.removeItem('recent_searches');
  console.log('User data cleared');
}
```

### 5. Error Logging

Log errors for debugging:

```javascript
async function logError(error, context) {
  const errorData = {
    message: error.message,
    code: error.code,
    context,
    timestamp: new Date().toISOString(),
    userAgent: navigator.userAgent
  };

  // Send to your logging service
  try {
    await fetch('/api/log-error', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(errorData)
    });
  } catch (e) {
    console.error('Failed to log error:', e);
  }
}
```

---

## Advanced Topics

### Multi-Language Support

Provide translations for UI elements:

```javascript
const i18n = {
  en: {
    noRoutes: 'No routes found',
    fastest: 'Fastest',
    recommended: 'Recommended'
  },
  fr: {
    noRoutes: 'Aucun itinéraire trouvé',
    fastest: 'Le plus rapide',
    recommended: 'Recommandé'
  }
};

function translate(key, lang = 'en') {
  return i18n[lang]?.[key] || i18n.en[key];
}
```

### Route Comparison

Help users compare different strategies:

```javascript
function compareStrategies(routes) {
  const comparison = Object.entries(routes).map(([strategy, route]) => ({
    strategy,
    duration: route.duration_seconds,
    walking: route.walk_distance_meters,
    transfers: route.transfers,
    score: calculateScore(route) // Your custom scoring logic
  }));

  return comparison.sort((a, b) => b.score - a.score);
}
```

### Accessibility

Ensure your integration is accessible:

```javascript
function announceRoute(route) {
  const announcement = `Route found: ${route.duration_seconds / 60} minutes,
    ${route.transfers} transfers, ${route.walk_distance_meters} meters walking`;

  // Use ARIA live region
  const liveRegion = document.getElementById('aria-live');
  liveRegion.textContent = announcement;
}
```

---

## Next Steps

- **[Quickstart Guide](quickstart-integration.md)** - 5-minute integration tutorial
- **[Routing Strategies Deep Dive](routing-strategies.md)** - Algorithm details
- **[Code Examples](../api/examples/)** - Complete examples in multiple languages
- **[Error Reference](../api/reference/errors.md)** - Complete error catalog
- **[Data Models Reference](../api/reference/data-models.md)** - Full data structures
- **[OpenAPI Specification](../api/openapi.yaml)** - Machine-readable API spec

---

## Support

For questions or issues:
- Check the [Error Reference](../api/reference/errors.md)
- Review [Code Examples](../api/examples/)
- Open an issue on GitHub

**Happy integrating! 🚌🚶⚡**
