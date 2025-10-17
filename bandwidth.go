package bandwidthcontroller

import "errors"

type GroupType int64

const (
	KB GroupType = 1024
	MB GroupType = 1024 * KB
	GB GroupType = 1024 * MB
	TB GroupType = 1024 * GB
)

type StreamID int64

type BandwidthGroup map[StreamID]*Stream

var InvalidBandwidth = errors.New("invalid bandwidth")

type ControllerStats struct {
	TotalStreams  int64
	OpenStreams   int64
	ActiveStreams int64
	GroupsStats   map[GroupType]*BandwidthStats
}

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
	totalActiveStreams      int64
	weights                 map[GroupType]streamWeights
}

type streamWeights struct {
	weights            []streamWeight
	totalWeights       float64
	totalRemainingSize int64
}

type streamWeight struct {
	id     StreamID
	weight float64
}
