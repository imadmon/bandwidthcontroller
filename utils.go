package bandwidthcontroller

import (
	"context"
	"errors"

	"github.com/imadmon/limitedreader"
)

type streamWeights struct {
	weights            []streamWeight
	totalWeights       float64
	totalRemainingSize int64
}

type streamWeight struct {
	id     int64
	weight float64
}

var InvalidStreamSize = errors.New("invalid stream size")

func getGroupsSortedWeights(streams map[GroupType]BandwidthGroup) (map[GroupType]streamWeights, int64) {
	weights := make(map[GroupType]streamWeights)

	weights[KB] = getStreamsSortedWeights(streams[KB])
	weights[MB] = getStreamsSortedWeights(streams[MB])
	weights[GB] = getStreamsSortedWeights(streams[GB])
	weights[TB] = getStreamsSortedWeights(streams[TB])

	return weights, (weights[KB].totalRemainingSize +
		weights[MB].totalRemainingSize +
		weights[GB].totalRemainingSize +
		weights[TB].totalRemainingSize)
}

func getStreamsSortedWeights(streams BandwidthGroup) streamWeights {
	result := streamWeights{
		weights: make([]streamWeight, 0),
	}

	i := 0
	for id, stream := range streams {
		remainingSize := stream.Size - stream.GetBytesRead()
		if remainingSize > 0 {
			weight := 1.0 / float64(remainingSize)
			result.totalWeights += weight
			result.totalRemainingSize += remainingSize
			result.weights = insertSorted(result.weights, streamWeight{id: id, weight: weight}, i)
			i++
		}
	}

	return result
}

func insertSorted(weights []streamWeight, weight streamWeight, currentIndex int) []streamWeight {
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

// returns the bandwidth required for completing the stream in one pulse
func getStreamMaxBandwidth(size int64) int64 {
	return size * (1000 / limitedreader.DefaultReadIntervalMilliseconds)
}

// removing deviation from determenistic ratelimit time calculations
func getStreamBandwidthWithoutDeviation(bandwidth int64) int64 {
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
		return 0, InvalidStreamSize
	}
}
