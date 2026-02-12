/**
 * PassBi JavaScript/TypeScript SDK
 * Official client library for the PassBi routing API
 *
 * @version 2.0.0
 * @author PassBi Team
 * @license MIT
 */

class PassBiClient {
    /**
     * Initialize PassBi client
     * @param {string} apiKey - Your API key (pk_live_... or pk_test_...)
     * @param {Object} options - Configuration options
     * @param {string} options.baseURL - Base URL for API (default: https://api.passbi.com)
     * @param {number} options.timeout - Request timeout in milliseconds (default: 30000)
     * @param {boolean} options.debug - Enable debug logging (default: false)
     */
    constructor(apiKey, options = {}) {
        if (!apiKey) {
            throw new Error('API key is required');
        }

        if (!apiKey.startsWith('pk_')) {
            throw new Error('Invalid API key format. API key must start with "pk_"');
        }

        this.apiKey = apiKey;
        this.baseURL = options.baseURL || 'https://api.passbi.com';
        this.timeout = options.timeout || 30000;
        this.debug = options.debug || false;
        this.rateLimitInfo = {};
    }

    /**
     * Make an HTTP request to the API
     * @private
     */
    async _request(method, endpoint, params = null, body = null) {
        const url = new URL(`${this.baseURL}${endpoint}`);

        // Add query parameters
        if (params) {
            Object.keys(params).forEach(key => {
                if (params[key] !== null && params[key] !== undefined) {
                    url.searchParams.append(key, params[key]);
                }
            });
        }

        const headers = {
            'Authorization': `Bearer ${this.apiKey}`,
            'Content-Type': 'application/json',
            'User-Agent': 'PassBi-JS-SDK/2.0.0',
        };

        const options = {
            method,
            headers,
        };

        if (body) {
            options.body = JSON.stringify(body);
        }

        if (this.debug) {
            console.log(`[PassBi] ${method} ${url.toString()}`);
        }

        try {
            const controller = new AbortController();
            const timeoutId = setTimeout(() => controller.abort(), this.timeout);

            const response = await fetch(url.toString(), {
                ...options,
                signal: controller.signal,
            });

            clearTimeout(timeoutId);

            // Store rate limit information from headers
            this._extractRateLimitInfo(response.headers);

            const data = await response.json();

            if (!response.ok) {
                throw new PassBiError(
                    data.message || data.error || 'API request failed',
                    response.status,
                    data.error,
                    data
                );
            }

            return data;
        } catch (error) {
            if (error.name === 'AbortError') {
                throw new PassBiError('Request timeout', 408, 'timeout');
            }
            throw error;
        }
    }

    /**
     * Extract rate limit information from response headers
     * @private
     */
    _extractRateLimitInfo(headers) {
        this.rateLimitInfo = {
            limitSecond: parseInt(headers.get('X-RateLimit-Limit-Second')) || null,
            remainingSecond: parseInt(headers.get('X-RateLimit-Remaining-Second')) || null,
            limitDay: parseInt(headers.get('X-RateLimit-Limit-Day')) || null,
            remainingDay: parseInt(headers.get('X-RateLimit-Remaining-Day')) || null,
            limitMonth: parseInt(headers.get('X-RateLimit-Limit-Month')) || null,
            remainingMonth: parseInt(headers.get('X-RateLimit-Remaining-Month')) || null,
        };
    }

    /**
     * Get current rate limit status
     * @returns {Object} Rate limit information
     */
    getRateLimitInfo() {
        return this.rateLimitInfo;
    }

    /**
     * Search for routes between two locations
     * @param {Object} options - Search options
     * @param {string} options.from - Origin coordinates as "lat,lon"
     * @param {string} options.to - Destination coordinates as "lat,lon"
     * @returns {Promise<Object>} Route results with different strategies
     */
    async searchRoutes({ from, to }) {
        if (!from || !to) {
            throw new Error('Both "from" and "to" coordinates are required');
        }

        return await this._request('GET', '/v2/route-search', { from, to });
    }

    /**
     * Find nearby stops
     * @param {Object} options - Search options
     * @param {number} options.lat - Latitude
     * @param {number} options.lon - Longitude
     * @param {number} options.radius - Search radius in meters (default: 500, max: 5000)
     * @returns {Promise<Object>} Nearby stops
     */
    async findNearbyStops({ lat, lon, radius = 500 }) {
        if (!lat || !lon) {
            throw new Error('Both "lat" and "lon" are required');
        }

        return await this._request('GET', '/v2/stops/nearby', { lat, lon, radius });
    }

    /**
     * List all available routes
     * @param {Object} options - Filter options
     * @param {string} options.mode - Filter by mode (BUS, BRT, TER)
     * @param {string} options.agency - Filter by agency ID
     * @param {number} options.limit - Limit results (default: 100, max: 1000)
     * @returns {Promise<Object>} List of routes
     */
    async listRoutes({ mode, agency, limit = 100 } = {}) {
        return await this._request('GET', '/v2/routes/list', { mode, agency, limit });
    }

    /**
     * Dashboard: Get partner information
     * @returns {Promise<Object>} Partner account information
     */
    async getPartnerInfo() {
        return await this._request('GET', '/dashboard/me');
    }

    /**
     * Dashboard: List all API keys
     * @returns {Promise<Object>} List of API keys
     */
    async listAPIKeys() {
        return await this._request('GET', '/dashboard/api-keys');
    }

    /**
     * Dashboard: Create a new API key
     * @param {Object} options - API key options
     * @param {string} options.name - Name for the API key
     * @param {string} options.description - Description
     * @param {string[]} options.scopes - Permission scopes (default: ['read:routes'])
     * @param {Date} options.expiresAt - Expiration date (optional)
     * @returns {Promise<Object>} Created API key (including the secret key)
     */
    async createAPIKey({ name, description, scopes = ['read:routes'], expiresAt = null }) {
        if (!name) {
            throw new Error('API key name is required');
        }

        return await this._request('POST', '/dashboard/api-keys', null, {
            name,
            description,
            scopes,
            expires_at: expiresAt,
        });
    }

    /**
     * Dashboard: Revoke an API key
     * @param {string} keyId - API key ID to revoke
     * @returns {Promise<Object>} Confirmation message
     */
    async revokeAPIKey(keyId) {
        if (!keyId) {
            throw new Error('API key ID is required');
        }

        return await this._request('DELETE', `/dashboard/api-keys/${keyId}`);
    }

    /**
     * Dashboard: Get usage statistics
     * @param {Object} options - Filter options
     * @param {number} options.days - Number of days to retrieve (default: 30, max: 90)
     * @returns {Promise<Object>} Usage statistics
     */
    async getUsageStats({ days = 30 } = {}) {
        return await this._request('GET', '/dashboard/usage', { days });
    }

    /**
     * Dashboard: Get quota usage
     * @returns {Promise<Object>} Current quota status
     */
    async getQuotaUsage() {
        return await this._request('GET', '/dashboard/quota');
    }

    /**
     * Health check
     * @returns {Promise<Object>} API health status
     */
    async healthCheck() {
        return await this._request('GET', '/health');
    }
}

/**
 * Custom error class for PassBi API errors
 */
class PassBiError extends Error {
    constructor(message, statusCode, errorCode, details = null) {
        super(message);
        this.name = 'PassBiError';
        this.statusCode = statusCode;
        this.errorCode = errorCode;
        this.details = details;
    }

    isRateLimitError() {
        return this.errorCode === 'rate_limit_exceeded' ||
               this.errorCode === 'daily_quota_exceeded' ||
               this.errorCode === 'monthly_quota_exceeded';
    }

    isAuthError() {
        return this.statusCode === 401 || this.statusCode === 403;
    }
}

// Export for Node.js and browser
if (typeof module !== 'undefined' && module.exports) {
    module.exports = { PassBiClient, PassBiError };
}

// Example usage:
/*
const client = new PassBiClient('pk_live_abc123...', {
    debug: true
});

// Search for routes
const routes = await client.searchRoutes({
    from: '14.7167,-17.4677',
    to: '14.6928,-17.4467'
});

console.log('Routes found:', routes.routes);
console.log('Rate limit info:', client.getRateLimitInfo());

// Find nearby stops
const stops = await client.findNearbyStops({
    lat: 14.6928,
    lon: -17.4467,
    radius: 500
});

console.log('Nearby stops:', stops.stops);

// Get usage statistics
const usage = await client.getUsageStats({ days: 30 });
console.log('Usage stats:', usage.stats);
*/
