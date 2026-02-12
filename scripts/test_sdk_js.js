/**
 * Test script for PassBi JavaScript SDK
 * Usage: node scripts/test_sdk_js.js [API_KEY]
 */

// Load SDK (adjust path as needed)
const { PassBiClient, PassBiError } = require('../sdks/javascript/passbi-client');

// Configuration
const API_KEY = process.argv[2] || process.env.PASSBI_API_KEY;
const BASE_URL = process.env.API_BASE_URL || 'http://localhost:8080';

// Colors for console
const colors = {
    reset: '\x1b[0m',
    red: '\x1b[31m',
    green: '\x1b[32m',
    yellow: '\x1b[33m',
    blue: '\x1b[34m',
};

function log(color, message) {
    console.log(`${color}${message}${colors.reset}`);
}

async function testEndpoint(name, fn) {
    process.stdout.write(`${colors.blue}Testing: ${name}${colors.reset} ... `);
    try {
        const result = await fn();
        log(colors.green, '✅ PASS');
        return { name, success: true, result };
    } catch (error) {
        log(colors.red, `❌ FAIL`);
        console.error(`  Error: ${error.message}`);
        if (error.details) {
            console.error(`  Details:`, error.details);
        }
        return { name, success: false, error: error.message };
    }
}

async function main() {
    console.log('═══════════════════════════════════════════════════');
    console.log('🧪 PassBi JavaScript SDK Test Suite');
    console.log('═══════════════════════════════════════════════════');
    console.log(`Base URL: ${BASE_URL}`);
    console.log(`API Key: ${API_KEY ? API_KEY.substring(0, 20) + '...' : 'NOT PROVIDED'}`);
    console.log('');

    if (!API_KEY) {
        log(colors.red, '❌ Error: API key is required');
        console.log('Usage: node scripts/test_sdk_js.js YOUR_API_KEY');
        console.log('   or: PASSBI_API_KEY=xxx node scripts/test_sdk_js.js');
        process.exit(1);
    }

    // Initialize client
    const client = new PassBiClient(API_KEY, {
        baseURL: BASE_URL,
        debug: false,
    });

    const results = [];

    // ============================================
    // Core API Tests
    // ============================================
    log(colors.yellow, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    log(colors.yellow, 'CORE API TESTS');
    log(colors.yellow, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');

    // Test 1: Route search
    results.push(await testEndpoint('Route search', async () => {
        const routes = await client.searchRoutes({
            from: '14.7167,-17.4677',
            to: '14.6928,-17.4467'
        });
        console.log(`    Found ${Object.keys(routes.routes).length} route strategies`);
        return routes;
    }));

    // Test 2: Nearby stops
    results.push(await testEndpoint('Find nearby stops', async () => {
        const stops = await client.findNearbyStops({
            lat: 14.6928,
            lon: -17.4467,
            radius: 500
        });
        console.log(`    Found ${stops.stops.length} stops`);
        return stops;
    }));

    // Test 3: List routes
    results.push(await testEndpoint('List routes', async () => {
        const routes = await client.listRoutes({ limit: 10 });
        console.log(`    Found ${routes.routes.length} routes`);
        return routes;
    }));

    // Test 4: Rate limit info
    results.push(await testEndpoint('Get rate limit info', async () => {
        const info = client.getRateLimitInfo();
        console.log(`    Daily remaining: ${info.remainingDay}/${info.limitDay}`);
        return info;
    }));

    // ============================================
    // Dashboard API Tests
    // ============================================
    log(colors.yellow, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    log(colors.yellow, 'DASHBOARD API TESTS');
    log(colors.yellow, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');

    // Test 5: Partner info
    results.push(await testEndpoint('Get partner info', async () => {
        const info = await client.getPartnerInfo();
        console.log(`    Partner: ${info.name} (${info.tier})`);
        console.log(`    Limits: ${info.rate_limit_per_day}/day, ${info.rate_limit_per_month}/month`);
        return info;
    }));

    // Test 6: List API keys
    results.push(await testEndpoint('List API keys', async () => {
        const keys = await client.listAPIKeys();
        console.log(`    Found ${keys.total} API keys`);
        return keys;
    }));

    // Test 7: Usage stats
    results.push(await testEndpoint('Get usage stats', async () => {
        const usage = await client.getUsageStats({ days: 7 });
        console.log(`    Stats for ${usage.stats.length} days`);
        return usage;
    }));

    // Test 8: Quota usage
    results.push(await testEndpoint('Get quota usage', async () => {
        const quota = await client.getQuotaUsage();
        console.log(`    Daily: ${quota.daily.requests}/${quota.daily.limit}`);
        console.log(`    Monthly: ${quota.monthly.requests}/${quota.monthly.limit}`);
        return quota;
    }));

    // ============================================
    // Error Handling Tests
    // ============================================
    log(colors.yellow, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    log(colors.yellow, 'ERROR HANDLING TESTS');
    log(colors.yellow, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');

    // Test 9: Invalid coordinates
    results.push(await testEndpoint('Invalid coordinates (should fail gracefully)', async () => {
        try {
            await client.searchRoutes({ from: 'invalid', to: 'invalid' });
            throw new Error('Should have thrown an error');
        } catch (error) {
            if (error instanceof PassBiError && error.statusCode === 400) {
                console.log(`    Correctly handled: ${error.message}`);
                return { handled: true };
            }
            throw error;
        }
    }));

    // Test 10: Missing parameters
    results.push(await testEndpoint('Missing parameters (should fail gracefully)', async () => {
        try {
            await client.searchRoutes({ from: '', to: '' });
            throw new Error('Should have thrown an error');
        } catch (error) {
            if (error.message.includes('required')) {
                console.log(`    Correctly handled: ${error.message}`);
                return { handled: true };
            }
            throw error;
        }
    }));

    // ============================================
    // Performance Test
    // ============================================
    log(colors.yellow, '\n━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━');
    log(colors.yellow, 'PERFORMANCE TEST');
    log(colors.yellow, '━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━\n');

    // Test 11: Response time
    results.push(await testEndpoint('Response time test', async () => {
        const iterations = 5;
        const times = [];

        for (let i = 0; i < iterations; i++) {
            const start = Date.now();
            await client.searchRoutes({
                from: '14.7167,-17.4677',
                to: '14.6928,-17.4467'
            });
            times.push(Date.now() - start);
            await new Promise(resolve => setTimeout(resolve, 200)); // Respect rate limits
        }

        const avgTime = times.reduce((a, b) => a + b, 0) / times.length;
        console.log(`    Average response time: ${avgTime.toFixed(0)}ms`);
        console.log(`    Min: ${Math.min(...times)}ms, Max: ${Math.max(...times)}ms`);

        return { avgTime, times };
    }));

    // ============================================
    // Summary
    // ============================================
    console.log('\n═══════════════════════════════════════════════════');
    console.log('📊 TEST SUMMARY');
    console.log('═══════════════════════════════════════════════════');

    const passed = results.filter(r => r.success).length;
    const failed = results.filter(r => !r.success).length;
    const total = results.length;

    console.log(`Total tests: ${total}`);
    log(colors.green, `Passed: ${passed}`);
    if (failed > 0) {
        log(colors.red, `Failed: ${failed}`);
    }

    console.log('\nFailed tests:');
    results.filter(r => !r.success).forEach(r => {
        log(colors.red, `  ❌ ${r.name}: ${r.error}`);
    });

    console.log('\n═══════════════════════════════════════════════════');

    if (failed > 0) {
        process.exit(1);
    }
}

// Run tests
main().catch(error => {
    console.error('Fatal error:', error);
    process.exit(1);
});
