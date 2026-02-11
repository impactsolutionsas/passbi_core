# Data Models Reference

Complete reference for all API data structures and contracts.

## Table of Contents

- [Route Search Models](#route-search-models)
- [Stop Models](#stop-models)
- [Route Catalog Models](#route-catalog-models)
- [System Models](#system-models)
- [Enumerations](#enumerations)
- [Validation Rules](#validation-rules)

---

## Route Search Models

### RouteSearchResponse

Response containing multiple route options optimized by different strategies.

**Structure:**
```json
{
  "routes": {
    "no_transfer": RouteResult,
    "direct": RouteResult,
    "simple": RouteResult,
    "fast": RouteResult
  }
}
```

**TypeScript:**
```typescript
interface RouteSearchResponse {
  routes: {
    no_transfer?: RouteResult;
    direct?: RouteResult;
    simple?: RouteResult;
    fast?: RouteResult;
  };
}
```

**Notes:**
- Up to 4 strategies may be present in the response
- If a strategy cannot find a route, it will be omitted
- At least one strategy will always be present (or 404 is returned)
- Strategy names are fixed: `no_transfer`, `direct`, `simple`, `fast`

---

### RouteResult

A single route option with complete journey details.

**Structure:**
```json
{
  "duration_seconds": 1200,
  "walk_distance_meters": 350,
  "transfers": 1,
  "steps": [Step, Step, ...]
}
```

**TypeScript:**
```typescript
interface RouteResult {
  duration_seconds: number;      // Total journey time (seconds)
  walk_distance_meters: number;  // Total walking distance (meters)
  transfers: number;             // Number of transfers
  steps: Step[];                 // Step-by-step directions
}
```

**Field Details:**

| Field | Type | Description | Example | Constraints |
|-------|------|-------------|---------|-------------|
| `duration_seconds` | integer | Total journey duration including walking, waiting, and riding | `1200` (20 minutes) | ≥ 0 |
| `walk_distance_meters` | integer | Sum of all walking distances in the route | `350` | ≥ 0 |
| `transfers` | integer | Number of times user must change routes | `1` | ≥ 0 |
| `steps` | Step[] | Ordered array of journey segments | See Step model | length ≥ 1 |

**Example:**
```json
{
  "duration_seconds": 1200,
  "walk_distance_meters": 350,
  "transfers": 1,
  "steps": [
    {
      "type": "WALK",
      "from_stop": "origin",
      "to_stop": "D_123",
      "from_stop_name": "Your Location",
      "to_stop_name": "Gare Routière",
      "duration_seconds": 140,
      "distance_meters": 200
    },
    {
      "type": "RIDE",
      "from_stop": "D_123",
      "to_stop": "D_456",
      "from_stop_name": "Gare Routière",
      "to_stop_name": "Place de l'Indépendance",
      "route": "DDD_01",
      "route_name": "D1LP",
      "mode": "BUS",
      "duration_seconds": 900,
      "num_stops": 12
    },
    {
      "type": "TRANSFER",
      "from_stop": "D_456",
      "to_stop": "D_456",
      "from_stop_name": "Place de l'Indépendance",
      "to_stop_name": "Place de l'Indépendance",
      "duration_seconds": 180
    },
    {
      "type": "RIDE",
      "from_stop": "D_456",
      "to_stop": "D_789",
      "from_stop_name": "Place de l'Indépendance",
      "to_stop_name": "Plateau",
      "route": "DDD_05",
      "route_name": "D5TH",
      "mode": "BUS",
      "duration_seconds": 600,
      "num_stops": 8
    },
    {
      "type": "WALK",
      "from_stop": "D_789",
      "to_stop": "destination",
      "from_stop_name": "Plateau",
      "to_stop_name": "Destination",
      "duration_seconds": 60,
      "distance_meters": 80
    }
  ]
}
```

---

### Step

A single segment of a journey (walking, riding, or transferring).

**TypeScript:**
```typescript
interface Step {
  type: "WALK" | "RIDE" | "TRANSFER";
  from_stop: string;              // Stop ID
  to_stop: string;                // Stop ID
  from_stop_name: string;         // Human-readable stop name
  to_stop_name: string;           // Human-readable stop name
  route?: string;                 // Route ID (RIDE only)
  route_name?: string;            // Route name (RIDE only)
  mode?: TransitMode;             // Transit mode (RIDE only)
  duration_seconds: number;       // Duration of this step
  distance_meters?: number;       // Walking distance (WALK only)
  num_stops?: number;             // Stops traversed (RIDE only)
}
```

**Step Types:**

#### WALK Step
User walks between two stops.

```json
{
  "type": "WALK",
  "from_stop": "origin",
  "to_stop": "D_123",
  "from_stop_name": "Your Location",
  "to_stop_name": "Gare Routière",
  "duration_seconds": 140,
  "distance_meters": 200
}
```

**Fields:**
- `type`: Always `"WALK"`
- `from_stop`, `to_stop`: Stop IDs (origin may be `"origin"`, destination may be `"destination"`)
- `from_stop_name`, `to_stop_name`: Human-readable names
- `duration_seconds`: Walking time (calculated from distance ÷ walking speed)
- `distance_meters`: Walking distance
- `route`, `route_name`, `mode`, `num_stops`: Not present

#### RIDE Step
User rides a transit vehicle.

```json
{
  "type": "RIDE",
  "from_stop": "D_123",
  "to_stop": "D_456",
  "from_stop_name": "Gare Routière",
  "to_stop_name": "Place de l'Indépendance",
  "route": "DDD_01",
  "route_name": "D1LP",
  "mode": "BUS",
  "duration_seconds": 900,
  "num_stops": 12
}
```

**Fields:**
- `type`: Always `"RIDE"`
- `from_stop`, `to_stop`: Stop IDs
- `from_stop_name`, `to_stop_name`: Human-readable names
- `route`: Route ID (e.g., `"DDD_01"`)
- `route_name`: Human-readable route name/number (e.g., `"D1LP"`)
- `mode`: Transit mode (BUS, BRT, TER, FERRY, TRAM)
- `duration_seconds`: Riding time
- `num_stops`: Number of stops traversed during this ride
- `distance_meters`: Not present

#### TRANSFER Step
User transfers between routes at the same stop.

```json
{
  "type": "TRANSFER",
  "from_stop": "D_456",
  "to_stop": "D_456",
  "from_stop_name": "Place de l'Indépendance",
  "to_stop_name": "Place de l'Indépendance",
  "duration_seconds": 180
}
```

**Fields:**
- `type`: Always `"TRANSFER"`
- `from_stop`, `to_stop`: Same stop ID (transfer happens at one location)
- `from_stop_name`, `to_stop_name`: Same stop name
- `duration_seconds`: Transfer time (default: 180 seconds / 3 minutes)
- `route`, `route_name`, `mode`, `distance_meters`, `num_stops`: Not present

---

## Stop Models

### NearbyStopsResponse

Response containing nearby transit stops.

**Structure:**
```json
{
  "stops": [NearbyStop, NearbyStop, ...]
}
```

**TypeScript:**
```typescript
interface NearbyStopsResponse {
  stops: NearbyStop[];
}
```

**Notes:**
- Maximum 20 stops returned
- Ordered by distance (closest first)
- Empty array if no stops found within radius

---

### NearbyStop

A transit stop with its location and serving routes.

**Structure:**
```json
{
  "id": "D_771",
  "name": "Face Eglise Temple Évangélique",
  "lat": 14.692267,
  "lon": -17.447672,
  "distance_meters": 120,
  "routes": ["D105CP", "D111LY", "D7OP"],
  "routes_count": 3
}
```

**TypeScript:**
```typescript
interface NearbyStop {
  id: string;                  // Unique stop ID
  name: string;                // Stop name
  lat: number;                 // Latitude
  lon: number;                 // Longitude
  distance_meters: number;     // Distance from query point
  routes: string[];            // Route names serving this stop
  routes_count: number;        // Number of routes
}
```

**Field Details:**

| Field | Type | Description | Example | Constraints |
|-------|------|-------------|---------|-------------|
| `id` | string | Unique stop identifier | `"D_771"` | Non-empty |
| `name` | string | Human-readable stop name | `"Gare Routière"` | Non-empty |
| `lat` | number | Latitude | `14.692267` | -90 to 90 |
| `lon` | number | Longitude | `-17.447672` | -180 to 180 |
| `distance_meters` | integer | Distance from query point | `120` | ≥ 0 |
| `routes` | string[] | Route names/numbers | `["D1LP", "D5TH"]` | May be empty |
| `routes_count` | integer | Number of routes | `3` | ≥ 0 |

**Example:**
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
    },
    {
      "id": "D_772",
      "name": "Marché Kermel",
      "lat": 14.692567,
      "lon": -17.448012,
      "distance_meters": 250,
      "routes": ["D1LP", "D5TH"],
      "routes_count": 2
    }
  ]
}
```

---

## Route Catalog Models

### RoutesListResponse

Response containing a list of transit routes.

**Structure:**
```json
{
  "routes": [RouteInfo, RouteInfo, ...],
  "total": 134
}
```

**TypeScript:**
```typescript
interface RoutesListResponse {
  routes: RouteInfo[];
  total: number;
}
```

**Notes:**
- `total` reflects the number of routes in the response (not total in database)
- Empty array if no routes match filters

---

### RouteInfo

Information about a single transit route.

**Structure:**
```json
{
  "id": "DDD_01",
  "name": "D1LP",
  "mode": "BUS",
  "agency_id": "dakar_dem_dikk",
  "stops_count": 75
}
```

**TypeScript:**
```typescript
interface RouteInfo {
  id: string;                  // Unique route ID
  name: string;                // Route name/number
  mode: TransitMode;           // Transit mode
  agency_id: string;           // Operating agency ID
  stops_count: number;         // Number of stops
}
```

**Field Details:**

| Field | Type | Description | Example | Constraints |
|-------|------|-------------|---------|-------------|
| `id` | string | Unique route identifier | `"DDD_01"` | Non-empty |
| `name` | string | Human-readable route name/number | `"D1LP"` | Non-empty |
| `mode` | TransitMode | Transit mode | `"BUS"` | See TransitMode enum |
| `agency_id` | string | Agency operating this route | `"dakar_dem_dikk"` | Non-empty |
| `stops_count` | integer | Number of stops on this route | `75` | ≥ 0 |

**Example:**
```json
{
  "routes": [
    {
      "id": "DDD_01",
      "name": "D1LP",
      "mode": "BUS",
      "agency_id": "dakar_dem_dikk",
      "stops_count": 75
    },
    {
      "id": "DDD_05",
      "name": "D5TH",
      "mode": "BUS",
      "agency_id": "dakar_dem_dikk",
      "stops_count": 48
    }
  ],
  "total": 134
}
```

---

## System Models

### HealthResponse

Health status of the API and its dependencies.

**Structure:**
```json
{
  "status": "healthy",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

**TypeScript:**
```typescript
interface HealthResponse {
  status: "healthy" | "unhealthy";
  checks: {
    database: string;  // "ok" or error message
    redis: string;     // "ok" or error message
  };
}
```

**Status Values:**
- `"healthy"`: All dependencies are operational
- `"unhealthy"`: One or more dependencies have failed

**Check Values:**
- `"ok"`: Dependency is healthy
- Error message string: Dependency has failed (e.g., `"connection refused"`)

**Examples:**

**Healthy:**
```json
{
  "status": "healthy",
  "checks": {
    "database": "ok",
    "redis": "ok"
  }
}
```

**Unhealthy:**
```json
{
  "status": "unhealthy",
  "checks": {
    "database": "connection refused",
    "redis": "ok"
  }
}
```

---

### ErrorResponse

Standard error response format.

**Structure:**
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

**Examples:**
```json
{"error": "missing required parameters: from and to"}
{"error": "invalid 'from' coordinates: latitude must be between -90 and 90"}
{"error": "no routes found between the specified locations"}
{"error": "internal server error"}
```

See [Error Reference](errors.md) for complete error catalog.

---

## Enumerations

### TransitMode

Types of transit services.

**TypeScript:**
```typescript
enum TransitMode {
  BUS = "BUS",       // Standard bus service
  BRT = "BRT",       // Bus Rapid Transit
  TER = "TER",       // Train (Train Express Régional)
  FERRY = "FERRY",   // Ferry service
  TRAM = "TRAM"      // Tramway
}
```

**Values:**
- `"BUS"`: Standard bus service
- `"BRT"`: Bus Rapid Transit (dedicated lanes, high frequency)
- `"TER"`: Train / Rail service (Train Express Régional)
- `"FERRY"`: Ferry / Boat service
- `"TRAM"`: Tramway / Light rail

---

### StepType

Types of journey segments.

**TypeScript:**
```typescript
enum StepType {
  WALK = "WALK",         // Walking between stops
  RIDE = "RIDE",         // Riding a transit vehicle
  TRANSFER = "TRANSFER"  // Transferring between routes
}
```

**Values:**
- `"WALK"`: User walks between stops
- `"RIDE"`: User rides a transit vehicle
- `"TRANSFER"`: User transfers between routes at the same stop

---

### RoutingStrategy

Available routing optimization strategies.

**TypeScript:**
```typescript
enum RoutingStrategy {
  NO_TRANSFER = "no_transfer",  // Zero transfers, single line
  DIRECT = "direct",            // Minimize transfers
  SIMPLE = "simple",            // Balanced (recommended)
  FAST = "fast"                 // Minimize time
}
```

**Values:**
- `"no_transfer"`: Absolutely zero transfers (single transit line only)
- `"direct"`: Minimize or eliminate transfers (simplicity focused)
- `"simple"`: Balance time, walking, and transfers (recommended default)
- `"fast"`: Minimize total travel time (may require more transfers/walking)

See [Routing Strategies Guide](../../guides/routing-strategies.md) for detailed explanations.

---

## Validation Rules

### Coordinates

**Latitude:**
- Type: `number` (float64)
- Range: `-90` to `90`
- Format: Decimal degrees (e.g., `14.6928`)

**Longitude:**
- Type: `number` (float64)
- Range: `-180` to `180`
- Format: Decimal degrees (e.g., `-17.4467`)

**Coordinate Pair Format:**
- Format: `"lat,lon"` (comma-separated string)
- Example: `"14.7167,-17.4677"`
- No spaces allowed
- Used in `from` and `to` parameters

---

### Radius

**Nearby Stops:**
- Type: `integer`
- Range: `0` to `5000` meters
- Default: `500` meters
- Maximum: `5000` meters (5 km)

---

### Limit

**Routes List:**
- Type: `integer`
- Range: `1` to `1000`
- Default: `100`
- Maximum: `1000`

---

## JSON Schema

Full JSON schemas are available in [`/docs/schemas/json-schema/`](../../schemas/json-schema/) for validation and code generation.

## TypeScript Types

Complete TypeScript type definitions are available in [`/docs/schemas/typescript/types.ts`](../../schemas/typescript/types.ts).

---

## See Also

- [API Reference](endpoints.md) - Detailed endpoint documentation
- [Error Reference](errors.md) - Complete error catalog
- [Integration Guide](../../guides/integration-guide.md) - Using these models in your app
- [OpenAPI Specification](../openapi.yaml) - Machine-readable API spec
