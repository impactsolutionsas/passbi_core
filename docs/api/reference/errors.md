# Error Reference

Complete catalog of API errors with troubleshooting guidance.

## Table of Contents

- [Error Response Format](#error-response-format)
- [HTTP Status Codes](#http-status-codes)
- [Common Errors](#common-errors)
- [Debugging Tips](#debugging-tips)
- [Error Handling Best Practices](#error-handling-best-practices)

---

## Error Response Format

All API errors return JSON with this structure:

```json
{
  "error": "human-readable error message"
}
```

**TypeScript:**
```typescript
interface ErrorResponse {
  error: string;
}
```

**Example:**
```json
{
  "error": "missing required parameters: from and to"
}
```

---

## HTTP Status Codes

### 200 OK
Request succeeded. Response body contains the requested data.

**Endpoints:**
- `GET /health` (when healthy)
- `GET /v2/route-search` (when routes found)
- `GET /v2/stops/nearby` (always)
- `GET /v2/routes/list` (always)

---

### 400 Bad Request

**Meaning:** Invalid or malformed request parameters.

**Common Causes:**
- Missing required parameters
- Invalid coordinate format
- Coordinates out of valid range
- Invalid parameter values

**Solutions:**
- Validate input client-side before making requests
- Check coordinate format: `"lat,lon"` (comma-separated, no spaces)
- Ensure latitude is between -90 and 90
- Ensure longitude is between -180 and 180
- Check parameter constraints (e.g., radius 0-5000)

---

#### Missing Required Parameters

**Endpoint:** `/v2/route-search`

**Error:**
```json
{
  "error": "missing required parameters: from and to"
}
```

**Cause:** The `from` or `to` query parameter is missing.

**Solution:**
```bash
# Bad
curl "http://localhost:8080/v2/route-search?from=14.7,-17.4"

# Good
curl "http://localhost:8080/v2/route-search?from=14.7,-17.4&to=14.6,-17.3"
```

**Code Example:**
```javascript
// Validate before making request
if (!fromLat || !fromLon || !toLat || !toLon) {
  throw new Error("Origin and destination coordinates are required");
}

const url = new URL('http://localhost:8080/v2/route-search');
url.searchParams.set('from', `${fromLat},${fromLon}`);
url.searchParams.set('to', `${toLat},${toLon}`);
```

---

**Endpoint:** `/v2/stops/nearby`

**Error:**
```json
{
  "error": "missing required parameters: lat and lon"
}
```

**Cause:** The `lat` or `lon` query parameter is missing.

**Solution:**
```bash
# Bad
curl "http://localhost:8080/v2/stops/nearby?lat=14.7"

# Good
curl "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4"
```

---

#### Invalid Coordinate Format

**Endpoint:** `/v2/route-search`

**Errors:**
```json
{
  "error": "invalid 'from' coordinates: expected format: lat,lon"
}
```
```json
{
  "error": "invalid 'to' coordinates: expected format: lat,lon"
}
```

**Common Causes:**
- Missing comma separator
- Space in coordinate string
- Non-numeric values
- Extra characters

**Examples:**

```bash
# Bad - space instead of comma
curl "http://localhost:8080/v2/route-search?from=14.7 -17.4&to=14.6,-17.3"

# Bad - missing comma
curl "http://localhost:8080/v2/route-search?from=14.7-17.4&to=14.6,-17.3"

# Good
curl "http://localhost:8080/v2/route-search?from=14.7,-17.4&to=14.6,-17.3"
```

**Solution:**
```javascript
// Always format as "lat,lon"
const from = `${fromLat},${fromLon}`;
const to = `${toLat},${toLon}`;

// Validate format with regex
const coordPattern = /^-?\d+\.?\d*,-?\d+\.?\d*$/;
if (!coordPattern.test(from)) {
  throw new Error("Invalid coordinate format");
}
```

---

#### Coordinates Out of Range

**Errors:**
```json
{
  "error": "invalid 'from' coordinates: latitude must be between -90 and 90"
}
```
```json
{
  "error": "invalid 'from' coordinates: longitude must be between -180 and 180"
}
```

**Cause:** Latitude or longitude values are outside valid ranges.

**Valid Ranges:**
- **Latitude:** -90 to 90
- **Longitude:** -180 to 180

**Solution:**
```javascript
function validateCoordinates(lat, lon) {
  if (lat < -90 || lat > 90) {
    throw new Error(`Invalid latitude: ${lat} (must be -90 to 90)`);
  }
  if (lon < -180 || lon > 180) {
    throw new Error(`Invalid longitude: ${lon} (must be -180 to 180)`);
  }
}

// Usage
validateCoordinates(fromLat, fromLon);
validateCoordinates(toLat, toLon);
```

---

#### Invalid Latitude/Longitude

**Endpoint:** `/v2/stops/nearby`

**Errors:**
```json
{
  "error": "invalid latitude"
}
```
```json
{
  "error": "invalid longitude"
}
```

**Cause:** Non-numeric or malformed latitude/longitude values.

**Solution:**
```bash
# Bad - non-numeric
curl "http://localhost:8080/v2/stops/nearby?lat=abc&lon=-17.4"

# Good
curl "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4"
```

```javascript
// Validate numeric values
const lat = parseFloat(latStr);
const lon = parseFloat(lonStr);

if (isNaN(lat) || isNaN(lon)) {
  throw new Error("Latitude and longitude must be numbers");
}
```

---

#### Invalid Radius

**Endpoint:** `/v2/stops/nearby`

**Error:**
```json
{
  "error": "invalid radius (must be between 0 and 5000 meters)"
}
```

**Cause:** Radius parameter is not a number, negative, or exceeds 5000 meters.

**Valid Range:** 0 to 5000 meters

**Solution:**
```bash
# Bad - exceeds max
curl "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4&radius=10000"

# Bad - negative
curl "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4&radius=-500"

# Good
curl "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4&radius=500"
```

```javascript
// Validate radius
const radius = parseInt(radiusStr, 10);

if (isNaN(radius) || radius < 0 || radius > 5000) {
  throw new Error("Radius must be between 0 and 5000 meters");
}
```

---

### 404 Not Found

**Meaning:** Resource not found or no results available.

**Common Causes:**
- No routes found between specified locations
- Invalid endpoint path

---

#### No Routes Found

**Endpoint:** `/v2/route-search`

**Error:**
```json
{
  "error": "no routes found between the specified locations"
}
```

**Common Reasons:**
1. Locations are too far apart (>50km typically)
2. One or both locations are not near any transit stops
3. No transit service connects the two areas
4. Locations are in different regions served by different agencies

**Solutions:**

1. **Check if locations are near transit stops:**
```bash
# Check origin
curl "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4&radius=1000"

# Check destination
curl "http://localhost:8080/v2/stops/nearby?lat=14.6&lon=-17.3&radius=1000"
```

2. **Try closer locations:**
```javascript
// If no routes found, suggest adjusting locations
if (response.status === 404) {
  showMessage(
    "No routes found. Your start or end location may be too far " +
    "from transit stops, or they may not be connected by transit."
  );

  // Suggest nearby stops
  const nearbyStops = await fetchNearbyStops(fromLat, fromLon);
  if (nearbyStops.stops.length > 0) {
    showMessage(`Nearest stop: ${nearbyStops.stops[0].name}`);
  }
}
```

3. **Verify service area:**
```bash
# Check available routes in the area
curl "http://localhost:8080/v2/routes/list?limit=10"
```

---

#### Invalid Endpoint

**Error:** HTML 404 page or no response

**Cause:** Incorrect endpoint path.

**Common Mistakes:**
- Missing `/v2/` prefix
- Typo in endpoint name
- Using wrong HTTP method

**Solution:**
```bash
# Bad - missing /v2/ prefix
curl "http://localhost:8080/route-search?from=14.7,-17.4&to=14.6,-17.3"

# Good
curl "http://localhost:8080/v2/route-search?from=14.7,-17.4&to=14.6,-17.3"
```

---

### 500 Internal Server Error

**Meaning:** Unexpected server-side error.

**Error:**
```json
{
  "error": "internal server error"
}
```

**Common Causes:**
- Database connection failure
- Cache (Redis) connection failure
- Unexpected exception during processing

**Solutions:**

1. **Retry the request** (may be transient):
```javascript
async function fetchWithRetry(url, maxRetries = 3) {
  for (let i = 0; i < maxRetries; i++) {
    try {
      const response = await fetch(url);

      if (response.status === 500 && i < maxRetries - 1) {
        // Wait before retrying (exponential backoff)
        await new Promise(r => setTimeout(r, 1000 * (i + 1)));
        continue;
      }

      return response;
    } catch (error) {
      if (i === maxRetries - 1) throw error;
    }
  }
}
```

2. **Check API health:**
```bash
curl "http://localhost:8080/health"
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

3. **If problem persists:**
- Check server logs for details
- Verify database and Redis are running
- Contact support with request details

---

### 503 Service Unavailable

**Meaning:** API dependencies are unhealthy.

**Endpoint:** `/health`

**Error:**
```json
{
  "status": "unhealthy",
  "checks": {
    "database": "connection refused",
    "redis": "ok"
  }
}
```

**Causes:**
- Database server is down or unreachable
- Redis server is down or unreachable
- Network connectivity issues

**Solutions:**

1. **Check which dependency failed:**
```bash
curl "http://localhost:8080/health"
```

2. **Wait and retry:**
```javascript
async function waitForHealthy(maxAttempts = 5, delay = 3000) {
  for (let i = 0; i < maxAttempts; i++) {
    try {
      const response = await fetch('http://localhost:8080/health');
      const health = await response.json();

      if (health.status === 'healthy') {
        return true;
      }

      console.log(`Attempt ${i + 1}: ${health.checks.database}, ${health.checks.redis}`);
      await new Promise(r => setTimeout(r, delay));
    } catch (error) {
      console.error('Health check failed:', error);
    }
  }

  return false;
}
```

3. **If persists:**
- Service may be down for maintenance
- Check status page or announcements
- Contact support

---

## Debugging Tips

### 1. Test with cURL First

Before debugging your application, verify the API works with cURL:

```bash
# Health check
curl -v "http://localhost:8080/health"

# Route search
curl -v "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"

# Nearby stops
curl -v "http://localhost:8080/v2/stops/nearby?lat=14.7&lon=-17.4&radius=500"
```

The `-v` flag shows full request/response details.

---

### 2. Verify Coordinate Format

```javascript
// Test coordinate parsing
const coordStr = "14.7167,-17.4677";
const [latStr, lonStr] = coordStr.split(',');
const lat = parseFloat(latStr);
const lon = parseFloat(lonStr);

console.log(`Lat: ${lat}, Lon: ${lon}`);
console.log(`Valid: ${!isNaN(lat) && !isNaN(lon)}`);
```

---

### 3. Check URL Encoding

Special characters in URLs must be encoded:

```javascript
// Coordinates don't need encoding (comma is OK in query strings)
const url = new URL('http://localhost:8080/v2/route-search');
url.searchParams.set('from', '14.7167,-17.4677');
url.searchParams.set('to', '14.6928,-17.4467');

console.log(url.toString());
// http://localhost:8080/v2/route-search?from=14.7167%2C-17.4677&to=14.6928%2C-17.4467
```

---

### 4. Validate JSON Response

```javascript
try {
  const response = await fetch(url);
  const contentType = response.headers.get('content-type');

  if (!contentType || !contentType.includes('application/json')) {
    throw new Error(`Expected JSON, got ${contentType}`);
  }

  const data = await response.json();
  console.log(data);
} catch (error) {
  console.error('JSON parsing error:', error);
}
```

---

### 5. Check Health Endpoint First

Always verify API availability before complex debugging:

```javascript
async function verifyAPIHealth() {
  try {
    const response = await fetch('http://localhost:8080/health');
    const health = await response.json();

    if (health.status !== 'healthy') {
      console.error('API is unhealthy:', health.checks);
      return false;
    }

    return true;
  } catch (error) {
    console.error('Cannot reach API:', error);
    return false;
  }
}
```

---

## Error Handling Best Practices

### 1. Validate Input Client-Side

```javascript
function validateRouteSearchParams(fromLat, fromLon, toLat, toLon) {
  const errors = [];

  if (typeof fromLat !== 'number' || typeof fromLon !== 'number') {
    errors.push('Origin coordinates must be numbers');
  }
  if (typeof toLat !== 'number' || typeof toLon !== 'number') {
    errors.push('Destination coordinates must be numbers');
  }

  if (fromLat < -90 || fromLat > 90) {
    errors.push('Origin latitude must be between -90 and 90');
  }
  if (fromLon < -180 || fromLon > 180) {
    errors.push('Origin longitude must be between -180 and 180');
  }
  if (toLat < -90 || toLat > 90) {
    errors.push('Destination latitude must be between -90 and 90');
  }
  if (toLon < -180 || toLon > 180) {
    errors.push('Destination longitude must be between -180 and 180');
  }

  if (errors.length > 0) {
    throw new Error(errors.join('; '));
  }
}
```

---

### 2. Handle All HTTP Status Codes

```javascript
async function searchRoute(fromLat, fromLon, toLat, toLon) {
  const url = new URL('http://localhost:8080/v2/route-search');
  url.searchParams.set('from', `${fromLat},${fromLon}`);
  url.searchParams.set('to', `${toLat},${toLon}`);

  try {
    const response = await fetch(url);

    // 200 OK
    if (response.ok) {
      return await response.json();
    }

    // 404 Not Found
    if (response.status === 404) {
      throw new Error('NO_ROUTES_FOUND');
    }

    // 400 Bad Request
    if (response.status === 400) {
      const error = await response.json();
      throw new Error(`INVALID_PARAMS: ${error.error}`);
    }

    // 500 Internal Server Error
    if (response.status === 500) {
      throw new Error('SERVER_ERROR');
    }

    // 503 Service Unavailable
    if (response.status === 503) {
      throw new Error('SERVICE_UNAVAILABLE');
    }

    // Other errors
    throw new Error(`HTTP_${response.status}`);

  } catch (error) {
    if (error.message === 'Failed to fetch') {
      throw new Error('NETWORK_ERROR');
    }
    throw error;
  }
}
```

---

### 3. Implement Retry Logic for Transient Errors

```javascript
async function fetchWithRetry(url, options = {}, maxRetries = 3) {
  const retryStatuses = [500, 502, 503, 504];

  for (let attempt = 1; attempt <= maxRetries; attempt++) {
    try {
      const response = await fetch(url, options);

      if (!retryStatuses.includes(response.status) || attempt === maxRetries) {
        return response;
      }

      // Exponential backoff
      const delay = Math.min(1000 * Math.pow(2, attempt - 1), 10000);
      console.log(`Retry attempt ${attempt} after ${delay}ms`);
      await new Promise(r => setTimeout(r, delay));

    } catch (error) {
      if (attempt === maxRetries) throw error;

      const delay = Math.min(1000 * Math.pow(2, attempt - 1), 10000);
      await new Promise(r => setTimeout(r, delay));
    }
  }
}
```

---

### 4. Provide User-Friendly Error Messages

```javascript
function getUserFriendlyErrorMessage(error) {
  const messages = {
    NO_ROUTES_FOUND: "No routes found. Try different locations or check if they're within the service area.",
    INVALID_PARAMS: "Invalid search parameters. Please check your locations.",
    SERVER_ERROR: "Server error. Please try again in a moment.",
    SERVICE_UNAVAILABLE: "Service temporarily unavailable. Please try again later.",
    NETWORK_ERROR: "Network error. Please check your internet connection.",
  };

  return messages[error.message] || "An unexpected error occurred.";
}

// Usage
try {
  const routes = await searchRoute(fromLat, fromLon, toLat, toLon);
  showRoutes(routes);
} catch (error) {
  const message = getUserFriendlyErrorMessage(error);
  showErrorMessage(message);
}
```

---

### 5. Log Errors for Debugging

```javascript
async function searchRouteWithLogging(fromLat, fromLon, toLat, toLon) {
  const context = {
    endpoint: '/v2/route-search',
    params: { from: `${fromLat},${fromLon}`, to: `${toLat},${toLon}` },
    timestamp: new Date().toISOString()
  };

  try {
    const routes = await searchRoute(fromLat, fromLon, toLat, toLon);

    console.log('Route search succeeded', context);
    return routes;

  } catch (error) {
    console.error('Route search failed', {
      ...context,
      error: error.message,
      stack: error.stack
    });

    // Optional: Send to error tracking service
    // await logErrorToService(error, context);

    throw error;
  }
}
```

---

## See Also

- [Data Models Reference](data-models.md) - Complete data structure documentation
- [Integration Guide](../../guides/integration-guide.md) - Error handling best practices
- [OpenAPI Specification](../openapi.yaml) - Complete API specification with error schemas
