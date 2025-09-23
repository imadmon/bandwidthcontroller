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

type BandwidthStatistics struct {
	BandwidthAllocated int64
	BandwidthUsed      int64
	CurrentPulseAmount int64
	CurrentPulseSize   int64
	Pulses             *PulsesStatistics // 1 second statistics
}

type bandwidthInsights struct {
	bandwidthLeft           int64
	leftGroupsRemainingSize int64
}
