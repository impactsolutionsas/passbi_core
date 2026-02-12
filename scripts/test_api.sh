#!/bin/bash

# Test script for PassBI Partner API
# Usage: ./scripts/test_api.sh [API_KEY]

set -e

# Configuration
API_BASE_URL="${API_BASE_URL:-http://localhost:8080}"
API_KEY="${1:-}"

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

echo "═══════════════════════════════════════════════════"
echo "🧪 PassBI API Test Suite"
echo "═══════════════════════════════════════════════════"
echo "Base URL: $API_BASE_URL"
echo ""

# Function to test an endpoint
test_endpoint() {
    local name=$1
    local method=$2
    local endpoint=$3
    local auth=$4
    local body=$5

    echo -e "${BLUE}Testing: $name${NC}"
    echo "  $method $endpoint"

    if [ "$auth" == "true" ]; then
        if [ -z "$API_KEY" ]; then
            echo -e "${RED}  ❌ SKIPPED - No API key provided${NC}"
            echo ""
            return
        fi

        if [ -n "$body" ]; then
            response=$(curl -s -w "\n%{http_code}" -X "$method" \
                -H "Authorization: Bearer $API_KEY" \
                -H "Content-Type: application/json" \
                -d "$body" \
                "$API_BASE_URL$endpoint")
        else
            response=$(curl -s -w "\n%{http_code}" -X "$method" \
                -H "Authorization: Bearer $API_KEY" \
                "$API_BASE_URL$endpoint")
        fi
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" "$API_BASE_URL$endpoint")
    fi

    http_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n-1)

    if [ "$http_code" -ge 200 ] && [ "$http_code" -lt 300 ]; then
        echo -e "${GREEN}  ✅ SUCCESS ($http_code)${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    else
        echo -e "${RED}  ❌ FAILED ($http_code)${NC}"
        echo "$body" | jq '.' 2>/dev/null || echo "$body"
    fi
    echo ""
}

# ============================================
# Public Endpoints (No Auth Required)
# ============================================
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}PUBLIC ENDPOINTS${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

test_endpoint "Root endpoint" "GET" "/" "false"
test_endpoint "Health check" "GET" "/health" "false"

# ============================================
# Protected Endpoints (Auth Required)
# ============================================
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}PROTECTED ENDPOINTS${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

if [ -z "$API_KEY" ]; then
    echo -e "${YELLOW}⚠️  No API key provided. Skipping protected endpoints.${NC}"
    echo "   Usage: ./scripts/test_api.sh YOUR_API_KEY"
    echo ""
else
    # Core API endpoints
    test_endpoint "Route search" "GET" \
        "/v2/route-search?from=14.7167,-17.4677&to=14.6928,-17.4467" "true"

    test_endpoint "Nearby stops" "GET" \
        "/v2/stops/nearby?lat=14.6928&lon=-17.4467&radius=500" "true"

    test_endpoint "List routes" "GET" \
        "/v2/routes/list?limit=5" "true"

    # Dashboard endpoints
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}DASHBOARD ENDPOINTS${NC}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""

    test_endpoint "Get partner info" "GET" "/dashboard/me" "true"
    test_endpoint "List API keys" "GET" "/dashboard/api-keys" "true"
    test_endpoint "Get usage stats" "GET" "/dashboard/usage?days=7" "true"
    test_endpoint "Get quota status" "GET" "/dashboard/quota" "true"

    # Rate limit test
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}RATE LIMIT TEST${NC}"
    echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo ""
    echo "Making 5 rapid requests to test rate limiting..."
    for i in {1..5}; do
        echo -n "Request $i: "
        http_code=$(curl -s -o /dev/null -w "%{http_code}" \
            -H "Authorization: Bearer $API_KEY" \
            "$API_BASE_URL/v2/route-search?from=14.7,-17.4&to=14.8,-17.3")

        if [ "$http_code" == "200" ]; then
            echo -e "${GREEN}✅ OK${NC}"
        elif [ "$http_code" == "429" ]; then
            echo -e "${YELLOW}⚠️  Rate limited (expected)${NC}"
        else
            echo -e "${RED}❌ Error ($http_code)${NC}"
        fi
        sleep 0.2
    done
    echo ""
fi

# ============================================
# Error Cases
# ============================================
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}ERROR HANDLING${NC}"
echo -e "${YELLOW}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo ""

test_endpoint "Missing auth header" "GET" "/v2/route-search?from=14.7,-17.4&to=14.8,-17.3" "false"

test_endpoint "Invalid API key" "GET" "/v2/route-search?from=14.7,-17.4&to=14.8,-17.3" "true" "" <<< "API_KEY=invalid_key"

test_endpoint "Missing parameters" "GET" "/v2/route-search" "true"

test_endpoint "Invalid coordinates" "GET" "/v2/route-search?from=invalid&to=invalid" "true"

# ============================================
# Summary
# ============================================
echo "═══════════════════════════════════════════════════"
echo "✅ Test suite completed"
echo "═══════════════════════════════════════════════════"
