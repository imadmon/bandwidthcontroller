package bandwidthcontroller

const PulsesAmount = 5

type PulsesStatistics struct {
	TotalAmount    int64
	TotalSize      int64
	PulseAvgAmount int64
	PulseAvgSize   int64
	Pulses         [PulsesAmount]*PulseStatistics // index 0 == newest
}

type PulseStatistics struct {
	NewStreamsAmount    int64
	NewStreamsTotalSize int64
}

func NewPulsesStatistics() *PulsesStatistics {
	return &PulsesStatistics{
		Pulses: [PulsesAmount]*PulseStatistics{},
	}
}

func (ps *PulsesStatistics) AppendNewPulse(newStreamsAmount, newStreamsTotalSize int64) {
	prevPulse := &PulseStatistics{
		NewStreamsAmount:    newStreamsAmount,
		NewStreamsTotalSize: newStreamsTotalSize,
	}
	for i := 0; i < PulsesAmount; i++ {
		if ps.Pulses[i] == nil {
			ps.Pulses[i] = prevPulse
			break
		}

		t := ps.Pulses[i]
		ps.Pulses[i] = prevPulse
		prevPulse = t
	}

	ps.TotalAmount += newStreamsAmount - prevPulse.NewStreamsAmount
	ps.TotalSize += newStreamsTotalSize - prevPulse.NewStreamsTotalSize
	ps.PulseAvgAmount = ps.TotalAmount / PulsesAmount
	ps.PulseAvgSize = ps.TotalSize / PulsesAmount
}
