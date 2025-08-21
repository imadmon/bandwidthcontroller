package bandwidthcontroller

import "context"

type fileWeight struct {
	id     int64
	weight float64
}

func getFilesSortedWeights(files map[int64]*File) ([]fileWeight, float64) {
	weights := make([]fileWeight, 0)
	totalWeight := 0.0
	i := 0

	for id, file := range files {
		remainingSize := file.Size - file.Reader.GetBytesRead()
		if remainingSize > 0 {
			weight := 1.0 / float64(remainingSize)
			totalWeight += weight
			weights = insertSorted(weights, fileWeight{id: id, weight: weight}, i)
			i++
		}
	}

	return weights, totalWeight
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
