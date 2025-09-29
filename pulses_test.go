package bandwidthcontroller

import (
	"testing"
)

func TestPulsesNewPulsesStats(t *testing.T) {
	stats := NewPulsesStats()

	if len(stats.PulseIntervals) != PulsesIntervalsAmount {
		t.Fatalf("unexpected Pulses len, len: %d, expected: %d", len(stats.PulseIntervals), PulsesIntervalsAmount)
	}

	if cap(stats.PulseIntervals) != PulsesIntervalsAmount {
		t.Fatalf("unexpected Pulses cap, cap: %d, expected: %d", cap(stats.PulseIntervals), PulsesIntervalsAmount)
	}
}

func TestPulsesAppendNewPulse(t *testing.T) {
	stats := NewPulsesStats()
	smallValuesSum := 0

	for i := 0; i < PulsesIntervalsAmount*2; i++ {
		stats.AppendNewPulse(int64(i), int64(i))

		for j, pulse := range stats.PulseIntervals {
			expectedValue := i - j
			if expectedValue < 0 {
				if pulse != nil {
					t.Fatalf("unexpected PulseStats on lap #%d, got: %v expected: nil", i, pulse)
				}
				continue
			}

			if pulse.NewStreamsAmount != int64(i-j) || pulse.NewStreamsTotalSize != int64(i-j) {
				t.Fatalf("unexpected PulseStats on lap #%d, got: %v expected both values to be: %d", i, pulse, i-j)
			}
		}

		expectedTotalsValue := (int64(i) - (PulsesIntervalsAmount / 2)) * PulsesIntervalsAmount
		expectedAvgsValue := expectedTotalsValue / PulsesIntervalsAmount
		if i < PulsesIntervalsAmount-1 {
			smallValuesSum += i
			expectedTotalsValue = int64(smallValuesSum)
			expectedAvgsValue = expectedTotalsValue / int64(i+1)
		}

		if stats.TotalAmount != expectedTotalsValue || stats.TotalSize != expectedTotalsValue {
			t.Fatalf("unexpected PulsesStats total amount or size on lap #%d, got: amount=%d size=%d expected both values to be: %d", i, stats.TotalAmount, stats.TotalSize, expectedTotalsValue)
		}

		if stats.PulseAvgAmount != expectedAvgsValue || stats.PulseAvgSize != expectedAvgsValue {
			t.Fatalf("unexpected PulsesStats avg amount or size on lap #%d, got: avg amount=%d avg size=%d expected both values to be: %d", i, stats.PulseAvgAmount, stats.PulseAvgSize, expectedAvgsValue)
		}
	}
}

func TestPulsesAppendEmptyPulses(t *testing.T) {
	stats := NewPulsesStats()
	sum := 0

	for i := 1; i <= PulsesIntervalsAmount; i++ {
		stats.AppendNewPulse(int64(i), int64(i))
		sum += i
	}

	for i := 1; i <= PulsesIntervalsAmount; i++ {
		stats.AppendNewPulse(0, 0)
		sum -= i
		if stats.TotalAmount != int64(sum) || stats.TotalSize != int64(sum) {
			t.Fatalf("unexpected PulsesStats total amount or size on lap #%d, got: amount=%d size=%d expected both values to be: %d", i, stats.TotalAmount, stats.TotalSize, sum)
		}
	}
}
