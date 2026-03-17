package query

import (
	"testing"
	"time"

	"github.com/influxdata/influxql"
)

func TestCalculateMovingAverageFloat_ZeroValidCount(t *testing.T) {
	// When minPeriods=0 and there are no valid values, the function must not
	// divide by zero. It should return an empty result.
	result, count := calculateMovingAverageFloat(1000, 5, 0, 0, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestCalculateMovingAverageInt_ZeroValidCount(t *testing.T) {
	result, count := calculateMovingAverageInt(1000, 5, 0, 0, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestCalculateMovingAverageUint_ZeroValidCount(t *testing.T) {
	result, count := calculateMovingAverageUint(1000, 5, 0, 0, nil, nil)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestCalculateMovingAverageFloat_MinPeriodsOne(t *testing.T) {
	// With minPeriods=1 and one value, should emit.
	timeBuf := []int64{100}
	valueBuf := []float64{42.0}
	result, count := calculateMovingAverageFloat(100, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 || result[0] != 42.0 {
		t.Fatalf("expected [42.0], got %v", result)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestCalculateMovingAverageFloat_MinPeriodsExceedsData(t *testing.T) {
	// With minPeriods=10 and only 3 values, should not emit.
	timeBuf := []int64{100, 200, 300}
	valueBuf := []float64{1.0, 2.0, 3.0}
	result, count := calculateMovingAverageFloat(300, 5, 10, 0, timeBuf, valueBuf)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestCalculateMovingAverageFloat_RTimeZero(t *testing.T) {
	// rTime=0 should short-circuit to empty.
	result, count := calculateMovingAverageFloat(0, 5, 1, 0, []int64{100}, []float64{42.0})
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
	if count != 0 {
		t.Fatalf("expected count 0, got %d", count)
	}
}

func TestCalculateMovingAverageFloat_WithInterval(t *testing.T) {
	// Test with time-based windowing
	interval := time.Second
	timeBuf := []int64{
		1000000000, // 1s
		2000000000, // 2s
		3000000000, // 3s
		4000000000, // 4s
		5000000000, // 5s
	}
	valueBuf := []float64{10.0, 20.0, 30.0, 40.0, 50.0}

	// Window of 3, rTime at 5s: valid window is [3s, 5s] → values 30, 40, 50
	result, count := calculateMovingAverageFloat(5000000000, 3, 1, interval, timeBuf, valueBuf)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	expected := 40.0 // (30+40+50)/3
	if result[0] != expected {
		t.Fatalf("expected %f, got %f", expected, result[0])
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestCalculateMovingMedianFloat_MinPeriodsOne(t *testing.T) {
	timeBuf := []int64{100}
	valueBuf := []float64{42.0}
	result, count := calculateMovingMedianFloat(100, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 || result[0] != 42.0 {
		t.Fatalf("expected [42.0], got %v", result)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestCalculateMovingMedianFloat_EvenCount(t *testing.T) {
	timeBuf := []int64{100, 200, 300, 400}
	valueBuf := []float64{1.0, 3.0, 5.0, 7.0}
	result, count := calculateMovingMedianFloat(400, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Median of [1,3,5,7] = 3 + (5-3)/2 = 4.0
	if result[0] != 4.0 {
		t.Fatalf("expected 4.0, got %f", result[0])
	}
	if count != 4 {
		t.Fatalf("expected count 4, got %d", count)
	}
}

func TestCalculateMovingMedianFloat_OddCount(t *testing.T) {
	timeBuf := []int64{100, 200, 300}
	valueBuf := []float64{1.0, 5.0, 3.0}
	result, count := calculateMovingMedianFloat(300, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Sorted: [1,3,5], median = 3.0
	if result[0] != 3.0 {
		t.Fatalf("expected 3.0, got %f", result[0])
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

// Test-2: filterValidIndices gap handling — sparse data with timestamps outside the window.

func TestFilterValidIndices_OnlyWindowedTimestamps(t *testing.T) {
	// Window size=3, interval=1s. At rTime=5s the valid range is [3s, 5s].
	// timeBuf has points at 1s, 2s, 3s, 4s, 5s; only the last three are valid.
	interval := time.Second
	timeBuf := []int64{
		1_000_000_000, // outside window
		2_000_000_000, // outside window
		3_000_000_000, // inside (boundary)
		4_000_000_000, // inside
		5_000_000_000, // inside (== rTime)
	}
	rTime := int64(5_000_000_000)
	indices := filterValidIndices(rTime, 3, interval, timeBuf)
	if len(indices) != 3 {
		t.Fatalf("expected 3 valid indices, got %d: %v", len(indices), indices)
	}
	expected := []int{2, 3, 4}
	for i, idx := range indices {
		if idx != expected[i] {
			t.Errorf("index[%d]: expected %d, got %d", i, expected[i], idx)
		}
	}
}

func TestFilterValidIndices_MinPeriodsSuppressesGappedWindow(t *testing.T) {
	// Sparse data: only 2 points fall in the window but minPeriods=3 → no output.
	interval := time.Second
	timeBuf := []int64{
		1_000_000_000, // outside [3s,7s]
		2_000_000_000, // outside
		6_000_000_000, // inside
		7_000_000_000, // inside (== rTime)
	}
	valueBuf := []float64{10.0, 20.0, 30.0, 40.0}
	rTime := int64(7_000_000_000)

	result, count := calculateMovingAverageFloat(rTime, 5, 3, interval, timeBuf, valueBuf)
	if len(result) != 0 {
		t.Fatalf("expected no output (only 2 valid points < minPeriods 3), got %v", result)
	}
	if count != 2 {
		t.Fatalf("expected count=2, got %d", count)
	}
}

func TestFilterValidIndices_AllOutsideWindow(t *testing.T) {
	// No point falls within the window at all.
	interval := time.Second
	timeBuf := []int64{1_000_000_000, 2_000_000_000}
	rTime := int64(10_000_000_000) // window [8s,10s], none qualify
	indices := filterValidIndices(rTime, 3, interval, timeBuf)
	if len(indices) != 0 {
		t.Fatalf("expected 0 valid indices, got %d", len(indices))
	}
}

func TestFilterValidIndices_ZeroIntervalAcceptsAll(t *testing.T) {
	// interval=0 means no time-based filtering; all indices are returned.
	timeBuf := []int64{100, 200, 300, 400, 500}
	indices := filterValidIndices(500, 3, 0, timeBuf)
	if len(indices) != len(timeBuf) {
		t.Fatalf("expected all %d indices with interval=0, got %d", len(timeBuf), len(indices))
	}
}

// Test-6: moving median unit tests for Int and Unsigned types.

func TestCalculateMovingMedianInt_SingleValue(t *testing.T) {
	timeBuf := []int64{100}
	valueBuf := []int64{7}
	result, count := calculateMovingMedianInt(100, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 || result[0] != 7.0 {
		t.Fatalf("expected [7.0], got %v", result)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestCalculateMovingMedianInt_EvenCount(t *testing.T) {
	timeBuf := []int64{100, 200, 300, 400}
	valueBuf := []int64{1, 3, 5, 7}
	result, count := calculateMovingMedianInt(400, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Median of [1,3,5,7] = 4.0
	if result[0] != 4.0 {
		t.Fatalf("expected 4.0, got %f", result[0])
	}
	if count != 4 {
		t.Fatalf("expected count 4, got %d", count)
	}
}

func TestCalculateMovingMedianInt_MinPeriodsBoundary(t *testing.T) {
	// 3 points in buffer, minPeriods=3: should emit.
	timeBuf := []int64{100, 200, 300}
	valueBuf := []int64{2, 4, 6}
	result, count := calculateMovingMedianInt(300, 5, 3, 0, timeBuf, valueBuf)
	if len(result) != 1 || result[0] != 4.0 {
		t.Fatalf("expected [4.0], got %v", result)
	}
	if count != 3 {
		t.Fatalf("expected count 3, got %d", count)
	}
}

func TestCalculateMovingMedianUint_SingleValue(t *testing.T) {
	timeBuf := []int64{100}
	valueBuf := []uint64{42}
	result, count := calculateMovingMedianUint(100, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 || result[0] != 42.0 {
		t.Fatalf("expected [42.0], got %v", result)
	}
	if count != 1 {
		t.Fatalf("expected count 1, got %d", count)
	}
}

func TestCalculateMovingMedianUint_EvenCount(t *testing.T) {
	timeBuf := []int64{100, 200, 300, 400}
	valueBuf := []uint64{2, 4, 6, 8}
	result, count := calculateMovingMedianUint(400, 5, 1, 0, timeBuf, valueBuf)
	if len(result) != 1 {
		t.Fatalf("expected 1 result, got %d", len(result))
	}
	// Median of [2,4,6,8] = 5.0
	if result[0] != 5.0 {
		t.Fatalf("expected 5.0, got %f", result[0])
	}
	if count != 4 {
		t.Fatalf("expected count 4, got %d", count)
	}
}

func TestCalculateMovingMedianUint_MinPeriodsNotMet(t *testing.T) {
	timeBuf := []int64{100, 200}
	valueBuf := []uint64{10, 20}
	result, count := calculateMovingMedianUint(200, 5, 3, 0, timeBuf, valueBuf)
	if len(result) != 0 {
		t.Fatalf("expected empty result, got %v", result)
	}
	if count != 2 {
		t.Fatalf("expected count 2, got %d", count)
	}
}

// Test-4: parseMovingWindowArgs applies asymmetric time range padding for even N.
//
// For N=4, center=true: startPad = floor(3/2)*interval = 1 interval,
// endPad = ceil(3/2)*interval = 2 intervals. These must differ (asymmetric).

func TestParseMovingWindowArgs_EvenN_AsymmetricPadding(t *testing.T) {
	intervalDur := time.Second
	intervalNS := int64(intervalDur)

	call := &influxql.Call{
		Name: "moving_average",
		Args: []influxql.Expr{
			&influxql.VarRef{Val: "value"},
			&influxql.IntegerLiteral{Val: 4}, // windowSize=4
			&influxql.IntegerLiteral{Val: 4}, // minPeriods=4
			&influxql.BooleanLiteral{Val: true}, // center=true
		},
	}

	origStart := int64(10) * intervalNS
	origEnd := int64(50) * intervalNS
	opt := IteratorOptions{
		StartTime: origStart,
		EndTime:   origEnd,
		Interval:  Interval{Duration: intervalDur},
		Ascending: true,
	}

	n, minPeriods, center := parseMovingWindowArgs(call, &opt)
	if n != 4 {
		t.Errorf("n: expected 4, got %d", n)
	}
	if minPeriods != 4 {
		t.Errorf("minPeriods: expected 4, got %d", minPeriods)
	}
	if !center {
		t.Error("center: expected true")
	}

	// startPad = (4-1)/2 = 1 interval; endPad = 4/2 = 2 intervals
	wantStart := origStart - 1*intervalNS
	wantEnd := origEnd + 2*intervalNS
	if opt.StartTime != wantStart {
		t.Errorf("StartTime: expected %d (-%d), got %d", wantStart, intervalNS, opt.StartTime)
	}
	if opt.EndTime != wantEnd {
		t.Errorf("EndTime: expected %d (+%d), got %d", wantEnd, 2*intervalNS, opt.EndTime)
	}
	// Verify the padding is actually asymmetric
	startDelta := origStart - opt.StartTime
	endDelta := opt.EndTime - origEnd
	if startDelta == endDelta {
		t.Errorf("padding is symmetric (%d == %d), expected asymmetric for even N=4", startDelta, endDelta)
	}
}

func TestParseMovingWindowArgs_OddN_SymmetricPadding(t *testing.T) {
	// For odd N=3, center=true: startPad = endPad = 1 interval (symmetric).
	intervalDur := time.Second
	intervalNS := int64(intervalDur)

	call := &influxql.Call{
		Name: "moving_average",
		Args: []influxql.Expr{
			&influxql.VarRef{Val: "value"},
			&influxql.IntegerLiteral{Val: 3}, // windowSize=3
			&influxql.IntegerLiteral{Val: 1}, // minPeriods=1
			&influxql.BooleanLiteral{Val: true}, // center=true
		},
	}

	origStart := int64(10) * intervalNS
	origEnd := int64(50) * intervalNS
	opt := IteratorOptions{
		StartTime: origStart,
		EndTime:   origEnd,
		Interval:  Interval{Duration: intervalDur},
		Ascending: true,
	}

	parseMovingWindowArgs(call, &opt)

	// startPad = (3-1)/2 = 1; endPad = 3/2 = 1 — equal for odd N
	wantStart := origStart - 1*intervalNS
	wantEnd := origEnd + 1*intervalNS
	if opt.StartTime != wantStart {
		t.Errorf("StartTime: expected %d, got %d", wantStart, opt.StartTime)
	}
	if opt.EndTime != wantEnd {
		t.Errorf("EndTime: expected %d, got %d", wantEnd, opt.EndTime)
	}
}
