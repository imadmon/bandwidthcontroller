package bandwidthcontroller

import "errors"

type GroupType int64

const (
	KB GroupType = 1024
	MB GroupType = 1024 * KB
	GB GroupType = 1024 * MB
	TB GroupType = 1024 * GB
)

type BandwidthGroup map[int64]*Stream

var InvalidBandwidth = errors.New("invalid bandwidth")

type BandwidthStats struct {
	ReservedBandwidth          int64
	AllocatedBandwidth         int64
	CurrentPulseIntervalAmount int64
	CurrentPulseIntervalSize   int64
	PulseIntervalsStats        *PulsesStats // 1 second stats
}

type bandwidthInsights struct {
	bandwidthLeft           int64
	leftGroupsRemainingSize int64
}
