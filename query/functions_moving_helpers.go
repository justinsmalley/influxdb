package query

import (
	"sort"
	"time"
)

type numeric interface {
	~int64 | ~uint64 | ~float64
}

// calculateMovingAverage computes the moving average using a generic buffer.
func calculateMovingAverage[T numeric](
	rTime int64,
	windowSize, minPeriods int,
	interval time.Duration,
	timeBuf []int64,
	valueBuf []T,
) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}

	var validValues []T
	if interval == 0 {
		validValues = valueBuf
	} else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime

		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd {
				validValues = append(validValues, valueBuf[i])
			}
		}
	}

	validCount := len(validValues)
	if validCount >= minPeriods {
		sum := 0.0
		for _, v := range validValues {
			sum += float64(v)
		}
		return []float64{sum / float64(validCount)}, int64(validCount)
	}
	return []float64{}, int64(validCount)
}

// calculateMovingMedian computes the moving median using a generic buffer.
func calculateMovingMedian[T numeric](
	rTime int64,
	windowSize, minPeriods int,
	interval time.Duration,
	timeBuf []int64,
	valueBuf []T,
) ([]float64, int64) {
	if rTime == 0 {
		return []float64{}, 0
	}

	var validValues []float64
	if interval == 0 {
		for _, v := range valueBuf {
			validValues = append(validValues, float64(v))
		}
	} else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime

		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd {
				validValues = append(validValues, float64(valueBuf[i]))
			}
		}
	}

	validCount := len(validValues)
	if validCount < minPeriods {
		return []float64{}, int64(validCount)
	}

	if validCount == 1 {
		return []float64{validValues[0]}, int64(validCount)
	}

	sort.Float64s(validValues)

	if validCount%2 == 0 {
		lo, hi := validValues[validCount/2-1], validValues[validCount/2]
		return []float64{lo + (hi-lo)/2}, int64(validCount)
	}
	return []float64{validValues[validCount/2]}, int64(validCount)
}
