#!/usr/bin/env python3
"""
Test field mapping operations: rename, delete, recreate.
Compatible with InfluxDB 1.7.11 API
"""

from influxdb import InfluxDBClient
import time

def test_field_operations(client):
    """Test the field mapping functionality."""
    
    print("\n" + "="*60)
    print("Testing Field Operations on Modified InfluxDB")
    print("="*60 + "\n")
    
    # 1. Show initial fields
    print("1. Initial field list:")
    result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
    for point in result.get_points():
        print(f"   - {point['fieldKey']}: {point['fieldType']}")
    
    # 2. Rename temperature to Air_Temperature
    print("\n2. Renaming 'temperature' to 'Air_Temperature'...")
    try:
        result = client.query("ALTER MEASUREMENT environmental_sensors RENAME FIELD temperature TO Air_Temperature")
        print("   ✓ Rename successful")
    except Exception as e:
        print(f"   ✗ Rename failed: {e}")
    
    time.sleep(1)
    
    # 3. Show fields after rename
    print("\n3. Fields after rename:")
    result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
    for point in result.get_points():
        print(f"   - {point['fieldKey']}: {point['fieldType']}")
    
    # 4. Write new data with 'temperature' field (should create temperature.v2)
    print("\n4. Writing new data with 'temperature' field...")
    new_point = [{
        "measurement": "environmental_sensors",
        "tags": {"location": "warehouse_a", "sensor_id": "sensor_warehouse_a"},
        "time": "2024-01-01T00:00:00Z",
        "fields": {"temperature": 25.5}
    }]
    try:
        client.write_points(new_point)
        print("   ✓ Write successful")
    except Exception as e:
        print(f"   ✗ Write failed: {e}")
    
    time.sleep(1)
    
    # 5. Show fields after new write
    print("\n5. Fields after writing new 'temperature':")
    result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
    for point in result.get_points():
        print(f"   - {point['fieldKey']}: {point['fieldType']}")
    
    # 6. Delete Air_Temperature field
    print("\n6. Deleting 'Air_Temperature' field...")
    try:
        result = client.query("DROP FIELD Air_Temperature FROM environmental_sensors")
        print("   ✓ Delete successful")
    except Exception as e:
        print(f"   ✗ Delete failed: {e}")
    
    time.sleep(1)
    
    # 7. Show final fields
    print("\n7. Final field list:")
    result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
    for point in result.get_points():
        print(f"   - {point['fieldKey']}: {point['fieldType']}")
    
    # 8. Query data to verify field mapping works
    print("\n8. Querying recent data:")
    result = client.query("SELECT * FROM environmental_sensors ORDER BY time DESC LIMIT 5")
    for point in result.get_points():
        print(f"   Time: {point['time']}, Location: {point.get('location', 'N/A')}")
        for key, value in point.items():
            if key not in ['time', 'location', 'sensor_id']:
                print(f"      {key}: {value}")

def main():
    print("Connecting to modified InfluxDB on port 8086...")
    client = InfluxDBClient(host='localhost', port=8086, database='testdb')
    
    try:
        test_field_operations(client)
        print("\n" + "="*60)
        print("Test complete!")
        print("="*60)
    except Exception as e:
        print(f"\nError during testing: {e}")

if __name__ == '__main__':
    main()
