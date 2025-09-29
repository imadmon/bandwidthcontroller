package bandwidthcontroller

const PulsesIntervalsAmount = 5

type PulsesStats struct {
	TotalAmount    int64
	TotalSize      int64
	PulseAvgAmount int64
	PulseAvgSize   int64
	PulseIntervals [PulsesIntervalsAmount]*PulseStats // index 0 == newest
}

type PulseStats struct {
	NewStreamsAmount    int64
	NewStreamsTotalSize int64
}

func NewPulsesStats() *PulsesStats {
	return &PulsesStats{
		PulseIntervals: [PulsesIntervalsAmount]*PulseStats{},
	}
}

func (ps *PulsesStats) AppendNewPulse(newStreamsAmount, newStreamsTotalSize int64) {
	prevPulse := &PulseStats{
		NewStreamsAmount:    newStreamsAmount,
		NewStreamsTotalSize: newStreamsTotalSize,
	}
	var i int64
	for i = 0; i < PulsesIntervalsAmount; i++ {
		if ps.PulseIntervals[i] == nil {
			ps.PulseIntervals[i] = prevPulse
			prevPulse = &PulseStats{
				NewStreamsAmount:    0,
				NewStreamsTotalSize: 0,
			}
			i++
			break
		}

		t := ps.PulseIntervals[i]
		ps.PulseIntervals[i] = prevPulse
		prevPulse = t
	}

	ps.TotalAmount += newStreamsAmount - prevPulse.NewStreamsAmount
	ps.TotalSize += newStreamsTotalSize - prevPulse.NewStreamsTotalSize
	ps.PulseAvgAmount = ps.TotalAmount / i
	ps.PulseAvgSize = ps.TotalSize / i
}
