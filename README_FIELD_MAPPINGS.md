# InfluxDB Field Mapping Implementation

## Overview

This implementation adds field renaming and soft deletion capabilities to InfluxDB 1.7.11, with centralized field mapping management across all shards.

## Key Features

- **Field Renaming**: `ALTER MEASUREMENT <measurement> RENAME FIELD <old_name> TO <new_name>`
- **Soft Field Deletion**: `DROP FIELD <field_name> FROM <measurement>`
- **Show Field Mappings**: `SHOW FIELD MAPPINGS [FROM <measurement>]`
- **Backward Compatible**: Existing data files work without migration
- **Centralized Mapping**: Single source of truth per database

## Architecture

- **Centralized Store**: `FieldMappingStore` manages all field mappings per database
- **Version-based Internal Naming**: Deleted and re-created fields get versioned internal names (e.g., `temperature.v2`)
- **Atomic Operations**: All field operations use writer-preferred RWMutex
- **Backward Compatible**: If using unmodified InfluxDB, deleted fields become visible again

## Compilation

### Prerequisites

- Go 1.10+ (InfluxDB 1.7.11 was originally built with Go 1.10)
- Docker for testing

### Step 1: Compile influxql Library

The `influxql` library must be compiled first as it's a dependency:

```bash
cd /Users/justinsmalley/Codebase/other/influxql
go build ./...
```

This compiles:
- `ast.go` - AST node definitions including `ShowFieldMappingsStatement`
- `parser.go` - Parser including `parseShowFieldMappingsStatement`
- `token.go` - Token definitions including `MAPPINGS`
- `parse_tree.go` - Statement routing including `SHOW FIELD MAPPINGS`

### Step 2: Compile InfluxDB

```bash
cd /Users/justinsmalley/Codebase/other/influxdb

# Compile for Linux (for Docker)
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /tmp/influxd-linux-amd64 ./cmd/influxd

# Or compile for local testing (macOS/Darwin)
go build -ldflags="-s -w" -o influxd ./cmd/influxd
```

**Compilation Flags:**
- `GOOS=linux GOARCH=amd64` - Target Linux x86_64 for Docker
- `CGO_ENнымED=0` - Disable CGO for static binary
- `-ldflags="-s -w"` - Strip debug symbols and reduce binary size

### Step 3: Check for Compilation Errors

If you see errors about `ShowFieldMappingsStatement` or `MAPPINGS`:
1. Ensure `influxql` library is compiled first
2. Check that `go.mod` in influxdb references the local influxql: `replace github.com/influxdata/influxql => ../influxql`

## Docker Testing Setup

### Container Configuration

- **Container Name**: `influxdb-modified`
- **Image**: `influxdb:1.7.11`
- **Port**: `8086`
- **Data Volume**: `/var/lib/influxdb`
- **Database**: `testdb` (for testing)
- **Additional DBs**: `fresh_db` (for fresh tests)

### Step 1: Run Docker Container

```bash
docker run -d \
  --name influxdb-modified \
  -p 8086:8086 \
  -e INFLUXDB_DB=testdb \
  influxdb:1.7.11
```

### Step 2: Copy Compiled Binary

```bash
docker cp /tmp/influxd-linux-amd64 influxdb-modified:/usr/bin/influxd
```

### Step 3: Restart Container

```bash
docker restart influxdb-modified
sleep 10  # Wait for InfluxDB to start
```

### Step 4: Verify Installation

```bash
docker exec influxdb-modified influx -version
# Should show: InfluxDB shell version: 1.7.11
```

## Testing Field Operations

### Test 1: Basic Field Rename

```bash
# Insert data with field 'temperature'
docker exec influxdb-modified influx -database testdb -execute \
  "INSERT environmental_sensors temperature=72.5,humidity=45"

# Rename field
docker exec influxdb-modified influx -database testdb -execute \
  "ALTER MEASUREMENT environmental_sensors RENAME FIELD temperature TO Air_Temperature"

# Verify rename
docker exec influxdb-modified influx -database testdb -execute \
  "SHOW FIELD KEYS FROM environmental_sensors"
# Should show: Air_Temperature (not temperature)

# Query with new name
docker exec influxdb-modified influx -database testdb -execute \
  "SELECT Air_Temperature FROM environmental_sensors"
# Should return data

# Query with old name (should fail or return empty)
docker exec influxdb-modified influx -database testdb -execute \
  "SELECT temperature FROM environmental_sensors"
# Should return empty or no data
```

### Test 2: Soft Field Deletion

```bash
# Insert data with field 'co2_level'
docker exec influxdb-modified influx -database testdb -execute \
  "INSERT environmental_sensors co2_level=400"

# Delete field
docker exec influxdb-modified influx -database testdb -execute \
  "DROP FIELD co2_level FROM environmental_sensors"

# Verify deletion
docker exec influxdb-modified influx -database testdb -execute \
  "SHOW FIELD KEYS FROM environmental_sensors"
# Should NOT show co2_level

# Query with deleted field (should return empty)
docker exec influxdb-modified influx -database testdb -execute \
  "SELECT co2_level FROM environmental_sensors"
# Should return empty

# Show mappings
docker exec influxdb-modified influx -database testdb -execute \
  "SHOW FIELD MAPPINGS FROM environmental_sensors"
# Should show co2_level with state=DELETED, user_name=null
```

### Test 3: Re-create Deleted Field

```bash
# After deleting a field, insert data with same name
docker exec influxdb-modified influx -database testdb -execute \
  "INSERT environmental_sensors co2_level=500"

# Show mappings
docker exec influxdb-modified influx -database testdb -execute \
  "SHOW FIELD MAPPINGS FROM environmental_sensors"
# Should show:
# - co2_level (old) -> DELETED
# - co2_level.v2 (new) -> ACTIVE

# Query should return new data
docker exec influxdb-modified influx -database testdb -execute \
  "SELECT co2_level FROM environmental_sensors"
# Should return data from co2_level.v2
```

### Test 4: SHOW FIELD MAPPINGS

```bash
# Show all mappings
docker exec influxdb-modified influx -database testdb -execute \
  "SHOW FIELD MAPPINGS"

# Show mappings for specific measurement
docker exec influxdb-modified influx -database testdb -execute \
  "SHOW FIELD MAPPINGS FROM environmental_sensors"
```

### Test 5: Complex Scenario

```bash
# Create fresh database for complex test
docker exec influxdb-modified influx -execute "CREATE DATABASE fresh_db"

# Step 1: Insert initial field
docker exec influxdb-modified influx -database fresh_db -execute \
  "INSERT test temperature=100"

# Step 2: Rename field
docker exec influxdb-modified influx -database fresh_db -execute \
  "ALTER MEASUREMENT test RENAME FIELD temperature TO Air_Temperature"

# Step 3: Insert new field with old name
docker exec influxdb-modified influx -database fresh_db -execute \
  "INSERT test temperature=200,other=300"

# Step 4: Delete renamed field
docker exec influxdb-modified influx -database fresh_db -execute \
  "DROP FIELD Air_Temperature FROM test"

# Verify:
docker exec influxdb-modified influx -database fresh_db -execute \
  "SHOW FIELD MAPPINGS FROM test"

# Should show:
# - temperature (internal) -> Air_Temperature (state=DELETED)
# - temperature (internal) -> temperature.v2 (state=ACTIVE)
```

### Test 6: Backward Compatibility

The mapping data is stored in `/var/lib/influxdb/data/<database>/field_mappings`. To test backward compatibility:

```bash
# Stop modified InfluxDB
docker stop influxdb-modified

# Start unmodified InfluxDB
docker run -d \
  --name influxdb-stock \
  -p 8087:8086 \
  -e INFLUXDB_DB=testdb \
  influxdb:1.7.11

# Copy data from modified to stock
docker cp influxdb-modified:/var/lib/influxdb/data/testdb /tmp/
docker cp /tmp/testdb influxdb-stock:/var/lib/influxdb/data/

# Restart stock InfluxDB
docker restart influxdb-stock

# Query - deleted fields should now be visible
docker exec influxdb-stock influx -database testdb -execute \
  "SHOW FIELD KEYS FROM environmental_sensors"
# Should show all fields including deleted ones
```

## File Mapping Structure

### Persistence Files

Field mappings are stored in:
```
/var/lib/influxdb/data/<database>/field_mappings
```

Format: Protobuf `FieldMappingSet` message containing:
- `Measurements[]` - One entry per measurement
  - `Name` - Measurement name
  - `Mappings[]` - Field mappings
    - `UserName` - User-facing name
    - `InternalName` - Internal storage name (may have .v2, .v3 suffix)
    - `Version` - Version number
    - `State` - ACTIVE, DELETED, or RENAMED

### View Raw Mapping Data

```bash
# View raw protobuf data (strings will be visible)
docker exec influxdb-modified cat /var/lib/influxdb/data/testdb/field_mappings | strings

# Or copy and inspect
docker cp influxdb-modified:/var/lib/influxdb/data/testdb/field_mappings /tmp/
cat /tmp/field_mappings | strings
```

## Testing with Grafana

### Setup Grafana

```bash
docker run -d \
  --name grafana \
  -p 3000:3000 \
  -e GF_SECURITY_ADMIN_PASSWORD=admin \
  grafana/grafana:latest

# Connect Grafana to InfluxDB at http://influxdb-modified:8086
# Use "testdb" as the database
```

### Sample Data for Grafana

```bash
# Generate sample sensor data
docker exec influxdb-modified influx -database testdb -execute "
INSERT environmental_sensors,tag1=value1 temperature=72.5,humidity=45,pressure=1013.25
INSERT environmental_sensors,tag1=value1 temperature=73.2,humidity=46,pressure=1013.30
INSERT environmental_sensors,tag1=value1 temperature=73.8,humidity=47,pressure=1013.35
"

# Rename temperature
docker exec influxdb-modified influx -database testdb -execute \
  "ALTER MEASUREMENT environmental_sensors RENAME FIELD temperature TO Air_Temperature"

# Generate more data
docker exec influxdb-modified influx -database testdb -execute "
INSERT environmental_sensors,tag1=value1 Air_Temperature=74.1,humidity=48,pressure=1013.40
"
```

### Grafana Queries

- Panel 1: `SELECT "Air_Temperature" FROM "environmental_sensors" WHERE $timeFilter`
- Panel 2: `SELECT "humidity" FROM "environmental_sensors" WHERE $timeFilter`

## Troubleshooting

### Issue: "field is not active (state: X)"

**Cause**: Trying to rename or delete a field that's already been renamed or deleted.

**Solution**: Check field state with `SHOW FIELD MAPPINGS` and use the correct current name.

### Issue: "database name required"

**Cause**: Database not specified in statement or context.

**Solution**: Use `-database` flag or specify database in query context.

### Issue: Deleted fields still appear

**Cause**: Old code path not filtering deleted fields.

**Solution**: Ensure latest version is deployed and container restarted.

### Issue: Compilation errors about missing types

**Cause**: influxql library not compiled or not in path.

**Solution**: 
1. `cd ../influxql && go build ./...`
2. Check `go.mod` has correct replace directive
3. Run `go mod tidy` in influxdb directory

## Cleanup

```bash
# Stop and remove containers
docker stop influxdb-modified influxdb-stock grafana
docker rm influxdb-modified influxdb-stock grafana

# Remove local binary
rm /tmp/influxd-linux-amd64
```

## Development Workflow

1. Make changes to `influxql` library
2. `cd other/influxql && go build ./...`
3. Make changes to `influxdb`
4. `cd other/influxdb && GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /tmp/influxd-linux-amd64 ./cmd/influxd`
5. `docker cp /tmp/influxd-linux-amd64 influxdb-modified:/usr/bin/influxd`
6. `docker restart influxdb-modified && sleep 10`
7. Test with InfluxQL commands

## Key Files Modified

### influxql Library
- `ast.go` - Added `ShowFieldMappingsStatement` AST node
- `token.go` - Added `MAPPINGS` keyword
- `parser.go` - Added `parseShowFieldMappingsStatement()`
- `parse_tree.go` - Added routing for `SHOW FIELD MAPPINGS`

### influxdb Application
- `tsdb/field_mapping_store.go` - Centralized field mapping store (NEW)
- `tsdb/store.go` - FieldMappingStore management
- `tsdb/shard.go` displaced - Write/read path translation, SHOW FIELD KEYS filtering
- `coordinator/statement_executor.go` - Drop/Rename execution, SHOW command, result translation
- `tsdb/engine/tsm1/engine.go` - Query field name translation
- `tsdb/internal/meta.proto` - Protobuf schema for field mappings
- `tsdb/internal/meta.pb.go` - Generated protobuf code

## References

- InfluxDB 1.7.11 Documentation: https://docs.influxdata.com/influxdb/v1.7/
- InfluxQL Reference: https://docs.influxdata.com/influxdb/v1.7/query_language/
- Go Protobuf: https://github.com/gogo/protobuf

