package bandwidthcontroller

import (
	"context"
	"io"
	"sync"
	"time"

	"github.com/imadmon/limitedreader"
)

type BandwidthController struct {
	cfg            Config
	files          map[int64]*File
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
		cfg:           defaultConfig(),
		files:         make(map[int64]*File),
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
		bc.removeFile(fileID)
	})

	file := NewFile(fileReader, fileSize)
	bc.files[fileID] = file
	bc.filesInSystems++

	// first file -> start updater
	if bc.filesInSystems == 1 {
		go bc.startLimitUpdater()
	}

	return file, nil
}

func (bc *BandwidthController) removeFile(fileID int64) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.filesInSystems--
	bc.freeBandwidth += bc.files[fileID].Reader.GetRateLimit()
	delete(bc.files, fileID)

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

	weights, totalWeight := getFilesSortedWeights(bc.files)
	totalBandwidth := bc.bandwidth
	for _, fileWeightPair := range weights {
		ratio := fileWeightPair.weight / totalWeight
		totalWeight -= fileWeightPair.weight
		file := bc.files[fileWeightPair.id]
		newLimit := int64(float64(totalBandwidth) * ratio)

		// no point in allocating bandwidth larger then the bandwidth required for completing the file in one pulse
		maxBandwidth := getFileMaxBandwidth(file.Size - file.Reader.GetBytesRead())
		if newLimit > maxBandwidth {
			newLimit = maxBandwidth
		} else {
			// in routine limit don't allocate bandwidth smaller then the minimun
			minLimit := int64(float64(file.Size) * *bc.cfg.MinFileBandwidthPercentage)
			if newLimit < minLimit {
				newLimit = minLimit
			}
		}

		// removing deviation from determenistic ratelimit time calculations
		newLimit -= newLimit % (1000 / limitedreader.DefaultReadIntervalMilliseconds)

		// in any case, don't exceed the desired bandwidth
		if newLimit > totalBandwidth {
			newLimit = totalBandwidth
		}

		totalBandwidth -= newLimit

		if file.Reader.GetRateLimit() != newLimit {
			bc.files[fileWeightPair.id].Reader.UpdateRateLimit(newLimit)
		}
	}

	bc.freeBandwidth = totalBandwidth
}
