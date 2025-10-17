package bandwidthcontroller

import (
	"context"
	"errors"

	"github.com/imadmon/limitedreader"
)

var InvalidStreamSize = errors.New("invalid stream size")

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
