package bandwidthcontroller

import (
	"testing"

	"github.com/google/uuid"
)

func TestUtils_getFilesSortedWeights(t *testing.T) {
	files := make(map[uuid.UUID]*File)
	var expectedTotalWeight float64
	for i := 1; i <= 4; i++ {
		files[uuid.New()] = NewFile(NewFileReader(nil, 0, nil), int64(i))
		expectedTotalWeight += 1.0 / float64(i)
	}

	weights, totalWeights := getFilesSortedWeights(files)

	if totalWeights != expectedTotalWeight {
		t.Fatalf("totalWeights is different then expected totalWeights: %f expected: %f", totalWeights, expectedTotalWeight)
	}

	for i := 0; i < 4; i++ {
		if weights[i].weight != 1.0/float64(i+1) {
			t.Fatalf("weights is not sorted current weight: %f expected: %f", weights[i].weight, 1.0/float64(i+1))
		}
	}
}
