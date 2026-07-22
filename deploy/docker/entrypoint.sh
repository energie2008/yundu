#!/bin/bash
set -e

echo "=== Starting container ==="

if [ "$MIGRATE_ON_START" = "true" ]; then
    echo "Running database migrations..."
    /app/migrate
    echo "Migrations completed successfully."
fi

if [ -f /app/service ]; then
    echo "Starting service: $@"
    exec /app/service "$@"
else
    echo "No service binary found, running command: $@"
    exec "$@"
fi
