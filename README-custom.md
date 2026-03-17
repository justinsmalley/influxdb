# InfluxDB Custom Fork

This fork introduces custom enhancements to InfluxDB 1.7.11 to support advanced naming mappings and analytical functions.

## High-Level Changes

1. **Measurement and Field Name Mapping**: Added the ability to rename measurements and fields (`ALTER MEASUREMENT ... RENAME TO ...`, `ALTER MEASUREMENT ... RENAME FIELD ...`) instantly without rewriting underlying data. This uses centralized mapping stores that map user-facing names to internal names (a version suffix in the internal name, e.g. `name.v2`, is used only when a dropped name is re-used).
2. **Database Renaming**: Added the ability to rename databases (`ALTER DATABASE ... RENAME TO ...`) using a similar mapping strategy.
3. **Regex and Wildcard Querying**: Modified the query engine to ensure that regex and wildcard queries match against the *user-facing* names prior to any internal translation.
4. **Advanced Moving Window Functions**: Implemented `moving_average` and `moving_median` window functions in InfluxQL.
5. **Modernized Build Process**: Enforced the use of `docker buildx` for image creation, removing deprecated legacy builders.
6. **Hybrid Testing Strategy**: Established a comprehensive test suite combining Go unit tests with an isolated Python E2E testing framework (`venv`) running against a live Docker container.
7. **Strict Dependency Versioning**: All dependencies must explicitly match InfluxDB 1.7.11's initial constraints. The Go version MUST remain `1.13.8`. Do not rewrite the lock files past their 2020 states.

## Functional Requirements

### 1. Entity Renaming (Databases, Measurements, Fields)
* **Zero-Copy Renaming**: Renaming a database, measurement, or field must not rewrite underlying `tsm1` data. It must use an internal mapping layer.
* **Dense Mapping**: Every field, measurement, and database gets a mapping entry on first write, not just renamed or dropped ones. This is required because collision detection (e.g., assigning `.v2` suffixes when a dropped name is re-used) depends on knowing all occupied internal name slots. For an unrenamed entity, the mapping is an identity entry (e.g., `temp -> temp`).
* **Internal Names**: The mapping layer stores only user name and internal name. When a dropped name is re-used, collision avoidance uses a version suffix in the internal name only (e.g., `user_name.v2`); there is no separate version or state field in memory or on disk. User-chosen names ending in `.v<N>` (e.g., `temp.v1`) are allowed; the system handles collisions transparently by cascading the internal suffix (e.g., `temp.v1.v2`).
* **Query Transparency**: All queries (`SELECT`, `SHOW MEASUREMENTS`, `SHOW FIELD KEYS`) must accept and return user-facing names. The translation to/from internal names must be entirely transparent to the user.
* **Regex Support**: Regular expressions in `WHERE` clauses or `FROM` clauses must apply to the *user-facing* names, expanding correctly to the internal storage names before execution.

### 2. Entity Dropping
* **Native Drops (Databases & Measurements)**: When a database or measurement is explicitly dropped via `DROP DATABASE` or `DROP MEASUREMENT`, its mapping records must be permanently purged from both memory and disk. This allows identical names to be recreated cleanly in the future.
* **Field Drops**: Because native field dropping is not supported in InfluxDB 1.7, dropped fields are represented implicitly: the mapping store clears the user name for that internal name (no separate state). Queries against dropped fields must act as if the field does not exist. Reusing the same user name creates a new mapping, with a version suffix in the internal name if the slot is still reserved.
* **Consistency**: A dropped database must implicitly clean up all measurement and field mappings belonging to it. A dropped measurement must implicitly clean up all field mappings belonging to it.

### 3. Namespace Isolation & Collisions
* **Alias Collision Prevention**: Mathematical function names in queries (e.g., `SELECT mean(f1)`) must not be accidentally translated or renamed if an internal field mapping happens to be named `mean`.
* **Scope**: Field mappings are scoped per-database and per-measurement. Measurement mappings are scoped per-database.

### 4. Advanced Moving Functions
* **`moving_average` and `moving_median`**: Implement standard moving window aggregations natively in InfluxQL.
* **Configurable Windows**: Must support `minPeriods` (to avoid computing aggregations with insufficient data points) and `center` alignment (shifting the result timestamp backward by half the window size).

### 5. Backwards compatibility
Changes must be backwards compatible with unmodified influxdb 1.7.11. This means if we revert the executable binaries to the base published image, but leave the data files, they will work correctly, albeit without the renaming or dropped fields

## Other requirements

### 1. Performance ###
* Renaming and dropping is rare and therefore this operation does not need to be optimized for high speed. It is acceptable to temporarily block reads and writes until the rename/drop is complete. Use existing patterns for dropping where possible.
* Reading and Writing are common, and therefore the mappings must be very fast, and non-blocking between concurrent threads or queries.
* Mapping should be in a fast data structure with O(1) access time for mapping internal name to user name. This may include generating a reverse lookup cache for mapping the user name back to internal name.
* During write batches, new mapping entries are created in-memory without immediate disk writes. A single flush to disk occurs at the end of the batch, reducing N disk writes per batch to at most 1 per mapping store. Field mapping lookups are cached per batch to avoid redundant store queries for the same measurement.
* Each mapping save rotates one rolling `.bak` backup. On load, if the primary file is corrupt, the store falls back to the backup automatically.

### 1a. Startup Reconciliation ###
* On startup, after loading shards, the system reconciles mapping stores with actual data on disk. Any database, measurement, or field that exists in the shard data but is missing from the mapping stores gets an identity mapping created automatically.
* This handles cases where data files were modified externally (e.g., backup restore, offline import tools) without going through the normal write path.
* Existing mappings (including renames) are never modified during reconciliation — only unmapped entities receive new identity entries.
* Reconciliation failures are non-fatal (logged as warnings); mappings will be created on first write anyway.

### 2. Code duplication ###
* Avoid duplicating code. Abstract into functions and classes where possible.

### 3. Smell test ###
* Investigate whether code passess a smell test.

### 4. Testing ###
* Tests must be made for new features. Include tests for basic functionality, as well as tests for edge cases and complex chains of renaming/dropping that imapct multiple layers.

### 4. Scope ###
* Keep the scope of edits to anything newer than branch tag v1.7.11. influxdb f11ad4780c8a61108108a18b141c1d067d920a80
* Remove vestigial code that was introduced after this fork, but whose function has since been removed

---

## See Also

- [README_FIELD_MAPPINGS.md](README_FIELD_MAPPINGS.md) — Field mapping implementation details, behavioral specification, and testing guide
- [README_FORK_NOTES.md](README_FORK_NOTES.md) — Protobuf changes, JSON persistence design, and architectural notes
- [README_DOCKER.md](README_DOCKER.md) — Build workflow and Docker setup
- [README_MOVING_FUNCTIONS.md](README_MOVING_FUNCTIONS.md) — Moving average/median implementation