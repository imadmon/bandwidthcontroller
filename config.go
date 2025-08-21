package bandwidthcontroller

import "time"

type Config struct {
	BandwidthUpdaterInterval   *time.Duration
	MinFileBandwidthPercentage *float64
}

func defaultConfig() Config {
	bandwidthUpdaterInterval := 200 * time.Millisecond
	minFileBandwidthPercentage := 0.10

	return Config{
		BandwidthUpdaterInterval:   &bandwidthUpdaterInterval,
		MinFileBandwidthPercentage: &minFileBandwidthPercentage,
	}
}
