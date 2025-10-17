package bandwidthcontroller

import (
	"context"
	"io"
	"sync"
	"time"
)

type BandwidthController struct {
	cfg            ControllerConfig
	streams        map[GroupType]BandwidthGroup
	bandwidth      int64
	freeBandwidth  int64
	stats          ControllerStats
	mu             sync.Mutex
	schedulerStopC chan struct{}
	ctx            context.Context
}

func NewBandwidthController(bandwidth int64, opts ...Option) *BandwidthController {
	bc := &BandwidthController{
		cfg: defaultConfig(),
		// declaring here to prevent checking with each append later
		streams: map[GroupType]BandwidthGroup{
			KB: make(BandwidthGroup),
			MB: make(BandwidthGroup),
			GB: make(BandwidthGroup),
			TB: make(BandwidthGroup),
		},
		bandwidth:     bandwidth,
		freeBandwidth: bandwidth,
		stats: ControllerStats{
			GroupsStats: map[GroupType]*BandwidthStats{
				KB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
				MB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
				GB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
				TB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
			},
		},
		schedulerStopC: make(chan struct{}),
		ctx:            context.Background(),
	}

	for _, opt := range opts {
		opt(bc)
	}

	return bc
}

func (bc *BandwidthController) AppendStreamReader(r io.Reader, streamSize int64) (*Stream, error) {
	return bc.AppendStreamReadCloser(io.NopCloser(r), streamSize)
}

func (bc *BandwidthController) AppendStreamReadCloser(r io.ReadCloser, streamSize int64) (*Stream, error) {
	if isContextCancelled(bc.ctx) {
		return nil, bc.ctx.Err()
	}

	if bc.bandwidth < getStreamMaxBandwidth(1) {
		return nil, InvalidBandwidth
	}

	group, err := getGroup(streamSize)
	if err != nil {
		return nil, err
	}

	bc.mu.Lock()

	// no point in allocating bandwidth larger then the bandwidth required for completing the stream in one pulse
	streamBandwidth := getStreamMaxBandwidth(streamSize)
	if bc.freeBandwidth < streamBandwidth {
		// in any case, don't exceed the desired bandwidth
		streamBandwidth = getStreamBandwidthWithoutDeviation(bc.freeBandwidth)
	}

	bc.freeBandwidth -= streamBandwidth

	bc.stats.TotalStreams++
	streamID := StreamID(bc.stats.TotalStreams)
	streamReader := NewStreamReadCloser(r, streamBandwidth, func() {
		bc.removeStream(group, streamID)
	})

	stream := NewStream(streamReader, streamSize)
	bc.streams[group][streamID] = stream
	bc.stats.OpenStreams++

	bc.stats.GroupsStats[group].CurrentPulseIntervalAmount++
	bc.stats.GroupsStats[group].CurrentPulseIntervalSize += streamSize

	// first stream -> start updater
	if bc.stats.OpenStreams == 1 {
		go bc.startScheduler()
	}

	bc.mu.Unlock()
	return stream, nil
}

func (bc *BandwidthController) removeStream(group GroupType, streamID StreamID) {
	bc.mu.Lock()

	bc.stats.OpenStreams--
	bc.freeBandwidth += bc.streams[group][streamID].RateLimit()
	delete(bc.streams[group], streamID)

	// no more streams -> stop updater
	if bc.stats.OpenStreams == 0 {
		go bc.stopScheduler()
	}

	bc.mu.Unlock()
}

func (bc *BandwidthController) Stats() ControllerStats {
	return bc.stats
}

func (bc *BandwidthController) UpdateBandwidth(bandwidth int64) error {
	if bandwidth < getStreamMaxBandwidth(1) {
		return InvalidBandwidth
	}

	bc.mu.Lock()
	bc.bandwidth = bandwidth
	bc.mu.Unlock()

	bc.updateLimits()
	return nil
}

func (bc *BandwidthController) startScheduler() {
	ticker := time.NewTicker(*bc.cfg.SchedulerInterval)
	for {
		select {
		case <-bc.schedulerStopC:
			ticker.Stop()
			bc.updateLimits()
			bc.updateStatistics()
			return
		case <-ticker.C:
			bc.updateLimits()
			bc.updateStatistics()
		}
	}
}

func (bc *BandwidthController) stopScheduler() {
	bc.schedulerStopC <- struct{}{}
}

func (bc *BandwidthController) updateLimits() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	insights := bc.calculateAndSortGroupsActiveStreamsWeights()

	bc.updateBandwidthGroupLimits(KB, insights)
	bc.updateBandwidthGroupLimits(MB, insights)
	bc.updateBandwidthGroupLimits(GB, insights)
	bc.updateBandwidthGroupLimits(TB, insights)

	bc.freeBandwidth = insights.bandwidthLeft
	bc.stats.ActiveStreams = insights.totalActiveStreams
}

func (bc *BandwidthController) updateBandwidthGroupLimits(group GroupType, insights *bandwidthInsights) {
	weights := insights.weights[group]
	if weights.totalRemainingSize <= 0 || insights.leftGroupsRemainingSize <= 0 {
		bc.stats.GroupsStats[group].ReservedBandwidth = 0
		bc.stats.GroupsStats[group].AllocatedBandwidth = 0
		return
	}

	// calculate bandwidth for group
	groupBandwidth := int64(float64(insights.bandwidthLeft) * (float64(weights.totalRemainingSize) / float64(insights.leftGroupsRemainingSize)))
	minGroupBandwidth := int64(float64(bc.bandwidth) * bc.cfg.MinGroupBandwidthPercentShare[group])
	if groupBandwidth < minGroupBandwidth {
		groupBandwidth = minGroupBandwidth
	}

	bc.stats.GroupsStats[group].ReservedBandwidth = groupBandwidth
	insights.bandwidthLeft -= groupBandwidth
	insights.leftGroupsRemainingSize -= weights.totalRemainingSize

	bandwidthGroup := bc.streams[group]
	for _, streamWeightPair := range weights.weights {
		ratio := streamWeightPair.weight / weights.totalWeights
		weights.totalWeights -= streamWeightPair.weight
		stream := bandwidthGroup[streamWeightPair.id]
		newLimit := int64(float64(groupBandwidth) * ratio)

		// no point in allocating bandwidth larger then the bandwidth required for completing the stream in one pulse
		maxBandwidth := getStreamMaxBandwidth(stream.Size - stream.BytesRead())
		if newLimit > maxBandwidth {
			newLimit = maxBandwidth
		} else {
			// in routine limit don't allocate bandwidth smaller then the minimun
			if newLimit < bc.cfg.MinStreamBandwidthInBytesPerSec[group] {
				newLimit = bc.cfg.MinStreamBandwidthInBytesPerSec[group]
			}
		}

		// removing deviation from determenistic ratelimit time calculations
		newLimit = getStreamBandwidthWithoutDeviation(newLimit)

		// in any case, don't exceed the desired bandwidth
		if newLimit > groupBandwidth {
			newLimit = groupBandwidth
		}

		groupBandwidth -= newLimit

		if stream.RateLimit() != newLimit {
			stream.UpdateRateLimit(newLimit)
		}
	}

	// return left over bandwidth
	bc.stats.GroupsStats[group].AllocatedBandwidth = bc.stats.GroupsStats[group].ReservedBandwidth - groupBandwidth
	insights.bandwidthLeft += groupBandwidth
}

func (bc *BandwidthController) calculateAndSortGroupsActiveStreamsWeights() *bandwidthInsights {
	insights := bandwidthInsights{
		bandwidthLeft: bc.bandwidth,
		weights:       make(map[GroupType]streamWeights),
	}

	insights.weights[KB] = bc.calculateAndSortActiveStreamsWeights(bc.streams[KB])
	insights.weights[MB] = bc.calculateAndSortActiveStreamsWeights(bc.streams[MB])
	insights.weights[GB] = bc.calculateAndSortActiveStreamsWeights(bc.streams[GB])
	insights.weights[TB] = bc.calculateAndSortActiveStreamsWeights(bc.streams[TB])
	insights.leftGroupsRemainingSize = (insights.weights[KB].totalRemainingSize +
		insights.weights[MB].totalRemainingSize +
		insights.weights[GB].totalRemainingSize +
		insights.weights[TB].totalRemainingSize)
	insights.totalActiveStreams = int64(len(insights.weights[KB].weights) +
		len(insights.weights[MB].weights) +
		len(insights.weights[GB].weights) +
		len(insights.weights[TB].weights))

	return &insights
}

func (bc *BandwidthController) calculateAndSortActiveStreamsWeights(streams BandwidthGroup) streamWeights {
	result := streamWeights{
		weights: make([]streamWeight, 0),
	}

	i := 0
	for id, stream := range streams {
		if !stream.Active.Load() {
			continue
		} else if !stream.Reading.Load() && time.Now().UnixNano()-stream.lastReadEndingTime.Load() > int64(*bc.cfg.StreamIdleTimeout) {
			bc.deactivateStream(stream)
		} else if remainingSize := stream.Size - stream.BytesRead(); remainingSize > 0 {
			weight := 1.0 / float64(remainingSize)
			result.totalWeights += weight
			result.totalRemainingSize += remainingSize
			result.weights = bc.insertSorted(result.weights, streamWeight{id: id, weight: weight}, i)
			i++
		} else {
			bc.deactivateStream(stream)
		}
	}

	return result
}

func (bc *BandwidthController) insertSorted(weights []streamWeight, weight streamWeight, currentIndex int) []streamWeight {
	weights = append(weights, weight)
	i := currentIndex
	for i > 0 && weights[i-1].weight < weight.weight {
		weights[i] = weights[i-1]
		i--
	}

	weights[i] = weight
	return weights
}

func (bc *BandwidthController) deactivateStream(stream *Stream) {
	stream.Active.Store(false)
	stream.UpdateRateLimit(0)
}

func (bc *BandwidthController) updateStatistics() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.updateGroupStatistics(KB)
	bc.updateGroupStatistics(MB)
	bc.updateGroupStatistics(GB)
	bc.updateGroupStatistics(TB)
}

func (bc *BandwidthController) updateGroupStatistics(group GroupType) {
	bc.stats.GroupsStats[group].PulseIntervalsStats.AppendNewPulse(
		bc.stats.GroupsStats[group].CurrentPulseIntervalAmount,
		bc.stats.GroupsStats[group].CurrentPulseIntervalSize)
	bc.stats.GroupsStats[group].CurrentPulseIntervalAmount = 0
	bc.stats.GroupsStats[group].CurrentPulseIntervalSize = 0
}
