package bandwidthcontroller

import "github.com/google/uuid"

type fileWeight struct {
	id     uuid.UUID
	weight float64
}

func getFilesSortedWeights(files map[uuid.UUID]*File) ([]fileWeight, float64) {
	weights := make([]fileWeight, len(files))
	totalWeight := 0.0
	size := 0

	for id, file := range files {
		remainingSize := file.Size - file.Reader.BytesRead()
		if remainingSize > 0 {
			weight := 1.0 / float64(remainingSize)
			totalWeight += weight
			size = insertSorted(weights, fileWeight{id: id, weight: weight}, size)
		}
	}

	return weights, totalWeight
}

func insertSorted(weights []fileWeight, weight fileWeight, size int) int {
	i := size
	for i > 0 && weights[i-1].weight < weight.weight {
		weights[i] = weights[i-1]
		i--
	}
	weights[i] = weight
	return size + 1
}
