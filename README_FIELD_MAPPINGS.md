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
- **Dense Mapping**: Every field, measurement, and database gets a mapping entry on first write, not just renamed or dropped ones. This is required because collision detection (e.g., assigning `.v2` suffixes) depends on knowing all occupied internal name slots. For an unrenamed field, the mapping is an identity entry (`temp -> temp`). The same dense approach applies to measurement and database mapping stores.
- **Internal naming**: Each mapping is (user name, internal name) only. When a dropped name is re-used, collision avoidance uses a version suffix in the internal name (e.g., `temperature.v2`); there is no separate version or state in the store or on disk. Users are free to create fields whose names end in `.v<N>` (e.g., `temp.v1`); the system handles this transparently by cascading the suffix (e.g., internal name becomes `temp.v1.v2` if needed).
- **Batched Saves**: During write batches, new mapping entries are created in-memory without immediate disk writes. A single flush to disk occurs at the end of the batch, reducing N disk writes per batch to at most 1 per mapping store.
- **Backup-on-Save**: Each save rotates one rolling `.bak` backup. On load, if the primary file is corrupt, the store falls back to the backup automatically.
- **Atomic Operations**: All field operations use writer-preferred RWMutex
- **Backward Compatible**: If using unmodified InfluxDB, deleted fields become visible again

## Compilation

See **[README_DOCKER.md](README_DOCKER.md)** for the full build workflow.

Quick reference (from repo root):

```bash
# Compile influxql dependency
cd influxql && go build ./... && cd ..

# Compile influxdb (local macOS binary)
cd influxdb
go mod download
CGO_ENABLED=0 go build -ldflags="-s -w" -o influxd ./cmd/influxd

# Compile for Linux/Docker
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -ldflags="-s -w" -o /tmp/influxd-linux-amd64 ./cmd/influxd
```

## Docker Testing

See **[README_DOCKER.md](README_DOCKER.md)** for the current build and testing workflow.
The recommended approach uses `docker build -f influxdb/Dockerfile.local` rather than
manually copying binaries into a running container.

## Behavioral Specification

All scenarios are covered by the E2E test suite (`e2e_tests/test_e2e.py`). The **Core Principle** governing all renames: `RENAME <old_name> TO <new_name>` will **FAIL** if `<new_name>` is currently an active user name. If `<new_name>` is inactive (previously dropped or renamed away), the operation will **SUCCEED** and the name is reclaimed.

### Scenario 1: Swapping Names via a Temporary Name
*Tested in: `test_06_complex_field_renames`*

**Initial State:**
* Internal Name `a` -> User Name `a` (Active)
* Internal Name `b` -> User Name `b` (Active)

**`RENAME FIELD a TO b`** — **FAILS**. User Name `b` is currently active.

**`RENAME FIELD a TO tmp`** — **SUCCESS**.
* Internal Name `a` -> User Name `tmp` (Active)
* Internal Name `b` -> User Name `b` (Active)

**`RENAME FIELD b TO a`** — **SUCCESS** (User Name `a` is now inactive).
* Internal Name `a` -> User Name `tmp` (Active)
* Internal Name `b` -> User Name `a` (Active)

**`RENAME FIELD tmp TO b`** — **SUCCESS** (User Name `b` is now inactive).
* Internal Name `a` -> User Name `b` (Active)
* Internal Name `b` -> User Name `a` (Active)

---

### Scenario 2: Reclaiming a Dropped Name
*Tested in: `test_04_complex_measurement_renames` and `test_07_mapping_deletion_resolution`*

**Initial State:** Internal Name `old_sensor` -> User Name `old_sensor` (Active)

**`DROP FIELD old_sensor`** — **SUCCESS**. Internal Name `old_sensor` has no current user name (slot reusable with `.v2` suffix).

**Write new data to `old_sensor`** — **SUCCESS**.
* Internal Name `old_sensor` -> no current user name
* Internal Name `old_sensor.v2` -> User Name `old_sensor` (Active)

*If `old_sensor.v2` is later dropped and the name reused, the next internal name becomes `old_sensor.v3`, etc.*

---

### Scenario 3: Chain Renaming
*Tested in: `test_04_complex_measurement_renames` and `test_06_complex_field_renames`*

**Initial State:** Internal Name `temp` -> User Name `temp` (Active)

**`RENAME FIELD temp TO temperature`** → **`RENAME FIELD temperature TO heat`** — **SUCCESS**. Result: Internal Name `temp` -> User Name `heat` (Active).

**`SELECT heat FROM sensors`** → returns data (resolves `heat` → internal `temp`).
**`SELECT temp FROM sensors`** → no data (old user name is inactive).

---

### Scenario 4: Consolidating/Merging (Intentional Failure)
*Tested in: `test_04_complex_measurement_renames` and `test_06_complex_field_renames`*

**`RENAME FIELD status_code TO statusCode`** — **FAILS** when `statusCode` is already Active. Renames are 1:1 mapping updates and cannot merge physical data.

---

### Scenario 5: Regex Queries with Mappings
*Tested in: `test_08_regex_with_multiple_mappings`*

**Initial State:**
* Internal Name `field_1` -> User Name `other_field` (Active)
* Internal Name `field_2` -> User Name `field_2` (Active)

**`SELECT /field_.*/ FROM sensors`** → matches only `field_2`. Internal name `field_1` is ignored because its active User Name `other_field` does not match the regex.

**`SELECT /other_.*/ FROM sensors`** → matches `other_field` successfully.

---

### Scenario 6: Database Renaming & Cache Clearing
*Tested in: `test_09_mapping_cache_clear_on_drop_db` and `test_10_database_renaming`*

**`ALTER DATABASE old_db RENAME TO new_db`** — **SUCCESS**. Queries to `old_db` return "database not found"; all traffic uses `new_db`.

**`DROP DATABASE new_db`** — **SUCCESS**. Entirely evicts the mapping cache and data for `old_db`/`new_db`; if `old_db` is recreated, it starts with a clean slate.

**`DROP MEASUREMENT some_measurement`** — **SUCCESS**. Drops the measurement mapping and deletes all associated field mappings from memory and disk.

---

### Scenario 7: Field Ops on a Renamed Measurement
*Tested in: `test_12_field_ops_on_renamed_measurement`*

After `ALTER MEASUREMENT m1 RENAME TO m2`, all subsequent field operations (`RENAME FIELD`, `DROP FIELD`) against `m2` correctly resolve `m2` → internal `m1` to locate and modify the field mappings.

---

### Scenario 8: Dropping a Renamed Measurement Clears Field Mappings
*Tested in: `test_13_drop_renamed_measurement_clears_fields`*

After `ALTER MEASUREMENT m1 RENAME TO m2`, a `DROP MEASUREMENT m2` resolves `m2` → internal `m1` and completely deletes all field mappings for `m1` from cache and disk, ensuring a clean slate if `m1` is recreated.

---

### Scenario 9: Renamed Names Stay Masked
*Tested in: `test_14_garbage_collection_protects_historical_names`*

After `RENAME FIELD f_old TO f_new`, only the active pair `(f_new, internal_f_old)` is stored and persisted. A query for `f_old` returns no data; a query for `f_new` correctly resolves to the internal name.

---

### Scenario 10: Measurement Swap Preserves Independent Fields
*Tested in: `test_15_measurement_swap_preserves_fields`*

Swapping two measurement names via a temporary preserves each measurement's independent field dictionary. After the swap, querying `m1` returns fields that belonged to the original `m2` and vice versa — no field cross-pollution.

---

### Scenario 11: Recreating a Dropped Measurement Clears Field History
*Tested in: `test_16_drop_recreate_measurement_clears_field_history`*

After `DROP MEASUREMENT m_recreate`, all mapping history is wiped. Writing a new point creates a new internal measurement (`m_recreate.v2`) with a completely clean field mapping — no history from the old incarnation is inherited.

---

### Scenario 12: Multi-Layer Chained Renames (Database → Measurement → Field)
*Tested in: `test_17_full_chain_db_meas_field_rename`*

After renaming `db_old → db_new`, `m_old → m_new`, and `f_old → f_new` in sequence, a query `SELECT f_new FROM m_new` against `db_new` correctly resolves through all three mapping layers to the internal storage path `db_old / m_old / f_old`.

---

## Backward Compatibility Testing

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

Field mappings are stored as **JSON** files (not protobuf). The JSON format was chosen
for debuggability — files can be inspected and manually repaired with any text editor.

| Store | File path |
|-------|-----------|
| Database mappings | `<data-dir>/database_mappings.json` |
| Measurement mappings | `<data-dir>/<db>/measurement_mappings.json` |
| Field mappings | `<data-dir>/<db>/field_mappings.json` |

Each file has a rolling `.bak` backup (one generation). The save flow writes to `.tmp`,
verifies the round-trip, rotates the current file to `.bak`, then renames `.tmp` to the
primary. On load, if the primary is corrupt or missing, the `.bak` is tried automatically.

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

### Issue: Rename or drop fails (name not found or already in use)

**Cause**: The name may have been renamed or dropped already; renames require the current user-facing name.

**Solution**: Use `SHOW FIELD MAPPINGS` to see current (user_name, internal_name) pairs and use the correct user-facing name.

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

See **[README_DOCKER.md](README_DOCKER.md)** for the current end-to-end build and test workflow.

## Key Files Modified

### influxql Library
- `ast.go` - Added `ShowFieldMappingsStatement` AST node
- `token.go` - Added `MAPPINGS` keyword
- `parser.go` - Added `parseShowFieldMappingsStatement()`
- `parse_tree.go` - Added routing for `SHOW FIELD MAPPINGS`

### influxdb Application
- `tsdb/field_mapping_store.go` - Centralized field mapping store (NEW)
- `tsdb/store.go` - FieldMappingStore management
- `tsdb/shard.go` - Write/read path translation, SHOW FIELD KEYS filtering
- `coordinator/statement_executor.go` - Drop/Rename execution, SHOW command, result translation
- `tsdb/engine/tsm1/engine.go` - Query field name translation

## References

- InfluxDB 1.7.11 Documentation: https://docs.influxdata.com/influxdb/v1.7/
- InfluxQL Reference: https://docs.influxdata.com/influxdb/v1.7/query_language/
- Go Protobuf: https://github.com/gogo/protobuf

