import re

files = ['tsdb/database_mapping_store.go', 'tsdb/field_mapping_store.go', 'tsdb/measurement_mapping_store.go']

for file in files:
    with open(file, 'r') as f:
        content = f.read()

    # The issue is that the inherited GenericMappingStore does not expose MarshalAndSave
    # Wait, MarshalAndSave was probably in GenericMappingStore originally.
    # We should add it back to GenericMappingStore, OR just use it here.
    # Let's see what methods were removed. We removed Save() and Load() bodies, and probably MarshalAndSave.

    # Also, ranging over `s.mappings[group]` is now ranging over a `*GroupMappings` struct, not a slice.
    # We need to range over `groupMappings.ByUser` instead.

    # Let's first fix the range over GroupMappings in the Save methods and others
    content = content.replace("for _, mapping := range mappings {", "for _, mapping := range s.GetAllMappings(\"\") {")
    content = content.replace("for _, mapping := range measMappings {", "for _, mapping := range s.GetAllMappings(measurement) {")
    content = content.replace("for _, mapping := range s.mappings[measurement] {", "for _, mapping := range s.GetAllMappings(measurement) {")

    # Fix the `len(mappings)` issues
    content = re.sub(r'len\(mappings\)', 'len(s.GetAllMappings(""))', content)
    content = re.sub(r'len\(measMappings\)', 'len(s.GetAllMappings(measurement))', content)

    with open(file, 'w') as f:
        f.write(content)

print("Patched stores!")
