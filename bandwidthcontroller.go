package bandwidthcontroller

import (
	"fmt"
	"io"
	"sync"
	"time"
)

var (
	MinFileLimitPrecentage           int64 = 10 // 0<<100
	LimitUpdaterIntervalMilliseconds int64 = 50
)

type BandwidthController struct {
	files          map[int64]*File
	mu             sync.Mutex
	bandwidth      int64
	fileCounter    int64
	filesInSystems int64
	updaterStopC   chan struct{}
}

func NewBandwidthController(bandwidth int64) *BandwidthController {
	return &BandwidthController{
		files:        make(map[int64]*File),
		bandwidth:    bandwidth,
		updaterStopC: make(chan struct{}),
	}
}

func (bc *BandwidthController) AppendFileReader(r io.Reader, fileSize int64) *File {
	return bc.AppendFileReadCloser(io.NopCloser(r), fileSize)
}

func (bc *BandwidthController) AppendFileReadCloser(r io.ReadCloser, fileSize int64) *File {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	bc.fileCounter++
	fileID := bc.fileCounter
	fileReader := NewFileReadCloser(r, bc.bandwidth, func() {
		bc.removeFile(fileID)
	})

	file := NewFile(fileReader, fileSize)
	bc.files[fileID] = file
	bc.filesInSystems++

	if bc.filesInSystems == 1 { // first file -> start updater
		go bc.startLimitUpdater()
	}

	return file
}

func (bc *BandwidthController) removeFile(fileID int64) {
	bc.mu.Lock()
	defer bc.mu.Unlock()

	delete(bc.files, fileID)
	bc.filesInSystems--

	if bc.filesInSystems == 0 { // no more files -> stop updater
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
	ticker := time.NewTicker(time.Duration(LimitUpdaterIntervalMilliseconds) * time.Millisecond)
	for {
		select {
		case <-ticker.C:
			bc.updateLimits()
		case <-bc.updaterStopC:
			ticker.Stop()
			return
		}
	}
}

func (bc *BandwidthController) stopLimitUpdater() {
	bc.updaterStopC <- struct{}{}
}

func (bc *BandwidthController) updateLimits() {
	fmt.Println("updating...")
	weights, totalWeight := getFilesSortedWeights(bc.files)
	totalBandwidth := bc.bandwidth
	for _, fileWeightPair := range weights {
		ratio := fileWeightPair.weight / totalWeight
		totalWeight -= fileWeightPair.weight
		newLimit := int64(float64(totalBandwidth) * ratio)
		file := bc.files[fileWeightPair.id]
		bytesLeft := file.Size - file.Reader.GetBytesRead()
		if newLimit > bytesLeft {
			newLimit = bytesLeft
		}
		minLimit := file.Size / MinFileLimitPrecentage
		if newLimit < minLimit {
			newLimit = minLimit
		}
		if newLimit > totalBandwidth {
			newLimit = totalBandwidth
		}

		totalBandwidth -= newLimit
		bc.files[fileWeightPair.id].Reader.UpdateRateLimit(newLimit)
	}
}
