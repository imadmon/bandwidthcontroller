package bandwidthcontroller

import (
	"fmt"
	"io"
	"sync"

	"github.com/google/uuid"
)

var (
	MinFileLimitPrecentage int64 = 10 // 0<<100
)

type BandwidthController struct {
	files     map[uuid.UUID]*File
	mu        sync.Mutex
	bandwidth int64
}

func NewBandwidthController(bandwidth int64) *BandwidthController {
	return &BandwidthController{
		files:     make(map[uuid.UUID]*File),
		bandwidth: bandwidth,
	}
}

func (bc *BandwidthController) AppendFileReader(r io.Reader, fileSize int64) *File {
	return bc.AppendFileReadCloser(io.NopCloser(r), fileSize)
}

func (bc *BandwidthController) AppendFileReadCloser(r io.ReadCloser, fileSize int64) *File {
	fileID := uuid.New()
	fileReader := NewFileReadCloser(r, bc.bandwidth, func() {
		bc.removeFile(fileID)
	})

	file := NewFile(fileReader, fileSize)

	bc.mu.Lock()
	bc.files[fileID] = file
	bc.updateLimits()
	bc.mu.Unlock()

	return file
}

func (bc *BandwidthController) removeFile(fileID uuid.UUID) {
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
