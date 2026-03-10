#!/bin/bash
set -e

# Constants
CONTAINER_NAME="influxdb-e2e-test"
IMAGE_NAME="influxdb-custom:latest"
PORT=8086

# Ensure we're in the repository root
cd "$(dirname "$0")/../.."

echo "Setting up Python virtual environment..."
python3 -m venv .venv
source .venv/bin/activate

echo "Building Docker image..."
# Build the image using Docker Buildx to avoid legacy builder deprecation warnings
docker buildx build -f influxdb/Dockerfile.local -t "$IMAGE_NAME" --load .

echo "Cleaning up any existing containers..."
docker rm -f "$CONTAINER_NAME" 2>/dev/null || true

echo "Starting InfluxDB container..."
docker run -d -p "$PORT:8086" --name "$CONTAINER_NAME" "$IMAGE_NAME"

# Wait for InfluxDB to start accepting connections
echo "Waiting for InfluxDB to become ready..."
max_attempts=30
attempt=0
while [ $attempt -lt $max_attempts ]; do
    if curl -s -f -o /dev/null "http://localhost:$PORT/ping"; then
        echo "InfluxDB is ready!"
        break
    fi
    echo "Waiting... ($attempt/$max_attempts)"
    sleep 1
    attempt=$((attempt + 1))
done

if [ $attempt -eq $max_attempts ]; then
    echo "Error: InfluxDB failed to start in time."
    docker logs "$CONTAINER_NAME"
    docker rm -f "$CONTAINER_NAME"
    exit 1
fi

echo "Running E2E tests..."
# Run the Python test suite
if python3 influxdb/e2e_tests/test_e2e.py -v; then
    echo "✅ E2E Tests Passed!"
    EXIT_CODE=0
else
    echo "❌ E2E Tests Failed!"
    docker logs "$CONTAINER_NAME"
    EXIT_CODE=1
fi

echo "Cleaning up container..."
docker rm -f "$CONTAINER_NAME"

exit $EXIT_CODE
