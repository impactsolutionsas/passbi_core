"""
PassBi Python SDK
Official client library for the PassBi routing API

Version: 2.0.0
Author: PassBi Team
License: MIT
"""

import requests
from typing import Dict, List, Optional, Any
from datetime import datetime
import time


class PassBiClient:
    """
    Official Python client for PassBi API

    Example:
        >>> client = PassBiClient('pk_live_abc123...')
        >>> routes = client.search_routes(from_coords='14.7167,-17.4677', to_coords='14.6928,-17.4467')
        >>> print(routes['routes'])
    """

    def __init__(
        self,
        api_key: str,
        base_url: str = 'https://api.passbi.com',
        timeout: int = 30,
        debug: bool = False
    ):
        """
        Initialize PassBi client

        Args:
            api_key: Your API key (pk_live_... or pk_test_...)
            base_url: Base URL for API (default: https://api.passbi.com)
            timeout: Request timeout in seconds (default: 30)
            debug: Enable debug logging (default: False)

        Raises:
            ValueError: If API key is invalid
        """
        if not api_key:
            raise ValueError('API key is required')

        if not api_key.startswith('pk_'):
            raise ValueError('Invalid API key format. API key must start with "pk_"')

        self.api_key = api_key
        self.base_url = base_url.rstrip('/')
        self.timeout = timeout
        self.debug = debug
        self.rate_limit_info = {}

        self.session = requests.Session()
        self.session.headers.update({
            'Authorization': f'Bearer {api_key}',
            'Content-Type': 'application/json',
            'User-Agent': 'PassBi-Python-SDK/2.0.0',
        })

    def _request(
        self,
        method: str,
        endpoint: str,
        params: Optional[Dict] = None,
        json: Optional[Dict] = None
    ) -> Dict[str, Any]:
        """
        Make an HTTP request to the API

        Args:
            method: HTTP method (GET, POST, DELETE, etc.)
            endpoint: API endpoint path
            params: Query parameters
            json: JSON body

        Returns:
            Response data as dictionary

        Raises:
            PassBiError: If the request fails
        """
        url = f"{self.base_url}{endpoint}"

        if self.debug:
            print(f"[PassBi] {method} {url}")
            if params:
                print(f"[PassBi] Params: {params}")
            if json:
                print(f"[PassBi] Body: {json}")

        try:
            response = self.session.request(
                method=method,
                url=url,
                params=params,
                json=json,
                timeout=self.timeout
            )

            # Extract rate limit information
            self._extract_rate_limit_info(response.headers)

            # Parse response
            data = response.json()

            # Check for errors
            if not response.ok:
                raise PassBiError(
                    message=data.get('message') or data.get('error') or 'API request failed',
                    status_code=response.status_code,
                    error_code=data.get('error'),
                    details=data
                )

            return data

        except requests.exceptions.Timeout:
            raise PassBiError('Request timeout', 408, 'timeout')
        except requests.exceptions.RequestException as e:
            raise PassBiError(f'Request failed: {str(e)}', 0, 'request_failed')

    def _extract_rate_limit_info(self, headers: requests.structures.CaseInsensitiveDict):
        """Extract rate limit information from response headers"""
        self.rate_limit_info = {
            'limit_second': int(headers.get('X-RateLimit-Limit-Second', 0)),
            'remaining_second': int(headers.get('X-RateLimit-Remaining-Second', 0)),
            'limit_day': int(headers.get('X-RateLimit-Limit-Day', 0)),
            'remaining_day': int(headers.get('X-RateLimit-Remaining-Day', 0)),
            'limit_month': int(headers.get('X-RateLimit-Limit-Month', 0)),
            'remaining_month': int(headers.get('X-RateLimit-Remaining-Month', 0)),
        }

    def get_rate_limit_info(self) -> Dict[str, int]:
        """
        Get current rate limit status

        Returns:
            Dictionary with rate limit information
        """
        return self.rate_limit_info

    # ============================================
    # Core API Methods
    # ============================================

    def search_routes(
        self,
        from_coords: str,
        to_coords: str
    ) -> Dict[str, Any]:
        """
        Search for routes between two locations

        Args:
            from_coords: Origin coordinates as "lat,lon"
            to_coords: Destination coordinates as "lat,lon"

        Returns:
            Route results with different strategies

        Example:
            >>> routes = client.search_routes('14.7167,-17.4677', '14.6928,-17.4467')
            >>> print(routes['routes']['direct'])
        """
        if not from_coords or not to_coords:
            raise ValueError('Both from_coords and to_coords are required')

        return self._request('GET', '/v2/route-search', params={
            'from': from_coords,
            'to': to_coords
        })

    def find_nearby_stops(
        self,
        lat: float,
        lon: float,
        radius: int = 500
    ) -> Dict[str, Any]:
        """
        Find nearby stops

        Args:
            lat: Latitude
            lon: Longitude
            radius: Search radius in meters (default: 500, max: 5000)

        Returns:
            Nearby stops

        Example:
            >>> stops = client.find_nearby_stops(14.6928, -17.4467, radius=500)
            >>> for stop in stops['stops']:
            ...     print(stop['name'])
        """
        if lat is None or lon is None:
            raise ValueError('Both lat and lon are required')

        return self._request('GET', '/v2/stops/nearby', params={
            'lat': lat,
            'lon': lon,
            'radius': radius
        })

    def list_routes(
        self,
        mode: Optional[str] = None,
        agency: Optional[str] = None,
        limit: int = 100
    ) -> Dict[str, Any]:
        """
        List all available routes

        Args:
            mode: Filter by mode (BUS, BRT, TER)
            agency: Filter by agency ID
            limit: Limit results (default: 100, max: 1000)

        Returns:
            List of routes

        Example:
            >>> routes = client.list_routes(mode='BUS', limit=10)
            >>> for route in routes['routes']:
            ...     print(route['name'])
        """
        params = {'limit': limit}
        if mode:
            params['mode'] = mode
        if agency:
            params['agency'] = agency

        return self._request('GET', '/v2/routes/list', params=params)

    # ============================================
    # Dashboard API Methods
    # ============================================

    def get_partner_info(self) -> Dict[str, Any]:
        """
        Get partner account information

        Returns:
            Partner information including tier, limits, etc.

        Example:
            >>> info = client.get_partner_info()
            >>> print(f"Tier: {info['tier']}")
            >>> print(f"Daily limit: {info['rate_limit_per_day']}")
        """
        return self._request('GET', '/dashboard/me')

    def list_api_keys(self) -> List[Dict[str, Any]]:
        """
        List all API keys

        Returns:
            List of API keys

        Example:
            >>> keys = client.list_api_keys()
            >>> for key in keys['api_keys']:
            ...     print(f"{key['name']}: {key['key_prefix']}")
        """
        return self._request('GET', '/dashboard/api-keys')

    def create_api_key(
        self,
        name: str,
        description: str = '',
        scopes: List[str] = None,
        expires_at: Optional[datetime] = None
    ) -> Dict[str, Any]:
        """
        Create a new API key

        Args:
            name: Name for the API key
            description: Description
            scopes: Permission scopes (default: ['read:routes'])
            expires_at: Expiration date (optional)

        Returns:
            Created API key information (including the secret key)

        Warning:
            The secret key is only returned once. Save it immediately!

        Example:
            >>> key = client.create_api_key('Production Key', scopes=['read:routes'])
            >>> print(f"API Key: {key['api_key']}")  # Save this!
            >>> print(key['warning'])
        """
        if not name:
            raise ValueError('API key name is required')

        if scopes is None:
            scopes = ['read:routes']

        body = {
            'name': name,
            'description': description,
            'scopes': scopes,
        }

        if expires_at:
            body['expires_at'] = expires_at.isoformat()

        return self._request('POST', '/dashboard/api-keys', json=body)

    def revoke_api_key(self, key_id: str) -> Dict[str, Any]:
        """
        Revoke an API key

        Args:
            key_id: API key ID to revoke

        Returns:
            Confirmation message

        Example:
            >>> result = client.revoke_api_key('key_abc123')
            >>> print(result['message'])
        """
        if not key_id:
            raise ValueError('API key ID is required')

        return self._request('DELETE', f'/dashboard/api-keys/{key_id}')

    def get_usage_stats(self, days: int = 30) -> Dict[str, Any]:
        """
        Get usage statistics

        Args:
            days: Number of days to retrieve (default: 30, max: 90)

        Returns:
            Usage statistics

        Example:
            >>> stats = client.get_usage_stats(days=7)
            >>> for day in stats['stats']:
            ...     print(f"{day['date']}: {day['total_requests']} requests")
        """
        return self._request('GET', '/dashboard/usage', params={'days': days})

    def get_quota_usage(self) -> Dict[str, Any]:
        """
        Get current quota status

        Returns:
            Current quota usage and limits

        Example:
            >>> quota = client.get_quota_usage()
            >>> daily = quota['daily']
            >>> print(f"Daily: {daily['requests']}/{daily['limit']}")
            >>> print(f"Remaining: {daily['remaining']}")
        """
        return self._request('GET', '/dashboard/quota')

    # ============================================
    # Utility Methods
    # ============================================

    def health_check(self) -> Dict[str, Any]:
        """
        Check API health status

        Returns:
            Health status information

        Example:
            >>> health = client.health_check()
            >>> print(health['status'])
        """
        return self._request('GET', '/health')

    def close(self):
        """Close the HTTP session"""
        self.session.close()

    def __enter__(self):
        """Context manager entry"""
        return self

    def __exit__(self, exc_type, exc_val, exc_tb):
        """Context manager exit"""
        self.close()


class PassBiError(Exception):
    """
    Custom exception for PassBi API errors

    Attributes:
        message: Error message
        status_code: HTTP status code
        error_code: API error code
        details: Additional error details
    """

    def __init__(
        self,
        message: str,
        status_code: int = 0,
        error_code: Optional[str] = None,
        details: Optional[Dict] = None
    ):
        super().__init__(message)
        self.message = message
        self.status_code = status_code
        self.error_code = error_code
        self.details = details

    def is_rate_limit_error(self) -> bool:
        """Check if this is a rate limit error"""
        return self.error_code in [
            'rate_limit_exceeded',
            'daily_quota_exceeded',
            'monthly_quota_exceeded'
        ]

    def is_auth_error(self) -> bool:
        """Check if this is an authentication error"""
        return self.status_code in [401, 403]

    def __str__(self):
        if self.error_code:
            return f"[{self.error_code}] {self.message}"
        return self.message


# Example usage
if __name__ == '__main__':
    # Initialize client
    client = PassBiClient('pk_live_abc123...', debug=True)

    try:
        # Search for routes
        routes = client.search_routes(
            from_coords='14.7167,-17.4677',
            to_coords='14.6928,-17.4467'
        )
        print('Routes found:', routes['routes'].keys())

        # Get rate limit info
        rate_info = client.get_rate_limit_info()
        print(f"Remaining today: {rate_info['remaining_day']}")

        # Find nearby stops
        stops = client.find_nearby_stops(lat=14.6928, lon=-17.4467, radius=500)
        print(f"Found {len(stops['stops'])} nearby stops")

        # Get usage statistics
        usage = client.get_usage_stats(days=7)
        print('Usage stats:', usage['stats'])

    except PassBiError as e:
        print(f"Error: {e}")
        if e.is_rate_limit_error():
            print("Rate limit exceeded!")

    finally:
        client.close()
