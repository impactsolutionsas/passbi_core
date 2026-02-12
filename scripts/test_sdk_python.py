#!/usr/bin/env python3
"""
Test script for PassBi Python SDK
Usage: python scripts/test_sdk_python.py [API_KEY]
"""

import sys
import os
import time
from datetime import datetime

# Add SDK to path
sys.path.insert(0, os.path.join(os.path.dirname(__file__), '..'))

from sdks.python.passbi_client import PassBiClient, PassBiError

# Configuration
API_KEY = sys.argv[1] if len(sys.argv) > 1 else os.getenv('PASSBI_API_KEY')
BASE_URL = os.getenv('API_BASE_URL', 'http://localhost:8080')

# Colors
class Colors:
    RESET = '\033[0m'
    RED = '\033[31m'
    GREEN = '\033[32m'
    YELLOW = '\033[33m'
    BLUE = '\033[34m'

def log(color, message):
    print(f"{color}{message}{Colors.RESET}")

def test_endpoint(name, fn):
    """Test a single endpoint"""
    print(f"{Colors.BLUE}Testing: {name}{Colors.RESET} ... ", end='', flush=True)
    try:
        result = fn()
        log(Colors.GREEN, '✅ PASS')
        return {'name': name, 'success': True, 'result': result}
    except Exception as error:
        log(Colors.RED, f'❌ FAIL')
        print(f"  Error: {error}")
        return {'name': name, 'success': False, 'error': str(error)}

def main():
    print('═══════════════════════════════════════════════════')
    print('🧪 PassBi Python SDK Test Suite')
    print('═══════════════════════════════════════════════════')
    print(f'Base URL: {BASE_URL}')
    print(f'API Key: {API_KEY[:20] + "..." if API_KEY else "NOT PROVIDED"}')
    print()

    if not API_KEY:
        log(Colors.RED, '❌ Error: API key is required')
        print('Usage: python scripts/test_sdk_python.py YOUR_API_KEY')
        print('   or: PASSBI_API_KEY=xxx python scripts/test_sdk_python.py')
        sys.exit(1)

    # Initialize client
    client = PassBiClient(API_KEY, base_url=BASE_URL, debug=False)
    results = []

    # ============================================
    # Core API Tests
    # ============================================
    log(Colors.YELLOW, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━')
    log(Colors.YELLOW, 'CORE API TESTS')
    log(Colors.YELLOW, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n')

    # Test 1: Route search
    def test_route_search():
        routes = client.search_routes('14.7167,-17.4677', '14.6928,-17.4467')
        print(f"    Found {len(routes['routes'])} route strategies")
        return routes

    results.append(test_endpoint('Route search', test_route_search))

    # Test 2: Nearby stops
    def test_nearby_stops():
        stops = client.find_nearby_stops(14.6928, -17.4467, radius=500)
        print(f"    Found {len(stops['stops'])} stops")
        return stops

    results.append(test_endpoint('Find nearby stops', test_nearby_stops))

    # Test 3: List routes
    def test_list_routes():
        routes = client.list_routes(limit=10)
        print(f"    Found {len(routes['routes'])} routes")
        return routes

    results.append(test_endpoint('List routes', test_list_routes))

    # Test 4: Rate limit info
    def test_rate_limit():
        info = client.get_rate_limit_info()
        print(f"    Daily remaining: {info['remaining_day']}/{info['limit_day']}")
        return info

    results.append(test_endpoint('Get rate limit info', test_rate_limit))

    # ============================================
    # Dashboard API Tests
    # ============================================
    log(Colors.YELLOW, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━')
    log(Colors.YELLOW, 'DASHBOARD API TESTS')
    log(Colors.YELLOW, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n')

    # Test 5: Partner info
    def test_partner_info():
        info = client.get_partner_info()
        print(f"    Partner: {info['name']} ({info['tier']})")
        print(f"    Limits: {info['rate_limit_per_day']}/day, {info['rate_limit_per_month']}/month")
        return info

    results.append(test_endpoint('Get partner info', test_partner_info))

    # Test 6: List API keys
    def test_list_keys():
        keys = client.list_api_keys()
        print(f"    Found {keys['total']} API keys")
        return keys

    results.append(test_endpoint('List API keys', test_list_keys))

    # Test 7: Usage stats
    def test_usage_stats():
        usage = client.get_usage_stats(days=7)
        print(f"    Stats for {len(usage['stats'])} days")
        return usage

    results.append(test_endpoint('Get usage stats', test_usage_stats))

    # Test 8: Quota usage
    def test_quota_usage():
        quota = client.get_quota_usage()
        print(f"    Daily: {quota['daily']['requests']}/{quota['daily']['limit']}")
        print(f"    Monthly: {quota['monthly']['requests']}/{quota['monthly']['limit']}")
        return quota

    results.append(test_endpoint('Get quota usage', test_quota_usage))

    # ============================================
    # Error Handling Tests
    # ============================================
    log(Colors.YELLOW, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━')
    log(Colors.YELLOW, 'ERROR HANDLING TESTS')
    log(Colors.YELLOW, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n')

    # Test 9: Invalid coordinates
    def test_invalid_coords():
        try:
            client.search_routes('invalid', 'invalid')
            raise Exception('Should have raised an error')
        except PassBiError as e:
            if e.status_code == 400:
                print(f"    Correctly handled: {e.message}")
                return {'handled': True}
            raise

    results.append(test_endpoint('Invalid coordinates (should fail gracefully)', test_invalid_coords))

    # Test 10: Missing parameters
    def test_missing_params():
        try:
            client.search_routes('', '')
            raise Exception('Should have raised an error')
        except ValueError as e:
            print(f"    Correctly handled: {e}")
            return {'handled': True}

    results.append(test_endpoint('Missing parameters (should fail gracefully)', test_missing_params))

    # ============================================
    # Performance Test
    # ============================================
    log(Colors.YELLOW, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━')
    log(Colors.YELLOW, 'PERFORMANCE TEST')
    log(Colors.YELLOW, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n')

    # Test 11: Response time
    def test_performance():
        iterations = 5
        times = []

        for i in range(iterations):
            start = time.time()
            client.search_routes('14.7167,-17.4677', '14.6928,-17.4467')
            elapsed = (time.time() - start) * 1000  # Convert to ms
            times.append(elapsed)
            time.sleep(0.2)  # Respect rate limits

        avg_time = sum(times) / len(times)
        print(f"    Average response time: {avg_time:.0f}ms")
        print(f"    Min: {min(times):.0f}ms, Max: {max(times):.0f}ms")

        return {'avg_time': avg_time, 'times': times}

    results.append(test_endpoint('Response time test', test_performance))

    # Close client
    client.close()

    # ============================================
    # Summary
    # ============================================
    print('\n═══════════════════════════════════════════════════')
    print('📊 TEST SUMMARY')
    print('═══════════════════════════════════════════════════')

    passed = len([r for r in results if r['success']])
    failed = len([r for r in results if not r['success']])
    total = len(results)

    print(f'Total tests: {total}')
    log(Colors.GREEN, f'Passed: {passed}')
    if failed > 0:
        log(Colors.RED, f'Failed: {failed}')

    if failed > 0:
        print('\nFailed tests:')
        for r in results:
            if not r['success']:
                log(Colors.RED, f"  ❌ {r['name']}: {r['error']}")

    print('\n═══════════════════════════════════════════════════')

    if failed > 0:
        sys.exit(1)

if __name__ == '__main__':
    try:
        main()
    except Exception as error:
        print(f'Fatal error: {error}')
        import traceback
        traceback.print_exc()
        sys.exit(1)
