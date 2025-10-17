package bandwidthcontroller

import "time"

type ControllerConfig struct {
	SchedulerInterval               *time.Duration
	StreamIdleTimeout               *time.Duration
	MinGroupBandwidthPercentShare   map[GroupType]float64 // values in [0.01, 1.00]
	MinStreamBandwidthInBytesPerSec map[GroupType]int64
}

func defaultConfig() ControllerConfig {
	schedulerInterval := 200 * time.Millisecond
	streamActivationTimeout := 100 * time.Millisecond
	return ControllerConfig{
		SchedulerInterval: &schedulerInterval,
		StreamIdleTimeout: &streamActivationTimeout,
		MinGroupBandwidthPercentShare: map[GroupType]float64{
			KB: 0.10,
			MB: 0.10,
			GB: 0.10,
			TB: 0.10,
		},
		MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
			KB: int64(KB / 10),
			MB: int64(MB / 10),
			GB: int64(GB / 10),
			TB: int64(TB / 10),
		},
	}
}
