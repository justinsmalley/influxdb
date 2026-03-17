# Fork Notes

Development notes and research findings for the InfluxDB 1.7.11 custom fork.
Captured here so the same ground doesn't need to be re-investigated.

---

## Name Validation

### What upstream enforces

Upstream InfluxDB 1.7.11 does **not** enforce per-name length limits on field names,
measurement names, or database names. The only length constraint is:

- `models.MaxKeyLength = 65535` (`models/points.go`) — the combined length of
  measurement name + all tag key=value pairs in a single line-protocol point.

Database and retention policy names are validated by `meta.ValidName()`
(`services/meta/data.go`): non-empty, printable characters, no `/` or `\`.
No length limit.

### What this fork adds (`tsdb/field_validation.go`)

`ValidateFieldName` and `ValidateMeasurementName` both reject names that:
- Are empty
- Contain invalid UTF-8
- Contain null bytes (`\x00`)
- Contain the internal key delimiter (`#!~#`)
- Have leading or trailing whitespace

`ValidateFieldName` additionally rejects reserved names: `_name`, `_tagKey`,
`_tagValue`, `_seriesKey`, `time`.

**No per-name length limit is enforced** — matching upstream behavior.
If you see a `MaxFieldNameLength` constant, it has been removed.

### Decision record

A 255-byte field name limit was briefly added in this fork but was removed after
confirming upstream has no equivalent limit. Adding a limit would be a breaking
change for any existing data with long names.

---

## Mapping File Format

### Background

This fork adds a mapping layer (`tsdb/mapping_store.go` and siblings) to support
renaming and dropping databases, measurements, and fields without rewriting data.

### File format

Mapping files use **JSON** (not protobuf). Protobuf was considered but JSON was
chosen for debuggability — you can inspect and manually repair a mapping file with
any text editor.

| Store | File path |
|-------|-----------|
| Database mappings | `<data-dir>/database_mappings.json` |
| Measurement mappings | `<data-dir>/<db>/measurement_mappings.json` |
| Field mappings | `<data-dir>/<db>/field_mappings.json` |

Each file has a rolling `.bak` backup (one generation). The save flow writes to
`.tmp`, verifies the round-trip, rotates the current file to `.bak`, then renames
`.tmp` to the primary. On load, if the primary is corrupt or missing, the `.bak`
is tried automatically.

### Protobuf types removed

The following protobuf message types were in `tsdb/internal/meta.proto` for the
mapping layer and have been removed (they are no longer generated or used):
`FieldMappingSet`, `FieldMappingMeasurement`, `MeasurementMapping`,
`MeasurementMappingSet`, `DatabaseMapping`, `DatabaseMappingSet`.

The remaining protobuf types (`Series`, `Tag`, `MeasurementFields`, `Field`,
`MeasurementFieldSet`) are still used by the TSM engine for
on-disk shard metadata and must not be removed.
