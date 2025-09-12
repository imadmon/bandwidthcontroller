package bandwidthcontroller

import (
	"context"
	"errors"

	"github.com/imadmon/limitedreader"
)

type fileWeights struct {
	weights            []fileWeight
	totalWeights       float64
	totalRemainingSize int64
}

type fileWeight struct {
	id     int64
	weight float64
}

var InvalidFileSize = errors.New("invalid file size")

func getGroupsSortedWeights(files map[GroupType]BandwidthGroup) (map[GroupType]fileWeights, int64) {
	weights := make(map[GroupType]fileWeights)

	weights[KB] = getFilesSortedWeights(files[KB])
	weights[MB] = getFilesSortedWeights(files[MB])
	weights[GB] = getFilesSortedWeights(files[GB])
	weights[TB] = getFilesSortedWeights(files[TB])

	return weights, (weights[KB].totalRemainingSize +
		weights[MB].totalRemainingSize +
		weights[GB].totalRemainingSize +
		weights[TB].totalRemainingSize)
}

func getFilesSortedWeights(files BandwidthGroup) fileWeights {
	result := fileWeights{
		weights: make([]fileWeight, 0),
	}

	i := 0
	for id, file := range files {
		remainingSize := file.Size - file.Reader.GetBytesRead()
		if remainingSize > 0 {
			weight := 1.0 / float64(remainingSize)
			result.totalWeights += weight
			result.totalRemainingSize += remainingSize
			result.weights = insertSorted(result.weights, fileWeight{id: id, weight: weight}, i)
			i++
		}
	}

	return result
}

func insertSorted(weights []fileWeight, weight fileWeight, currentIndex int) []fileWeight {
	weights = append(weights, weight)
	i := currentIndex
	for i > 0 && weights[i-1].weight < weight.weight {
		weights[i] = weights[i-1]
		i--
	}

	weights[i] = weight
	return weights
}

func isContextCancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}

// returns the bandwidth required for completing the file in one pulse
func getFileMaxBandwidth(size int64) int64 {
	return size * (1000 / limitedreader.DefaultReadIntervalMilliseconds)
}

// removing deviation from determenistic ratelimit time calculations
func getFileBandwidthWithoutDeviation(bandwidth int64) int64 {
	return bandwidth - bandwidth%(1000/limitedreader.DefaultReadIntervalMilliseconds)
}

func getGroup(size int64) (GroupType, error) {
	switch {
	case size >= int64(TB):
		return TB, nil
	case size >= int64(GB):
		return GB, nil
	case size >= int64(MB):
		return MB, nil
	case size >= 1:
		return KB, nil
	default:
		return 0, InvalidFileSize
	}
}
