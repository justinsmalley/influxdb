#!/bin/bash
# Build and tag Docker images for local testing

set -e

echo "Building modified InfluxDB image..."
cd /Users/justinsmalley/Codebase/other
docker build -f influxdb/Dockerfile.local -t influxdb-field-mapping:1.7.11-custom .

echo ""
echo "Tagging image for Docker Desktop..."
docker tag influxdb-field-mapping:1.7.11-custom influxdb-field-mapping:latest

echo ""
echo "Available images:"
docker images | grep -E "influxdb|REPOSITORY"

echo ""
echo "Build complete!"
echo "  - Stock InfluxDB: influxdb:1.7.11"
echo "  - Modified InfluxDB: influxdb-field-mapping:1.7.11-custom"
