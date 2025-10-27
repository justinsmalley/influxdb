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
    """Generate sensor data with multiple fields, gaps, and outliers."""
    
    measurements = []
    current_time = start_time
    end_time = start_time + timedelta(hours=duration_hours)
    
    # Sensor locations
    locations = ['warehouse_a', 'warehouse_b', 'warehouse_c']
    
    # Dates to skip (create gaps)
    skip_dates = []
    for date_str in ['2025-10-19', '2025-10-22']:
        try:
            skip_dates.append(datetime.strptime(date_str, '%Y-%m-%d').date())
        except ValueError:
            pass
    
    print(f"Generating data from {start_time} to {end_time}")
    print(f"Skipping dates: {skip_dates}")
    
    outlier_counter = 0
    last_outlier_time = None
    
    while current_time < end_time:
        # Skip dates that should have gaps
        if current_time.date() in skip_dates:
            current_time += timedelta(seconds=interval_seconds)
            continue
        
        for location in locations:
            # Check if we need to add an outlier
            add_outlier = False
            if last_outlier_time is None or (current_time - last_outlier_time).total_seconds() >= 21600:  # 6 hours
                add_outlier = True
                last_outlier_time = current_time
                outlier_counter += 1
                print(f"Adding outlier #{outlier_counter} at {current_time}")
            
            # Generate realistic sensor readings
            base_fields = {
                "temperature": round(random.uniform(18.0, 26.0), 2),
                "humidity": round(random.uniform(30.0, 70.0), 2),
                "pressure": round(random.uniform(980.0, 1020.0), 2),
            }
            
            if add_outlier:
                # Add a single field with an outlier value
                base_fields["co2_level"] = random.randint(400, 1000)
                base_fields["light_level"] = random.randint(100, 1000)  # Outlier: 100-1000
            else:
                base_fields["co2_level"] = random.randint(400, 1000)
                base_fields["light_level"] = random.randint(0, 100)  # Normal: 0-100
            
            point = {
                "measurement": "environmental_sensors",
                "tags": {
                    "location": location,
                    "sensor_id": f"sensor_{location}"
                },
                "time": current_time.isoformat() + "Z",
                "fields": base_fields
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
    print(f"Added {outlier_counter} outliers to the data")

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
