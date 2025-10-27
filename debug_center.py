#!/usr/bin/env python3
"""
Debug center window with minPeriods
"""
import requests
import json
from datetime import datetime

url = "http://localhost:8088/query"

# Test with center=true and different minPeriods values
for min_periods in [5, 10, 11, 15, 20]:
    query = f'SELECT moving_median(mean("light_level"), 20, {min_periods}, true) FROM "environmental_sensors" WHERE time >= \'2025-10-20T00:00:00Z\' AND time <= \'2025-10-20T04:00:00Z\' GROUP BY time(2m)'
    
    params = {
        "db": "testdb",
        "q": query
    }
    
    response = requests.get(url, params=params)
    data = response.json()
    
    if data.get("results", [{}])[0].get("series"):
        points = len(data["results"][0]["series"][0]["values"])
        print(f"minPeriods={min_periods}: {points} results")
    else:
        print(f"minPeriods={min_periods}: No results")

