package bandwidthcontroller

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/imadmon/limitedreader"
)

type GroupType int64

const (
	KB GroupType = 1024
	MB GroupType = 1024 * KB
	GB GroupType = 1024 * MB
	TB GroupType = 1024 * GB
)

type BandwidthGroup map[int64]*File

type BandwidthController struct {
	cfg            Config
	files          map[GroupType]BandwidthGroup
	mu             sync.Mutex
	bandwidth      int64
	freeBandwidth  int64
	fileCounter    int64
	filesInSystems int64
	updaterStopC   chan struct{}
	ctx            context.Context
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
		updaterStopC:  make(chan struct{}),
		ctx:           context.Background(),
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

	if fileSize <= 0 {
		return nil, InvalidFileSize
	}

	group := getGroup(fileSize)

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
		return
	}

	// calculate bandwidth for group
	groupBandwidth := insights.bandwidthLeft * (weights.totalRemainingSize / insights.leftGroupsRemainingSize)
	minGroupBandwidth := bc.bandwidth * int64(bc.cfg.MinGroupBandwidthPercentage[group])
	if groupBandwidth < minGroupBandwidth {
		groupBandwidth = minGroupBandwidth
	}

	insights.bandwidthLeft -= groupBandwidth
	insights.leftGroupsRemainingSize -= weights.totalRemainingSize

	bandwidthGroup := bc.files[group]
	for _, fileWeightPair := range weights.weights {
		ratio := fileWeightPair.weight / weights.totalWeight
		weights.totalWeight -= fileWeightPair.weight
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
		newLimit -= newLimit % (1000 / limitedreader.DefaultReadIntervalMilliseconds)

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
	insights.bandwidthLeft += groupBandwidth
}
