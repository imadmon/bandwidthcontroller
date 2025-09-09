package bandwidthcontroller

import "time"

type Config struct {
	BandwidthUpdaterInterval    *time.Duration
	MinGroupBandwidthPercentage map[GroupType]float64
	MinFileBandwidthInBytes     map[GroupType]int64
}

func defaultConfig() Config {
	bandwidthUpdaterInterval := 200 * time.Millisecond
	return Config{
		BandwidthUpdaterInterval: &bandwidthUpdaterInterval,
		MinGroupBandwidthPercentage: map[GroupType]float64{
			KB: 0.10,
			MB: 0.10,
			GB: 0.10,
			TB: 0.10,
		},
		MinFileBandwidthInBytes: map[GroupType]int64{
			KB: int64(KB / 10),
			MB: int64(MB / 10),
			GB: int64(GB / 10),
			TB: int64(TB / 10),
		},
	}
}
