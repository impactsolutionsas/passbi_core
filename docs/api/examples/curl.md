# cURL Examples

Quick reference for testing Pass Bi API with cURL.

## Health Check

```bash
curl http://localhost:8080/health
```

## Route Search

```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

### With Pretty Print (jq)

```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" | jq '.'
```

### Get Only Simple Strategy

```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" | jq '.routes.simple'
```

## Nearby Stops

```bash
curl "http://localhost:8080/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500"
```

## Routes List

```bash
# All routes
curl "http://localhost:8080/v2/routes/list"

# Bus routes only
curl "http://localhost:8080/v2/routes/list?mode=BUS&limit=10"

# Specific agency
curl "http://localhost:8080/v2/routes/list?agency=dakar_dem_dikk"
```

## Error Testing

```bash
# Missing parameters (400)
curl "http://localhost:8080/v2/route-search?from=14.7,-17.4"

# Invalid coordinates (400)
curl "http://localhost:8080/v2/route-search?from=200,300&to=14.6,-17.3"

# No routes found (404)
curl "http://localhost:8080/v2/route-search?from=0,0&to=1,1"
```

## Verbose Output

```bash
curl -v "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467"
```

## Save Response

```bash
curl "http://localhost:8080/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" > route.json
```

