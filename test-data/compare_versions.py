#!/usr/bin/env python3
"""
Compare behavior between stock and modified InfluxDB versions.
"""

from influxdb import InfluxDBClient
import time

def test_stock_version():
    """Test operations on stock InfluxDB 1.7.11"""
    print("\n" + "="*60)
    print("Testing Stock InfluxDB 1.7.11 (Port 8087)")
    print("="*60 + "\n")
    
    client = InfluxDBClient(host='localhost', port=8087, database='testdb')
    
    # Create database
    client.create_database('testdb')
    
    # Write test data
    points = [{
        "measurement": "test_measurement",
        "tags": {"location": "lab"},
        "time": "2024-01-01T00:00:00Z",
        "fields": {"temperature": 22.5, "humidity": 45.0}
    }]
    client.write_points(points)
    
    # Show fields
    print("Fields in stock version:")
    result = client.query("SHOW FIELD KEYS FROM test_measurement")
    for point in result.get_points():
        print(f"   - {point['fieldKey']}: {point['fieldType']}")
    
    # Try to rename (should fail in stock version)
    print("\nAttempting field rename (should fail):")
    try:
        client.query("ALTER MEASUREMENT test_measurement RENAME FIELD temperature TO temp")
        print("   ✓ Rename succeeded (unexpected!)")
    except Exception as e:
        print(f"   ✗ Rename failed (expected): {str(e)[:80]}")

def test_modified_version():
    """Test operations on modified InfluxDB"""
    print("\n" + "="*60)
    print("Testing Modified InfluxDB (Port 8086)")
    print("="*60 + "\n")
    
    client = InfluxDBClient(host='localhost', port=8086, database='testdb')
    
    # Show fields
    print("Fields in modified version:")
    result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
    for point in result.get_points():
        print(f"   - {point['fieldKey']}: {point['fieldType']}")
    
    # Try to rename (should succeed in modified version)
    print("\nAttempting field rename (should succeed):")
    try:
        client.query("ALTER MEASUREMENT environmental_sensors RENAME FIELD humidity TO relative_humidity")
        print("   ✓ Rename succeeded")
        
        time.sleep(1)
        
        # Show updated fields
        print("\nFields after rename:")
        result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
        for point in result.get_points():
            print(f"   - {point['fieldKey']}: {point['fieldType']}")
    except Exception as e:
        print(f"   ✗ Rename failed: {e}")

def main():
    print("\n" + "="*60)
    print("InfluxDB Version Comparison Test")
    print("="*60)
    
    test_stock_version()
    test_modified_version()
    
    print("\n" + "="*60)
    print("Comparison complete!")
    print("="*60 + "\n")

if __name__ == '__main__':
    main()
