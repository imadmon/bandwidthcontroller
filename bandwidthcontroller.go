package bandwidthcontroller

import (
	"context"
	"errors"
	"io"
	"sync"
	"time"
)

type GroupType int64

const (
	KB GroupType = 1024
	MB GroupType = 1024 * KB
	GB GroupType = 1024 * MB
	TB GroupType = 1024 * GB
)

type BandwidthGroup map[int64]*File

var InvalidBandwidth = errors.New("invalid bandwidth")

type BandwidthController struct {
	cfg             Config
	files           map[GroupType]BandwidthGroup
	bandwidth       int64
	freeBandwidth   int64
	groupsBandwidth map[GroupType]int64
	mu              sync.Mutex
	fileCounter     int64
	filesInSystems  int64
	updaterStopC    chan struct{}
	ctx             context.Context
}

func NewBandwidthController(bandwidth int64, opts ...Option) *BandwidthController {
	bc := &BandwidthController{
		cfg: defaultConfig(),
		// declaring here to prevent checking with each append later
		files: map[GroupType]BandwidthGroup{
			KB: make(BandwidthGroup),
			MB: make(BandwidthGroup),
			GB: make(BandwidthGroup),
			TB: make(BandwidthGroup),
		},
		bandwidth:     bandwidth,
		freeBandwidth: bandwidth,
		groupsBandwidth: map[GroupType]int64{
			KB: 0,
			MB: 0,
			GB: 0,
			TB: 0,
		},
		updaterStopC: make(chan struct{}),
		ctx:          context.Background(),
	}

	for _, opt := range opts {
		opt(bc)
	}

	return bc
}

func (bc *BandwidthController) AppendFileReader(r io.Reader, fileSize int64) (*File, error) {
	return bc.AppendFileReadCloser(io.NopCloser(r), fileSize)
}

func (bc *BandwidthController) AppendFileReadCloser(r io.ReadCloser, fileSize int64) (*File, error) {
	if isContextCancelled(bc.ctx) {
		return nil, bc.ctx.Err()
	}

	if bc.bandwidth < getFileMaxBandwidth(1) {
		return nil, InvalidBandwidth
	}

	group, err := getGroup(fileSize)
	if err != nil {
		return nil, err
	}

	bc.mu.Lock()
	defer bc.mu.Unlock()

	// no point in allocating bandwidth larger then the bandwidth required for completing the file in one pulse
	fileBandwidth := getFileMaxBandwidth(fileSize)
	if bc.freeBandwidth < fileBandwidth {
		// in any case, don't exceed the desired bandwidth
		fileBandwidth = bc.freeBandwidth
	}

	bc.freeBandwidth -= fileBandwidth

	bc.fileCounter++
	fileID := bc.fileCounter
	fileReader := NewFileReadCloser(r, fileBandwidth, func() {
		bc.removeFile(group, fileID)
	})

	file := NewFile(fileReader, fileSize)
	bc.files[group][fileID] = file
	bc.filesInSystems++

	// first file -> start updater
	if bc.filesInSystems == 1 {
		go bc.startLimitUpdater()
	}

	return file, nil
}

func (bc *BandwidthController) removeFile(group GroupType, fileID int64) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.filesInSystems--
	bc.freeBandwidth += bc.files[group][fileID].Reader.GetRateLimit()
	delete(bc.files[group], fileID)

	// no more files -> stop updater
	if bc.filesInSystems == 0 {
		go bc.stopLimitUpdater()
	}
}

func (bc *BandwidthController) GetTotalFilesInSystem() int64 {
	return bc.fileCounter
}

func (bc *BandwidthController) GetCurrentFilesInSystem() int64 {
	return bc.filesInSystems
}

func (bc *BandwidthController) UpdateBandwidth(bandwidth int64) error {
	if bandwidth < getFileMaxBandwidth(1) {
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
			return
		case <-ticker.C:
			bc.updateLimits()
		}
	}
}

func (bc *BandwidthController) stopLimitUpdater() {
	bc.updaterStopC <- struct{}{}
}

func (bc *BandwidthController) updateLimits() {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	weights, overallGroupsRemainingSize := getGroupsSortedWeights(bc.files)
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

type bandwidthInsights struct {
	bandwidthLeft           int64
	leftGroupsRemainingSize int64
}

func (bc *BandwidthController) updateBandwidthGroupLimits(group GroupType, insights *bandwidthInsights, weights fileWeights) {
	if weights.totalRemainingSize <= 0 || insights.leftGroupsRemainingSize <= 0 {
		bc.groupsBandwidth[group] = 0
		return
	}

	// calculate bandwidth for group
	groupBandwidth := int64(float64(insights.bandwidthLeft) * (float64(weights.totalRemainingSize) / float64(insights.leftGroupsRemainingSize)))
	minGroupBandwidth := int64(float64(bc.bandwidth) * bc.cfg.MinGroupBandwidthPercentage[group])
	if groupBandwidth < minGroupBandwidth {
		groupBandwidth = minGroupBandwidth
	}

	bc.groupsBandwidth[group] = groupBandwidth
	insights.bandwidthLeft -= groupBandwidth
	insights.leftGroupsRemainingSize -= weights.totalRemainingSize

	bandwidthGroup := bc.files[group]
	for _, fileWeightPair := range weights.weights {
		ratio := fileWeightPair.weight / weights.totalWeights
		weights.totalWeights -= fileWeightPair.weight
		file := bandwidthGroup[fileWeightPair.id]
		newLimit := int64(float64(groupBandwidth) * ratio)

		// no point in allocating bandwidth larger then the bandwidth required for completing the file in one pulse
		maxBandwidth := getFileMaxBandwidth(file.Size - file.Reader.GetBytesRead())
		if newLimit > maxBandwidth {
			newLimit = maxBandwidth
		} else {
			// in routine limit don't allocate bandwidth smaller then the minimun
			if newLimit < bc.cfg.MinFileBandwidthInBytes[group] {
				newLimit = bc.cfg.MinFileBandwidthInBytes[group]
			}
		}

		// removing deviation from determenistic ratelimit time calculations
		newLimit = getFileBandwidthWithoutDeviation(newLimit)

		// in any case, don't exceed the desired bandwidth
		if newLimit > groupBandwidth {
			newLimit = groupBandwidth
		}

		groupBandwidth -= newLimit

		if file.Reader.GetRateLimit() != newLimit {
			file.Reader.UpdateRateLimit(newLimit)
		}
	}

	// return left over bandwidth
	bc.groupsBandwidth[group] -= groupBandwidth
	insights.bandwidthLeft += groupBandwidth
}
