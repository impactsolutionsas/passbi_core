# JavaScript / Node.js Examples

Complete examples for integrating PassBi API in JavaScript applications (browser, Node.js, React).

## Table of Contents

- [Installation](#installation)
- [Basic Route Search](#basic-route-search)
- [Error Handling](#error-handling)
- [React Hooks](#react-hooks)
- [Nearby Stops](#nearby-stops)
- [Routes List](#routes-list)
- [Complete Client Library](#complete-client-library)
- [Advanced Examples](#advanced-examples)

---

## Installation

### Browser (No Installation Required)

Use native `fetch` API (available in all modern browsers):

```html
<script>
  // Your code here - no installation needed
</script>
```

### Node.js < 18

Install `node-fetch`:

```bash
npm install node-fetch
```

```javascript
import fetch from 'node-fetch';
```

### Node.js ≥ 18

Use built-in `fetch` (no installation needed):

```javascript
// fetch is globally available
```

---

## Basic Route Search

### Simple Example

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
console.log('Found routes:', Object.keys(routes.routes));

// Get recommended route (simple strategy)
const recommended = routes.routes.simple;
console.log(`Duration: ${Math.floor(recommended.duration_seconds / 60)} minutes`);
console.log(`Walking: ${recommended.walk_distance_meters}m`);
console.log(`Transfers: ${recommended.transfers}`);
```

### Display All Strategies

```javascript
async function displayAllStrategies(fromLat, fromLon, toLat, toLon) {
  const data = await searchRoute(fromLat, fromLon, toLat, toLon);

  console.log('\\n=== ROUTE OPTIONS ===\\n');

  if (data.routes.simple) {
    console.log('✓ RECOMMENDED (simple):');
    printRouteSummary(data.routes.simple);
  }

  if (data.routes.fast) {
    console.log('\\n⚡ FASTEST (fast):');
    printRouteSummary(data.routes.fast);
  }

  if (data.routes.no_transfer) {
    console.log('\\n🛋️  NO TRANSFERS (no_transfer):');
    printRouteSummary(data.routes.no_transfer);
  }

  if (data.routes.direct) {
    console.log('\\n➡️  DIRECT (direct):');
    printRouteSummary(data.routes.direct);
  }
}

function printRouteSummary(route) {
  const mins = Math.floor(route.duration_seconds / 60);
  console.log(`  Duration: ${mins} min`);
  console.log(`  Walking: ${route.walk_distance_meters}m`);
  console.log(`  Transfers: ${route.transfers}`);
  console.log(`  Steps: ${route.steps.length}`);
}
```

### Display Step-by-Step Directions

```javascript
function displayDirections(route) {
  console.log('\\n=== DIRECTIONS ===\\n');

  route.steps.forEach((step, index) => {
    console.log(`${index + 1}. ${formatStep(step)}`);
  });
}

function formatStep(step) {
  if (step.type === 'WALK') {
    return `🚶 Walk ${step.distance_meters}m to ${step.to_stop_name} (${Math.floor(step.duration_seconds / 60)} min)`;
  } else if (step.type === 'RIDE') {
    return `🚌 Take ${step.route_name} for ${step.num_stops} stops to ${step.to_stop_name} (${Math.floor(step.duration_seconds / 60)} min)`;
  } else if (step.type === 'TRANSFER') {
    return `🔄 Transfer at ${step.from_stop_name} (${Math.floor(step.duration_seconds / 60)} min wait)`;
  }
}

// Usage
const routes = await searchRoute(14.7167, -17.4677, 14.6928, -17.4467);
displayDirections(routes.routes.simple);
```

---

## Error Handling

### Comprehensive Error Handling

```javascript
class PassBiError extends Error {
  constructor(code, message, details = {}) {
    super(message);
    this.name = 'PassBiError';
    this.code = code;
    this.details = details;
  }
}

async function searchRouteWithErrorHandling(fromLat, fromLon, toLat, toLon) {
  // Validate coordinates
  validateCoordinates(fromLat, fromLon);
  validateCoordinates(toLat, toLon);

  const url = new URL('http://localhost:8080/v2/route-search');
  url.searchParams.set('from', `${fromLat},${fromLon}`);
  url.searchParams.set('to', `${toLat},${toLon}`);

  try {
    const response = await fetch(url, {
      signal: AbortSignal.timeout(15000) // 15 second timeout
    });

    // No routes found (404)
    if (response.status === 404) {
      throw new PassBiError(
        'NO_ROUTES_FOUND',
        'No routes found between the specified locations'
      );
    }

    // Invalid parameters (400)
    if (response.status === 400) {
      const error = await response.json();
      throw new PassBiError(
        'INVALID_PARAMS',
        error.error,
        { from: `${fromLat},${fromLon}`, to: `${toLat},${toLon}` }
      );
    }

    // Server error (500)
    if (response.status === 500) {
      throw new PassBiError('SERVER_ERROR', 'Internal server error');
    }

    // Service unavailable (503)
    if (response.status === 503) {
      throw new PassBiError('SERVICE_UNAVAILABLE', 'Service temporarily unavailable');
    }

    // Success
    if (response.ok) {
      return await response.json();
    }

    // Unknown error
    throw new PassBiError('UNKNOWN_ERROR', `HTTP ${response.status}`);

  } catch (error) {
    // Timeout
    if (error.name === 'TimeoutError' || error.name === 'AbortError') {
      throw new PassBiError('TIMEOUT', 'Request timeout after 15 seconds');
    }

    // Network error
    if (error.message === 'Failed to fetch') {
      throw new PassBiError('NETWORK_ERROR', 'Network connection failed');
    }

    // Re-throw PassBiError
    if (error instanceof PassBiError) {
      throw error;
    }

    // Unknown error
    throw new PassBiError('UNKNOWN_ERROR', error.message);
  }
}

function validateCoordinates(lat, lon) {
  if (typeof lat !== 'number' || typeof lon !== 'number') {
    throw new PassBiError('INVALID_COORDS', 'Coordinates must be numbers');
  }

  if (lat < -90 || lat > 90) {
    throw new PassBiError('INVALID_COORDS', `Latitude ${lat} out of range (-90 to 90)`);
  }

  if (lon < -180 || lon > 180) {
    throw new PassBiError('INVALID_COORDS', `Longitude ${lon} out of range (-180 to 180)`);
  }
}

// Usage
try {
  const routes = await searchRouteWithErrorHandling(14.7167, -17.4677, 14.6928, -17.4467);
  displayRoutes(routes);
} catch (error) {
  if (error instanceof PassBiError) {
    console.error(`Error [${error.code}]: ${error.message}`);

    // User-friendly messages
    switch (error.code) {
      case 'NO_ROUTES_FOUND':
        alert('No routes found. Try different locations or check if they\'re within the service area.');
        break;
      case 'INVALID_PARAMS':
      case 'INVALID_COORDS':
        alert('Invalid search parameters. Please check your locations.');
        break;
      case 'NETWORK_ERROR':
        alert('Network error. Please check your internet connection.');
        break;
      case 'SERVER_ERROR':
      case 'SERVICE_UNAVAILABLE':
        alert('Service temporarily unavailable. Please try again.');
        break;
      default:
        alert('An unexpected error occurred.');
    }
  } else {
    console.error('Unexpected error:', error);
  }
}
```

### Retry with Exponential Backoff

```javascript
async function fetchWithRetry(url, options = {}, maxRetries = 3) {
  const retryStatuses = [500, 502, 503, 504];

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch(url, options);

      // Don't retry on client errors or success
      if (!retryStatuses.includes(response.status) || attempt === maxRetries) {
        return response;
      }

      // Exponential backoff: 1s, 2s, 4s...
      const delay = Math.min(1000 * Math.pow(2, attempt - 1), 10000);
      console.log(`Attempt ${attempt} failed (HTTP ${response.status}), retrying in ${delay}ms...`);
      await new Promise(resolve => setTimeout(resolve, delay));

    } catch (error) {
      if (attempt === maxRetries) throw error;

      const delay = Math.min(1000 * Math.pow(2, attempt - 1), 10000);
      console.log(`Attempt ${attempt} failed (${error.message}), retrying in ${delay}ms...`);
      await new Promise(resolve => setTimeout(resolve, delay));
    }
  }
}

// Usage
async function searchRouteWithRetry(fromLat, fromLon, toLat, toLon) {
  const url = new URL('http://localhost:8080/v2/route-search');
  url.searchParams.set('from', `${fromLat},${fromLon}`);
  url.searchParams.set('to', `${toLat},${toLon}`);

  const response = await fetchWithRetry(url, {}, 3);

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return await response.json();
}
```

---

## React Hooks

### useRouteSearch Hook

```typescript
import { useState, useEffect } from 'react';

interface Coordinates {
  lat: number;
  lon: number;
}

interface UseRouteSearchResult {
  routes: any | null;
  loading: boolean;
  error: string | null;
  refetch: () => void;
}

function useRouteSearch(
  from: Coordinates | null,
  to: Coordinates | null,
  options = { enabled: true }
): UseRouteSearchResult {
  const [routes, setRoutes] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [refetchTrigger, setRefetchTrigger] = useState(0);

  const refetch = () => setRefetchTrigger(prev => prev + 1);

  useEffect(() => {
    if (!from || !to || !options.enabled) {
      return;
    }

    let cancelled = false;

    const fetchRoutes = async () => {
      setLoading(true);
      setError(null);

      try {
        const url = new URL('http://localhost:8080/v2/route-search');
        url.searchParams.set('from', `${from.lat},${from.lon}`);
        url.searchParams.set('to', `${to.lat},${to.lon}`);

        const response = await fetch(url);

        if (!response.ok) {
          if (response.status === 404) {
            throw new Error('No routes found between these locations');
          }
          const errorData = await response.json();
          throw new Error(errorData.error || `HTTP ${response.status}`);
        }

        const data = await response.json();

        if (!cancelled) {
          setRoutes(data);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Unknown error');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchRoutes();

    return () => {
      cancelled = true;
    };
  }, [from?.lat, from?.lon, to?.lat, to?.lon, options.enabled, refetchTrigger]);

  return { routes, loading, error, refetch };
}

// Usage in component
function RouteSearchComponent() {
  const [from] = useState({ lat: 14.7167, lon: -17.4677 });
  const [to] = useState({ lat: 14.6928, lon: -17.4467 });

  const { routes, loading, error, refetch } = useRouteSearch(from, to);

  if (loading) {
    return <div>Searching routes...</div>;
  }

  if (error) {
    return (
      <div>
        <p>Error: {error}</p>
        <button onClick={refetch}>Retry</button>
      </div>
    );
  }

  if (!routes) {
    return null;
  }

  return (
    <div>
      <h2>Route Options</h2>
      {Object.entries(routes.routes).map(([strategy, route]: [string, any]) => (
        <div key={strategy}>
          <h3>{strategy}</h3>
          <p>Duration: {Math.floor(route.duration_seconds / 60)} min</p>
          <p>Transfers: {route.transfers}</p>
        </div>
      ))}
      <button onClick={refetch}>Refresh</button>
    </div>
  );
}
```

### useNearbyStops Hook

```typescript
function useNearbyStops(
  lat: number | null,
  lon: number | null,
  radius = 500
): { stops: any[] | null; loading: boolean; error: string | null } {
  const [stops, setStops] = useState(null);
  const [loading, setLoading] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (lat === null || lon === null) {
      return;
    }

    let cancelled = false;

    const fetchStops = async () => {
      setLoading(true);
      setError(null);

      try {
        const url = new URL('http://localhost:8080/v2/stops/nearby');
        url.searchParams.set('lat', lat.toString());
        url.searchParams.set('lon', lon.toString());
        url.searchParams.set('radius', radius.toString());

        const response = await fetch(url);

        if (!response.ok) {
          const errorData = await response.json();
          throw new Error(errorData.error || `HTTP ${response.status}`);
        }

        const data = await response.json();

        if (!cancelled) {
          setStops(data.stops);
        }
      } catch (err) {
        if (!cancelled) {
          setError(err instanceof Error ? err.message : 'Unknown error');
        }
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };

    fetchStops();

    return () => {
      cancelled = true;
    };
  }, [lat, lon, radius]);

  return { stops, loading, error };
}
```

---

## Nearby Stops

```javascript
async function findNearbyStops(lat, lon, radiusMeters = 500) {
  const url = new URL('http://localhost:8080/v2/stops/nearby');
  url.searchParams.set('lat', lat.toString());
  url.searchParams.set('lon', lon.toString());
  url.searchParams.set('radius', radiusMeters.toString());

  const response = await fetch(url);

  if (!response.ok) {
    throw new Error(`HTTP ${response.status}`);
  }

  return await response.json();
}

// Usage
const stops = await findNearbyStops(14.6928, -17.4467, 500);

console.log(`Found ${stops.stops.length} nearby stops:\\n`);
stops.stops.forEach(stop => {
  console.log(`${stop.name}`);
  console.log(`  Distance: ${stop.distance_meters}m`);
  console.log(`  Routes: ${stop.routes.join(', ')} (${stop.routes_count} total)`);
  console.log();
});
```

---

## Routes List

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
console.log(`Total routes: ${allRoutes.total}`);

const busRoutes = await listRoutes({ mode: 'BUS', limit: 10 });
console.log(`Bus routes: ${busRoutes.total}`);

const agencyRoutes = await listRoutes({ agency: 'dakar_dem_dikk' });
console.log(`Agency routes: ${agencyRoutes.total}`);
```

---

## Complete Client Library

```javascript
class PassBiClient {
  constructor(baseURL = 'http://localhost:8080') {
    this.baseURL = baseURL;
    this.cache = new Map();
    this.cacheTTLs = {
      'route-search': 10 * 60 * 1000,      // 10 minutes
      'stops-nearby': 60 * 60 * 1000,      // 1 hour
      'routes-list': 24 * 60 * 60 * 1000   // 24 hours
    };
  }

  // Cache management
  _getCacheKey(endpoint, params) {
    const sortedParams = Object.keys(params)
      .sort()
      .map(k => `${k}=${params[k]}`)
      .join('&');
    return `${endpoint}?${sortedParams}`;
  }

  _getCache(endpoint, params) {
    const key = this._getCacheKey(endpoint, params);
    const cached = this.cache.get(key);

    if (!cached) return null;

    const ttl = this.cacheTTLs[endpoint] || 600000;
    const age = Date.now() - cached.timestamp;

    if (age > ttl) {
      this.cache.delete(key);
      return null;
    }

    return cached.data;
  }

  _setCache(endpoint, params, data) {
    const key = this._getCacheKey(endpoint, params);
    this.cache.set(key, {
      data,
      timestamp: Date.now()
    });
  }

  clearCache() {
    this.cache.clear();
  }

  // Core fetch method
  async _fetch(endpoint, params = {}) {
    // Check cache
    const cached = this._getCache(endpoint, params);
    if (cached) {
      console.log(`[PassBi] Cache hit: ${endpoint}`);
      return cached;
    }

    // Build URL
    const url = new URL(`${this.baseURL}${endpoint}`);
    Object.entries(params).forEach(([k, v]) => {
      url.searchParams.set(k, v.toString());
    });

    // Fetch
    const response = await fetch(url, {
      signal: AbortSignal.timeout(15000)
    });

    if (!response.ok) {
      const error = await response.json().catch(() => ({ error: `HTTP ${response.status}` }));
      throw new PassBiError(
        response.status === 404 ? 'NOT_FOUND' : 'API_ERROR',
        error.error
      );
    }

    const data = await response.json();

    // Cache result
    this._setCache(endpoint, params, data);

    return data;
  }

  // Route search
  async searchRoute(fromLat, fromLon, toLat, toLon) {
    return this._fetch('/v2/route-search', {
      from: `${fromLat},${fromLon}`,
      to: `${toLat},${toLon}`
    });
  }

  // Nearby stops
  async nearbyStops(lat, lon, radius = 500) {
    return this._fetch('/v2/stops/nearby', { lat, lon, radius });
  }

  // Routes list
  async listRoutes(filters = {}) {
    return this._fetch('/v2/routes/list', filters);
  }

  // Health check
  async health() {
    const response = await fetch(`${this.baseURL}/health`);
    return await response.json();
  }

  // Helper methods
  getRecommendedRoute(routeData) {
    return (
      routeData.routes.simple ||
      routeData.routes.direct ||
      routeData.routes.fast ||
      routeData.routes.no_transfer
    );
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
    });
  }
}

// Usage
const client = new PassBiClient();

try {
  // Search routes
  const routes = await client.searchRoute(14.7167, -17.4677, 14.6928, -17.4467);
  const recommended = client.getRecommendedRoute(routes);

  console.log(`Duration: ${Math.floor(recommended.duration_seconds / 60)} min`);
  console.log(`Transfers: ${recommended.transfers}`);

  const steps = client.formatSteps(recommended.steps);
  steps.forEach((step, i) => console.log(`${i + 1}. ${step}`));

  // Find nearby stops
  const stops = await client.nearbyStops(14.6928, -17.4467, 500);
  console.log(`Found ${stops.stops.length} stops`);

  // List routes
  const routesList = await client.listRoutes({ mode: 'BUS', limit: 10 });
  console.log(`${routesList.total} routes`);

} catch (error) {
  console.error('Error:', error.message);
}
```

---

## Advanced Examples

### Request Cancellation

```javascript
let currentController = null;

async function searchWithCancellation(fromLat, fromLon, toLat, toLon) {
  // Cancel previous request
  if (currentController) {
    currentController.abort();
  }

  // Create new controller
  currentController = new AbortController();

  try {
    const url = new URL('http://localhost:8080/v2/route-search');
    url.searchParams.set('from', `${fromLat},${fromLon}`);
    url.searchParams.set('to', `${toLat},${toLon}`);

    const response = await fetch(url, {
      signal: currentController.signal
    });

    return await response.json();
  } catch (error) {
    if (error.name === 'AbortError') {
      console.log('Request cancelled');
      return null;
    }
    throw error;
  } finally {
    if (currentController.signal.aborted) {
      currentController = null;
    }
  }
}
```

### Debounced Search

```javascript
function debounce(func, wait) {
  let timeout;
  return function(...args) {
    clearTimeout(timeout);
    timeout = setTimeout(() => func.apply(this, args), wait);
  };
}

const searchRouteDebounced = debounce(async (from, to) => {
  const routes = await searchRoute(from.lat, from.lon, to.lat, to.lon);
  displayRoutes(routes);
}, 500); // Wait 500ms after last input

// Usage with input events
originInput.addEventListener('input', () => {
  const from = getOriginCoordinates();
  const to = getDestinationCoordinates();
  searchRouteDebounced(from, to);
});
```

---

## See Also

- [Python Examples](python.md) - Python integration examples
- [cURL Examples](curl.md) - Command-line testing
- [Integration Guide](../../guides/integration-guide.md) - Complete integration guide
- [Error Reference](../reference/errors.md) - Error handling guide
