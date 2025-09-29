package bandwidthcontroller

import (
	"context"
	"io"
	"sync"
	"time"
)

type BandwidthController struct {
	cfg              ControllerConfig
	streams          map[GroupType]BandwidthGroup
	bandwidth        int64
	freeBandwidth    int64
	stats            map[GroupType]*BandwidthStats
	mu               sync.Mutex
	streamCounter    int64
	streamsInSystems int64
	updaterStopC     chan struct{}
	ctx              context.Context
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
		stats: map[GroupType]*BandwidthStats{
			KB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
			MB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
			GB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
			TB: &BandwidthStats{PulseIntervalsStats: NewPulsesStats()},
		},
		updaterStopC: make(chan struct{}),
		ctx:          context.Background(),
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
	defer bc.mu.Unlock()

	// no point in allocating bandwidth larger then the bandwidth required for completing the stream in one pulse
	streamBandwidth := getStreamMaxBandwidth(streamSize)
	if bc.freeBandwidth < streamBandwidth {
		// in any case, don't exceed the desired bandwidth
		streamBandwidth = bc.freeBandwidth
	}

	bc.freeBandwidth -= streamBandwidth

	bc.streamCounter++
	streamID := bc.streamCounter
	streamReader := NewStreamReadCloser(r, streamBandwidth, func() {
		bc.removeStream(group, streamID)
	})

	stream := NewStream(streamReader, streamSize)
	bc.streams[group][streamID] = stream
	bc.streamsInSystems++

	bc.stats[group].CurrentPulseIntervalAmount++
	bc.stats[group].CurrentPulseIntervalSize += streamSize

	// first stream -> start updater
	if bc.streamsInSystems == 1 {
		go bc.startLimitUpdater()
	}

	return stream, nil
}

func (bc *BandwidthController) removeStream(group GroupType, streamID int64) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.streamsInSystems--
	bc.freeBandwidth += bc.streams[group][streamID].GetRateLimit()
	delete(bc.streams[group], streamID)

	// no more streams -> stop updater
	if bc.streamsInSystems == 0 {
		go bc.stopLimitUpdater()
	}
}

func (bc *BandwidthController) GetTotalStreamsInSystem() int64 {
	return bc.streamCounter
}

func (bc *BandwidthController) GetCurrentStreamsInSystem() int64 {
	return bc.streamsInSystems
}

func (bc *BandwidthController) GetStatistics() map[GroupType]*BandwidthStats {
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

func (bc *BandwidthController) startLimitUpdater() {
	ticker := time.NewTicker(*bc.cfg.BandwidthUpdaterInterval)
	for {
		select {
		case <-bc.updaterStopC:
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

func (bc *BandwidthController) stopLimitUpdater() {
	bc.updaterStopC <- struct{}{}
}

func (bc *BandwidthController) updateLimits() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	weights, overallGroupsRemainingSize := getGroupsSortedWeights(bc.streams)
	insights := bandwidthInsights{
		bandwidthLeft:           bc.bandwidth,
		leftGroupsRemainingSize: overallGroupsRemainingSize,
	}

	bc.updateBandwidthGroupLimits(KB, &insights, weights[KB])
	bc.updateBandwidthGroupLimits(MB, &insights, weights[MB])
	bc.updateBandwidthGroupLimits(GB, &insights, weights[GB])
	bc.updateBandwidthGroupLimits(TB, &insights, weights[TB])

	bc.freeBandwidth = insights.bandwidthLeft
}

func (bc *BandwidthController) updateBandwidthGroupLimits(group GroupType, insights *bandwidthInsights, weights streamWeights) {
	if weights.totalRemainingSize <= 0 || insights.leftGroupsRemainingSize <= 0 {
		bc.stats[group].ReservedBandwidth = 0
		bc.stats[group].AllocatedBandwidth = 0
		return
	}

	// calculate bandwidth for group
	groupBandwidth := int64(float64(insights.bandwidthLeft) * (float64(weights.totalRemainingSize) / float64(insights.leftGroupsRemainingSize)))
	minGroupBandwidth := int64(float64(bc.bandwidth) * bc.cfg.MinGroupBandwidthPercentShare[group])
	if groupBandwidth < minGroupBandwidth {
		groupBandwidth = minGroupBandwidth
	}

	bc.stats[group].ReservedBandwidth = groupBandwidth
	insights.bandwidthLeft -= groupBandwidth
	insights.leftGroupsRemainingSize -= weights.totalRemainingSize

	bandwidthGroup := bc.streams[group]
	for _, streamWeightPair := range weights.weights {
		ratio := streamWeightPair.weight / weights.totalWeights
		weights.totalWeights -= streamWeightPair.weight
		stream := bandwidthGroup[streamWeightPair.id]
		newLimit := int64(float64(groupBandwidth) * ratio)

		// no point in allocating bandwidth larger then the bandwidth required for completing the stream in one pulse
		maxBandwidth := getStreamMaxBandwidth(stream.Size - stream.GetBytesRead())
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

		if stream.GetRateLimit() != newLimit {
			stream.UpdateRateLimit(newLimit)
		}
	}

	// return left over bandwidth
	bc.stats[group].AllocatedBandwidth = bc.stats[group].ReservedBandwidth - groupBandwidth
	insights.bandwidthLeft += groupBandwidth
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
	bc.stats[group].PulseIntervalsStats.AppendNewPulse(bc.stats[group].CurrentPulseIntervalAmount, bc.stats[group].CurrentPulseIntervalSize)
	bc.stats[group].CurrentPulseIntervalAmount = 0
	bc.stats[group].CurrentPulseIntervalSize = 0
}
