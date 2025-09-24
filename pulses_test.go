package bandwidthcontroller

import (
	"testing"
)

func TestPulsesNewPulsesStatistics(t *testing.T) {
	statistics := NewPulsesStatistics()

	if len(statistics.Pulses) != PulsesAmount {
		t.Fatalf("unexpected Pulses len, len: %d, expected: %d", len(statistics.Pulses), PulsesAmount)
	}

	if cap(statistics.Pulses) != PulsesAmount {
		t.Fatalf("unexpected Pulses cap, cap: %d, expected: %d", cap(statistics.Pulses), PulsesAmount)
	}
}

func TestPulsesAppendNewPulse(t *testing.T) {
	statistics := NewPulsesStatistics()
	smallValuesSum := 0

	for i := 0; i < PulsesAmount*2; i++ {
		statistics.AppendNewPulse(int64(i), int64(i))

		for j, pulse := range statistics.Pulses {
			expectedValue := i - j
			if expectedValue < 0 {
				if pulse != nil {
					t.Fatalf("unexpected PulseStatistics on lap #%d, got: %v expected: nil", i, pulse)
				}
				continue
			}

			if pulse.NewStreamsAmount != int64(i-j) || pulse.NewStreamsTotalSize != int64(i-j) {
				t.Fatalf("unexpected PulseStatistics on lap #%d, got: %v expected both values to be: %d", i, pulse, i-j)
			}
		}

		expectedTotalsValue := (int64(i) - (PulsesAmount / 2)) * PulsesAmount
		if i < PulsesAmount-1 {
			smallValuesSum += i
			expectedTotalsValue = int64(smallValuesSum)
		}

		expectedAvgsValue := expectedTotalsValue / PulsesAmount

		if statistics.TotalAmount != expectedTotalsValue || statistics.TotalSize != expectedTotalsValue {
			t.Fatalf("unexpected PulsesStatistics total amount or size on lap #%d, got: amount=%d size=%d expected both values to be: %d", i, statistics.TotalAmount, statistics.TotalSize, expectedTotalsValue)
		}

		if statistics.PulseAvgAmount != expectedAvgsValue || statistics.PulseAvgSize != expectedAvgsValue {
			t.Fatalf("unexpected PulsesStatistics avg amount or size on lap #%d, got: avg amount=%d avg size=%d expected both values to be: %d", i, statistics.PulseAvgAmount, statistics.PulseAvgSize, expectedAvgsValue)
		}
	}
}
