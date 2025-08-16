package bandwidthcontroller

import (
	"testing"
)

func TestUtilsGetFilesSortedWeights(t *testing.T) {
	files := make(map[int32]*File)
	var expectedTotalWeight float64
	for i := 5; i > 0; i-- {
		files[int32(i)] = NewFile(NewFileReadCloser(nil, 0, nil), int64(i))
		expectedTotalWeight += 1.0 / float64(i)
	}

	weights, totalWeights := getFilesSortedWeights(files)

	if totalWeights != expectedTotalWeight {
		t.Fatalf("totalWeights is different then expected totalWeights: %f expected: %f", totalWeights, expectedTotalWeight)
	}

	for i := 0; i < 5; i++ {
		if weights[i].weight != 1.0/float64(i+1) {
			t.Fatalf("weights is not sorted current weight: %f expected: %f", weights[i].weight, 1.0/float64(i+1))
		}
	}
}

func TestUtilsGetFilesSortedWeightsEmptyOnFinish(t *testing.T) {
	files := make(map[int32]*File)
	for i := 5; i > 0; i-- {
		file := NewFile(NewFileReadCloser(nil, 0, nil), 0)
		files[int32(i)] = file
	}

	weights, totalWeights := getFilesSortedWeights(files)

	if totalWeights != 0 {
		t.Fatalf("totalWeights is more then 0, totalWeights: %f", totalWeights)
	}

	if len(weights) != 0 {
		t.Fatalf("weights length is more then 0, weights length: %d", len(weights))
	}
}
