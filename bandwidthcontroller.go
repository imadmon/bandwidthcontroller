package bandwidthcontroller

import (
	"fmt"
	"io"
	"sync"
)

var (
	MinFileLimitPrecentage int64 = 10 // 0<<100
)

type BandwidthController struct {
	files          map[int32]*File
	mu             sync.Mutex
	bandwidth      int64
	fileCounter    int32
	filesInSystems int32
}

func NewBandwidthController(bandwidth int64) *BandwidthController {
	return &BandwidthController{
		files:     make(map[int32]*File),
		bandwidth: bandwidth,
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

	bc.updateLimits()

	return file
}

func (bc *BandwidthController) removeFile(fileID int32) {
	bc.mu.Lock()
	delete(bc.files, fileID)
	bc.updateLimits()
	bc.mu.Unlock()
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
		minLimit := file.Size / MinFileLimitPrecentage
		if newLimit > bytesLeft {
			newLimit = bytesLeft
		}
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
