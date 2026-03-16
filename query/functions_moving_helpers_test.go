package query

import (
	"testing"
	"time"
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
