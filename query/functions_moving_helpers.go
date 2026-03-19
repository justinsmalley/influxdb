package query

import (
	"sort"
	"time"
)

// filterValidIndices returns the indices of timeBuf entries within the window
// [rTime - (windowSize-1)*interval, rTime]. If interval is 0, all indices are valid.
func filterValidIndices(rTime int64, windowSize int, interval time.Duration, timeBuf []int64) []int {
	n := len(timeBuf)
	if interval == 0 {
		indices := make([]int, n)
		for i := range indices {
			indices[i] = i
		}
		return indices
	}
	intervalNS := interval.Nanoseconds()
	validStart := rTime - (int64(windowSize-1) * intervalNS)
	var indices []int
	for i := 0; i < n; i++ {
		if timeBuf[i] >= validStart && timeBuf[i] <= rTime {
			indices = append(indices, i)
		}
	}
	return indices
}

// computeMedian returns the median of a pre-sorted, non-empty float64 slice.
func computeMedian(sorted []float64) float64 {
	n := len(sorted)
	if n == 1 {
		return sorted[0]
	}
	if n%2 == 0 {
		lo, hi := sorted[n/2-1], sorted[n/2]
		return lo + (hi-lo)/2
	}
	return sorted[n/2]
}

// movingAverageCore computes the moving average over a pre-converted []float64 buffer.
func movingAverageCore(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, vals []float64) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}
	indices := filterValidIndices(rTime, windowSize, interval, timeBuf)
	validCount := len(indices)
	if validCount == 0 {
		return []float64{}, 0
	}
	if validCount >= minPeriods {
		sum := 0.0
		for _, i := range indices {
			sum += vals[i]
		}
		return []float64{sum / float64(validCount)}, int64(validCount)
	}
	return []float64{}, int64(validCount)
}

// movingMedianCore computes the moving median over a pre-converted []float64 buffer.
func movingMedianCore(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, vals []float64) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}
	indices := filterValidIndices(rTime, windowSize, interval, timeBuf)
	validCount := len(indices)
	if validCount < minPeriods {
		return []float64{}, int64(validCount)
	}
	window := make([]float64, validCount)
	for j, i := range indices {
		window[j] = vals[i]
	}
	sort.Float64s(window)
	return []float64{computeMedian(window)}, int64(validCount)
}

// Float helpers

func calculateMovingAverageFloat(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []float64) ([]float64, int64) {
	return movingAverageCore(rTime, windowSize, minPeriods, interval, timeBuf, valueBuf)
}

func calculateMovingMedianFloat(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []float64) ([]float64, int64) {
	return movingMedianCore(rTime, windowSize, minPeriods, interval, timeBuf, valueBuf)
}

// Int helpers

func calculateMovingAverageInt(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []int64) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}
	indices := filterValidIndices(rTime, windowSize, interval, timeBuf)
	n := len(indices)
	if n == 0 || n < minPeriods {
		return []float64{}, int64(n)
	}
	sum := 0.0
	for _, i := range indices {
		sum += float64(valueBuf[i])
	}
	return []float64{sum / float64(n)}, int64(n)
}

func calculateMovingMedianInt(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []int64) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}
	indices := filterValidIndices(rTime, windowSize, interval, timeBuf)
	n := len(indices)
	if n < minPeriods {
		return []float64{}, int64(n)
	}
	window := make([]float64, n)
	for j, i := range indices {
		window[j] = float64(valueBuf[i])
	}
	sort.Float64s(window)
	return []float64{computeMedian(window)}, int64(n)
}

// Uint helpers

func calculateMovingAverageUint(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []uint64) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}
	indices := filterValidIndices(rTime, windowSize, interval, timeBuf)
	n := len(indices)
	if n == 0 || n < minPeriods {
		return []float64{}, int64(n)
	}
	sum := 0.0
	for _, i := range indices {
		sum += float64(valueBuf[i])
	}
	return []float64{sum / float64(n)}, int64(n)
}

func calculateMovingMedianUint(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []uint64) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}
	indices := filterValidIndices(rTime, windowSize, interval, timeBuf)
	n := len(indices)
	if n < minPeriods {
		return []float64{}, int64(n)
	}
	window := make([]float64, n)
	for j, i := range indices {
		window[j] = float64(valueBuf[i])
	}
	sort.Float64s(window)
	return []float64{computeMedian(window)}, int64(n)
}
