#!/usr/bin/env python3
"""
Generate realistic time-series sensor data for testing InfluxDB field operations.
Compatible with InfluxDB 1.7.11
"""

import random
import time
from datetime import datetime, timedelta
from influxdb import InfluxDBClient

def generate_sensor_data(client, start_time, duration_hours=24, interval_seconds=60):
    """Generate sensor data with multiple fields."""
    
    measurements = []
    current_time = start_time
    end_time = start_time + timedelta(hours=duration_hours)
    
    # Sensor locations
    locations = ['warehouse_a', 'warehouse_b', 'warehouse_c']
    
    print(f"Generating data from {start_time} to {end_time}")
    
    while current_time < end_time:
        for location in locations:
            # Generate realistic sensor readings
            point = {
                "measurement": "environmental_sensors",
                "tags": {
                    "location": location,
                    "sensor_id": f"sensor_{location}"
                },
                "time": current_time.isoformat() + "Z",
                "fields": {
                    "temperature": round(random.uniform(18.0, 26.0), 2),
                    "humidity": round(random.uniform(30.0, 70.0), 2),
                    "pressure": round(random.uniform(980.0, 1020.0), 2),
                    "co2_level": random.randint(400, 1000),
                    "light_level": random.randint(0, 1000),
                }
            }
            measurements.append(point)
        
        current_time += timedelta(seconds=interval_seconds)
    
    # Write in batches
    batch_size = 1000
    for i in range(0, len(measurements), batch_size):
        batch = measurements[i:i+batch_size]
        client.write_points(batch)
        print(f"Wrote batch {i//batch_size + 1}/{(len(measurements)-1)//batch_size + 1}")
    
    print(f"Generated {len(measurements)} data points")

def main():
    # Connect to modified InfluxDB
    print("Connecting to InfluxDB (modified version on port 8088)...")
    client = InfluxDBClient(host='localhost', port=8088, database='testdb')
    
    # Create database
    print("Creating database 'testdb'...")
    client.create_database('testdb')
    
    # Generate data for the last 7 days
    start_time = datetime.now() - timedelta(days=7)
    generate_sensor_data(client, start_time, duration_hours=168, interval_seconds=300)
    
    print("\nData generation complete!")
    print("You can now test field operations on this data.")

if __name__ == '__main__':
    main()
