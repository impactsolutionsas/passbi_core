# Python Examples

Complete examples for integrating PassBi API in Python applications.

## Installation

```bash
pip install requests
# For async examples:
pip install aiohttp
```

## Basic Route Search

```python
import requests

def search_route(from_lat, from_lon, to_lat, to_lon):
    """Search for routes between two coordinates."""
    url = 'http://localhost:8080/v2/route-search'
    params = {
        'from': f'{from_lat},{from_lon}',
        'to': f'{to_lat},{to_lon}'
    }
    
    response = requests.get(url, params=params, timeout=15)
    response.raise_for_status()
    
    return response.json()

# Usage
routes = search_route(14.7167, -17.4677, 14.6928, -17.4467)
print(f"Found {len(routes['routes'])} route options")

# Get recommended route (simple strategy)
recommended = routes['routes'].get('simple')
if recommended:
    duration_min = recommended['duration_seconds'] // 60
    print(f"Duration: {duration_min} minutes")
    print(f"Walking: {recommended['walk_distance_meters']}m")
    print(f"Transfers: {recommended['transfers']}")
```

See full documentation at [/Users/macpro/Desktop/PASSBI-DEVLAND/passbi_core/docs/api/examples/python.md](python.md)
