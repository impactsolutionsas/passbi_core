#!/bin/bash

# PassBI Server Startup Script

echo "🚀 Starting PassBI Server..."
echo ""

# Load environment variables from .env file
if [ -f .env ]; then
    export $(cat .env | grep -v '^#' | xargs)
fi

echo "Configuration:"
echo "  Database: ${DB_USER}@${DB_HOST}:${DB_PORT}/${DB_NAME}"
echo "  Redis: ${REDIS_HOST}:${REDIS_PORT}"
echo "  API Port: ${API_PORT}"
echo "  Auth: ${ENABLE_AUTH}"
echo ""

# Start the server
echo "Starting server on port ${API_PORT}..."
go run cmd/api/main.go
