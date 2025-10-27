#!/usr/bin/env python3
"""
Test script for moving_average and moving_median functions
"""
import sys
from influxdb import InfluxDBClient

def test_moving_functions():
    # Connect to InfluxDB
    client = InfluxDBClient(host='localhost', port=8088, database='testdb')
    
    print("=" * 80)
    print("Testing moving_average and moving_median")
    print("=" * 80)
    
    # Test 1: Basic moving_average with small window
    print("\nTest 1: moving_average(mean(light_level), 5)")
    print("-" * 80)
    try:
        result = client.query(
            "SELECT moving_average(mean(light_level), 5) FROM environmental_sensors "
            "WHERE time >= '2025-10-20T00:00:00Z' AND time <= '2025-10-20T01:00:00Z' "
            "GROUP BY time(1m)"
        )
        print(f"Result: {result}")
        points = list(result.get_points())
        print(f"Number of points: {len(points)}")
        if points:
            for i, point in enumerate(points[:5]):
                print(f"  Point {i}: {point}")
    except Exception as e:
        print(f"Error: {e}")
    
    # Test 2: Basic SELECT without moving functions to verify data exists
    print("\nTest 2: Simple SELECT to verify data exists")
    print("-" * 80)
    try:
        result = client.query(
            "SELECT mean(light_level) FROM environmental_sensors "
            "WHERE time >= '2025-10-20T00:00:00Z' AND time <= '2025-10-20T01:00:00Z' "
            "GROUP BY time(1m) LIMIT 10"
        )
        points = list(result.get_points())
        print(f"Number of points: {len(points)}")
        for i, point in enumerate(points[:5]):
            print(f"  Point {i}: {point}")
    except Exception as e:
        print(f"Error: {e}")
    
    # Test 3: Test with different field
    print("\nTest 3: moving_average on temperature field")
    print("-" * 80)
    try:
        result = client.query(
            "SELECT moving_average(mean(temperature), 5) FROM environmental_sensors "
            "WHERE time >= '2025-10-20T00:00:00Z' AND time <= '2025-10-20T01:00:00Z' "
            "GROUP BY time(1m) LIMIT 10"
        )
        points = list(result.get_points())
        print(f"Number of points: {len(points)}")
        if points:
            for i, point in enumerate(points[:5]):
                print(f"  Point {i}: {point}")
        else:
            print("  No points returned")
    except Exception as e:
        print(f"Error: {e}")
    
    # Test 4: Test that the function name is recognized
    print("\nTest 4: Check if moving_average function exists")
    print("-" * 80)
    try:
        # Try a very simple test query
        result = client.query("SHOW FIELD KEYS FROM environmental_sensors")
        print(f"Fields exist: {list(result.get_points())}")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == '__main__':
    test_moving_functions()

