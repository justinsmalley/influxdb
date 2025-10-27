#!/usr/bin/env python3
"""
Simple test with dense data
"""
from influxdb import InfluxDBClient
from datetime import datetime, timedelta
import time

client = InfluxDBClient(host='localhost', port=8088, database='testdb')

# Create a simple test database
try:
    client.create_database('testdb_simple')
    client.switch_database('testdb_simple')
except:
    pass

client.switch_database('testdb_simple')

# Insert dense data: every minute for 10 minutes
measurements = []
for i in range(10):
    point = {
        "measurement": "test_measurement",
        "time": (datetime(2025, 10, 20, 12, 0) + timedelta(minutes=i)).isoformat() + "Z",
        "fields": {
            "value": float(i * 10)  # 0, 10, 20, 30, ..., 90
        }
    }
    measurements.append(point)

client.write_points(measurements)

time.sleep(1)

print("Data inserted")
print("=" * 80)

# Test 1: Simple mean
print("\nTest 1: Basic SELECT mean")
print("-" * 80)
try:
    result = client.query("SELECT mean(value) FROM test_measurement WHERE time >= '2025-10-20T12:00:00Z' GROUP BY time(1m)")
    points = list(result.get_points())
    print(f"Number of points: {len(points)}")
    for point in points:
        print(f"  {point}")
except Exception as e:
    print(f"Error: {e}")

# Test 2: Moving average window=3
print("\nTest 2: moving_average(mean(value), 3)")
print("-" * 80)
try:
    result = client.query("SELECT moving_average(mean(value), 3) FROM test_measurement WHERE time >= '2025-10-20T12:00:00Z' GROUP BY time(1m)")
    points = list(result.get_points())
    print(f"Number of points: {len(points)}")
    for point in points:
        print(f"  {point}")
except Exception as e:
    print(f"Error: {e}")

print("\n" + "=" * 80)

