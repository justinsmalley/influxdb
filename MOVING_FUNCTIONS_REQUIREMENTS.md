# Moving Window Functions - Requirements

## Overview

This document defines the requirements for `moving_average` and `moving_median` functions in InfluxDB 1.7.11+. These functions compute rolling statistics over a time-based window of data points.

## Function Signatures

```sql
moving_average(aggregation_function, window_size[, min_periods[, center]])
moving_median(aggregation_function, window_size[, min_periods[, center]])
```

**Parameters:**
- `aggregation_function`: The aggregated field (e.g., `mean("temperature")`)
- `window_size`: Number of data points in the window (integer)
- `min_periods`: Minimum number of points required to produce output (integer, optional, defaults to `window_size`)
- `center`: Boolean flag for centered windows (optional, defaults to `false`)

## Window Types

### Trailing Window (default, center=false)

The window includes `window_size` points **before and including** the current timestamp.

**Example:**
```sql
SELECT moving_median(mean("temperature"), 20) 
FROM "sensors" 
WHERE time >= '2025-10-20 00:00:00' 
GROUP BY time(1m)
```

- Window size: 20 points
- Interval: 1 minute
- Window span: 19 minutes (to avoid off-by-one: 20 points with 1-minute intervals spans 19 minutes)
- At timestamp `10:20:00`: window includes points from `10:01:00` to `10:20:00` (20 points)

**Query Time Range Padding:**
To produce output at the user's requested start time, the query must internally fetch data starting earlier:
- User requests: `time >= '2025-10-20 00:00:00'`
- Internal fetch: `time >= '2025-10-19 23:41:00'` (19 minutes before)
- This allows the first output at `00:00:00` to have a full 20-point window

### Center Window (center=true)

The window is centered around the current timestamp, with points distributed before and after.

**Example:**
```sql
SELECT moving_median(mean("temperature"), 20, 20, true) 
FROM "sensors" 
WHERE time >= '2025-10-20 00:00:00' 
GROUP BY time(1m)
```

- Window size: 20 points
- Interval: 1 minute
- Window span: 19 minutes total (calculated as `(window_size - 1) × interval`)
- Distribution: `floor((20-1)/2) = 9` minutes before + current point + `ceil((20-1)/2) = 10` minutes after
- At timestamp `10:20:00`: window includes points from `10:11:00` to `10:30:00`

**Query Time Range Padding:**
- User requests: `time >= '2025-10-20 00:00:00' AND time <= '2025-10-23 00:00:00'`
- Internal fetch start: `'2025-10-19 23:51:00'` (9 minutes before start)
- Internal fetch end: `'2025-10-23 00:10:00'` (10 minutes after end)
- Note: Padding split as `startPad = floor((window_size-1)/2) × interval` and `endPad = ceil((window_size-1)/2) × interval`

## Core Behavior Requirements

### 1. Timestamp-Based Window Validation

**The window MUST only include points whose timestamps fall within the valid time range.**

For trailing window at timestamp T:
- Include: points with timestamps in range `[T - (window_size - 1) * interval, T]`
- Exclude: all other points

For center window at timestamp T:
- Include: points with timestamps in range `[T - floor((window_size - 1) / 2) * interval, T + ceil((window_size - 1) / 2) * interval]`
- Exclude: all other points

### 2. Minimum Periods Requirement

Output is produced for a timestamp **only if** the number of valid points in the window meets or exceeds `min_periods`.

**Example:**
```sql
moving_median(mean("temperature"), 20, 15, false)
```
- Window size: 20 points
- Minimum required: 15 points
- If window contains 15-20 points: produce output
- If window contains < 15 points: skip output (no value for that timestamp)

### 3. Gap Handling

**No special gap detection logic is required.** Gaps are handled naturally through timestamp validation:

**Example with gap:**
```
Data: Points every 1 minute
Gap: 10:05 to 10:15 (10-minute gap, missing 10 points)
Query: moving_median(mean("temperature"), 20, 20, false) GROUP BY time(1m)

Timeline:
- 10:04: Window 09:45-10:04 (20 points) → Output ✓
- 10:05: Window 09:46-10:05 (20 points) → Output ✓
- [GAP: no data from 10:06 to 10:14]
- 10:15: Window 09:56-10:15, but missing 10:06-10:14 (only 10 points) → No output ✗
- 10:16: Window 09:57-10:16, but missing 10:06-10:14 (only 11 points) → No output ✗
- ...
- 10:24: Window 10:05-10:24, but missing 10:06-10:14 (only 19 points) → No output ✗
- 10:25: Window 10:06-10:25, but missing 10:06-10:14 (only 19 points) → No output ✗
- 10:26: Window 10:07-10:26 (20 points: 10:07-10:14 missing, but 10:15-10:26 = 12 points, still not enough)
- 10:34: Window 10:15-10:34 (20 points) → Output ✓ (first output after gap)
```

The window naturally "refills" as time progresses past the gap.

### 4. Off-By-One Prevention

**Critical:** Window time span calculation must account for interval boundaries:

- For `window_size = N` points with interval `I`:
  - Time span = `(N - 1) × I`
  - Example: 20 points at 1-minute intervals = 19 minutes
  
- Timestamps are inclusive at both ends:
  - Window from `10:00` to `10:19` with 1-minute interval = 20 points
  - Points at: 10:00, 10:01, 10:02, ..., 10:18, 10:19

## Required Functionality

The following functionality must be implemented:

1. **moving_median function** - Add median calculation capability for Float, Integer, and Unsigned types
2. **minPeriods parameter** - Optional third parameter (defaults to window_size) that allows partial window output when set lower
3. **center parameter** - Optional fourth parameter (defaults to false) that enables centered windows looking before and after current timestamp
4. **Time range padding** - Automatically adjust query time range to ensure full windows can be computed from the start
5. **Timestamp-based window validation** - All buffered points must be validated against the current window's time range

### Original v1.7.11 Problem

The existing `moving_average` in v1.7.11 uses a simple circular buffer based on point count, not timestamps. This causes issues when data has gaps:
- The buffer doesn't track timestamps of points
- After a gap, old pre-gap points contaminate the calculation
- The function incorrectly shows values before gaps are naturally refilled

### Required Fix

All buffered points must have their timestamps validated against the current window's time range:
- For trailing window at time T: Include only points with timestamps in `[T - (window_size-1)×interval, T]`
- For center window at time T: Include only points with timestamps in `[T - floor((window_size-1)/2)×interval, T + ceil((window_size-1)/2)×interval]`
- Points outside the window time range must be completely excluded from the buffer and calculations

## Test Cases

### Test 1: Basic Trailing Window
```sql
-- 20-point window, 1-minute interval, no gaps
-- Should produce output for every timestamp
SELECT moving_median(mean("value"), 20) FROM "test" 
WHERE time >= '2025-10-20 00:00:00' AND time <= '2025-10-20 01:00:00'
GROUP BY time(1m)
```

### Test 2: Gap Handling
```sql
-- Data with gap on 2025-10-22
-- Should produce no output during gap recovery period
SELECT moving_median(mean("value"), 20, 20, false) FROM "test"
WHERE time >= '2025-10-20' AND time <= '2025-10-23'
GROUP BY time(1m)
```

### Test 3: Minimum Periods
```sql
-- Window of 20, minimum of 5
-- Should produce output even with partial windows (if >= 5 points)
SELECT moving_median(mean("value"), 20, 5, false) FROM "test"
WHERE time >= '2025-10-20' AND time <= '2025-10-23'
GROUP BY time(1m)
```

### Test 4: Center Window
```sql
-- Centered 20-point window
-- Should look 9 minutes before and after current timestamp
SELECT moving_median(mean("value"), 20, 20, true) FROM "test"
WHERE time >= '2025-10-20' AND time <= '2025-10-23'
GROUP BY time(1m)
```

## Success Criteria

1. ✅ Window only includes points within the valid time range
2. ✅ Output is skipped when points in window < min_periods
3. ✅ Gaps naturally result in missing output until window refills
4. ✅ No off-by-one errors in window time span calculations
5. ✅ Query time range is properly padded to allow immediate output
6. ✅ Both trailing and center windows work correctly
7. ✅ Both moving_average and moving_median implement the same logic

