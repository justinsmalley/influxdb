import sys

def apply_patch():
    content = """package query

import (
	"sort"
	"time"
)

// Float helpers
func calculateMovingAverageFloat(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []float64) ([]float64, int64) {
	if rTime == 0 { return []float64{}, 0 }
	var validValues []float64
	if interval == 0 { validValues = valueBuf } else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime
		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd { validValues = append(validValues, valueBuf[i]) }
		}
	}
	validCount := len(validValues)
	if validCount >= minPeriods {
		sum := 0.0
		for _, v := range validValues { sum += v }
		return []float64{sum / float64(validCount)}, int64(validCount)
	}
	return []float64{}, int64(validCount)
}

func calculateMovingMedianFloat(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []float64) ([]float64, int64) {
	if rTime == 0 { return []float64{}, 0 }
	var validValues []float64
	if interval == 0 {
		for _, v := range valueBuf { validValues = append(validValues, v) }
	} else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime
		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd { validValues = append(validValues, valueBuf[i]) }
		}
	}
	validCount := len(validValues)
	if validCount < minPeriods { return []float64{}, int64(validCount) }
	if validCount == 1 { return []float64{validValues[0]}, int64(validCount) }
	sort.Float64s(validValues)
	if validCount%2 == 0 {
		lo, hi := validValues[validCount/2-1], validValues[validCount/2]
		return []float64{lo + (hi-lo)/2}, int64(validCount)
	}
	return []float64{validValues[validCount/2]}, int64(validCount)
}

// Int helpers
func calculateMovingAverageInt(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []int64) ([]float64, int64) {
	if rTime == 0 { return []float64{}, 0 }
	var validValues []int64
	if interval == 0 { validValues = valueBuf } else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime
		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd { validValues = append(validValues, valueBuf[i]) }
		}
	}
	validCount := len(validValues)
	if validCount >= minPeriods {
		sum := 0.0
		for _, v := range validValues { sum += float64(v) }
		return []float64{sum / float64(validCount)}, int64(validCount)
	}
	return []float64{}, int64(validCount)
}

func calculateMovingMedianInt(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []int64) ([]float64, int64) {
	if rTime == 0 { return []float64{}, 0 }
	var validValues []float64
	if interval == 0 {
		for _, v := range valueBuf { validValues = append(validValues, float64(v)) }
	} else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime
		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd { validValues = append(validValues, float64(valueBuf[i])) }
		}
	}
	validCount := len(validValues)
	if validCount < minPeriods { return []float64{}, int64(validCount) }
	if validCount == 1 { return []float64{validValues[0]}, int64(validCount) }
	sort.Float64s(validValues)
	if validCount%2 == 0 {
		lo, hi := validValues[validCount/2-1], validValues[validCount/2]
		return []float64{lo + (hi-lo)/2}, int64(validCount)
	}
	return []float64{validValues[validCount/2]}, int64(validCount)
}

// Uint helpers
func calculateMovingAverageUint(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []uint64) ([]float64, int64) {
	if rTime == 0 { return []float64{}, 0 }
	var validValues []uint64
	if interval == 0 { validValues = valueBuf } else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime
		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd { validValues = append(validValues, valueBuf[i]) }
		}
	}
	validCount := len(validValues)
	if validCount >= minPeriods {
		sum := 0.0
		for _, v := range validValues { sum += float64(v) }
		return []float64{sum / float64(validCount)}, int64(validCount)
	}
	return []float64{}, int64(validCount)
}

func calculateMovingMedianUint(rTime int64, windowSize, minPeriods int, interval time.Duration, timeBuf []int64, valueBuf []uint64) ([]float64, int64) {
	if rTime == 0 { return []float64{}, 0 }
	var validValues []float64
	if interval == 0 {
		for _, v := range valueBuf { validValues = append(validValues, float64(v)) }
	} else {
		intervalNS := interval.Nanoseconds()
		validStart := rTime - (int64(windowSize-1) * intervalNS)
		validEnd := rTime
		for i := 0; i < len(valueBuf); i++ {
			if timeBuf[i] >= validStart && timeBuf[i] <= validEnd { validValues = append(validValues, float64(valueBuf[i])) }
		}
	}
	validCount := len(validValues)
	if validCount < minPeriods { return []float64{}, int64(validCount) }
	if validCount == 1 { return []float64{validValues[0]}, int64(validCount) }
	sort.Float64s(validValues)
	if validCount%2 == 0 {
		lo, hi := validValues[validCount/2-1], validValues[validCount/2]
		return []float64{lo + (hi-lo)/2}, int64(validCount)
	}
	return []float64{validValues[validCount/2]}, int64(validCount)
}
"""
    with open('query/functions_moving_helpers.go', 'w') as f:
        f.write(content)

    # Now replace references in query/functions.go
    with open('query/functions.go', 'r') as f:
        funcs = f.read()

    funcs = funcs.replace("calculateMovingAverage(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)",
                        "calculateMovingAverageFloat(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)", 1)
    funcs = funcs.replace("calculateMovingAverage(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)",
                        "calculateMovingAverageInt(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)", 1)
    funcs = funcs.replace("calculateMovingAverage(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)",
                        "calculateMovingAverageUint(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)", 1)

    funcs = funcs.replace("calculateMovingMedian(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)",
                        "calculateMovingMedianFloat(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)", 1)
    funcs = funcs.replace("calculateMovingMedian(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)",
                        "calculateMovingMedianInt(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)", 1)
    funcs = funcs.replace("calculateMovingMedian(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)",
                        "calculateMovingMedianUint(r.time, r.windowSize, r.minPeriods, r.interval, r.timeBuf, r.valueBuf)", 1)

    with open('query/functions.go', 'w') as f:
        f.write(funcs)

    print("Patched moving helpers to avoid generics")

if __name__ == '__main__':
    apply_patch()
