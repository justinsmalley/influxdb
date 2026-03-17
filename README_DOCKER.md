# Docker Build & Test Guide

This guide explains how to build, deploy, and test the custom InfluxDB fork using Docker. The custom build replaces the standard InfluxDB 1.7.11 binary with our modified version containing the new features (Field/Measurement Mappings and Advanced Moving Window Functions).

## Prerequisites
- Docker installed and running (Docker Desktop, Colima, or native Linux Docker).
- (Optional) Go 1.13.8 for local compilation. Note: The codebase is strictly pinned to Go 1.13.8 and original dependencies.

## 1. Compiling Locally (Optional)

If you wish to compile the modified `influxd` binary locally on your host machine without Docker, run the following commands from the **root** of the repository:

```bash
cd influxdb
go mod download
CGO_ENABLED=0 go build -ldflags="-s -w" -o influxd ./cmd/influxd
CGO_ENABLED=0 go build -ldflags="-s -w" -o influx ./cmd/influx
```

This generates `influxd` (the server) and `influx` (the CLI) binaries inside the `influxdb` directory.

## 2. Building the Docker Image

Run the following command from the **root** of the repository (where both the `influxdb` and `influxql` directories reside) to build the custom image:

```bash
docker build -f influxdb/Dockerfile.local -t influxdb-custom .
```

This uses a multi-stage Dockerfile (`influxdb/Dockerfile.local`) to compile the modified `influxd` binary and injects it into the official `influxdb:1.7.11` runtime image.

## 3. Testing Strategy

The custom features are tested using a hybrid approach to ensure correctness at both the lowest storage layer and the highest HTTP protocol level.

### Internal Go Unit Tests (Fast, Granular)

These tests run inside a temporary `golang:1.13.8-alpine` container to mimic the compilation environment. They test internal structs, TSM engine functionality, and mathematical properties.

To run the internal unit tests across all directories (this downloads dependencies on the first run):

```bash
docker run --rm -v $(pwd):/src -w /src/influxdb golang:1.13.8-alpine bash -c "apk add --no-cache gcc musl-dev && go mod download && go test ./... -v"
```

### External E2E Python Tests (Production Validation)

We provide a Python test suite that tests the *fully compiled production container* (`influxdb-custom:latest`) from the outside, treating it as a black box. This guarantees the HTTP API, InfluxQL parser, and translation layers work harmoniously.

To run the automated E2E tests:
```bash
./influxdb/e2e_tests/run.sh
```
This script will:
1. Rebuild the `influxdb-custom` image.
2. Spin up a temporary container.
3. Wait for the server to become healthy.
4. Setup a Python `venv` and execute `influxdb/e2e_tests/test_e2e.py` over HTTP port 8086.
5. Cleanup the container automatically.

## 4. Deploying the Container

Start a background container using the newly built image, mapping port 8086 to your host for the HTTP API:

```bash
docker run -d -p 8086:8086 --name influxdb-test influxdb-custom
```

*(Optional)* To view the logs of the running container:
```bash
docker logs -f influxdb-test
```

## 5. Manual Testing

Once the container is running, you can test the new functionality (like measurement renaming) directly via the HTTP API using `curl`.

**Create a test database:**
```bash
curl -i -XPOST http://localhost:8086/query --data-urlencode "q=CREATE DATABASE testdb"
```

**Write some initial data:**
```bash
curl -i -XPOST 'http://localhost:8086/write?db=testdb' --data-binary 'm_original,tag=a temp1=1'
```

**Verify the measurement exists:**
```bash
curl -G http://localhost:8086/query --data-urlencode "db=testdb" --data-urlencode "q=SHOW MEASUREMENTS"
```

**Rename the measurement (Custom Feature):**
```bash
curl -i -XPOST http://localhost:8086/query --data-urlencode "db=testdb" --data-urlencode "q=ALTER MEASUREMENT m_original RENAME TO m_renamed"
```

**Verify the mapping was created:**  
`SHOW MEASUREMENT MAPPINGS` (and `SHOW FIELD MAPPINGS`, `SHOW DATABASE MAPPINGS`) return columns `user_name` and `internal_name` only (plus `measurement` for field mappings); there is no state or version column.
```bash
curl -G http://localhost:8086/query --data-urlencode "db=testdb" --data-urlencode "q=SHOW MEASUREMENT MAPPINGS"
```

**Query the renamed measurement:**
```bash
curl -G http://localhost:8086/query --data-urlencode "db=testdb" --data-urlencode "q=SELECT * FROM m_renamed"
```

**Query using Regex (matches the new user-facing name):**
```bash
curl -G http://localhost:8086/query --data-urlencode "db=testdb" --data-urlencode "q=SELECT * FROM /m_ren.*/"
```

## 6. Cleanup

When you're done testing, you can stop and remove the container to clean up your environment:

```bash
docker rm -f influxdb-test
```
