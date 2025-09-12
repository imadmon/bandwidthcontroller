package bandwidthcontroller

import (
	"testing"
)

func TestUtilsGetGroupsSortedWeights(t *testing.T) {
	cases := []struct {
		name                               string
		expectedOverallGroupsRemainingSize int64
		expectedGroupsAmounts              map[GroupType]int
		expectedGroupsTotalSizes           map[GroupType]int64
	}{
		{
			name:                               "only KB files",
			expectedOverallGroupsRemainingSize: 100,
			expectedGroupsAmounts: map[GroupType]int{
				KB: 10,
				MB: 0,
				GB: 0,
				TB: 0,
			},
			expectedGroupsTotalSizes: map[GroupType]int64{
				KB: 100,
				MB: 0,
				GB: 0,
				TB: 0,
			},
		},
		{
			name:                               "both KB and GB files",
			expectedOverallGroupsRemainingSize: 5*int64(GB) + 100,
			expectedGroupsAmounts: map[GroupType]int{
				KB: 10,
				MB: 0,
				GB: 5,
				TB: 0,
			},
			expectedGroupsTotalSizes: map[GroupType]int64{
				KB: 100,
				MB: 0,
				GB: 5 * int64(GB),
				TB: 0,
			},
		},
		{
			name:                               "all group types files",
			expectedOverallGroupsRemainingSize: 5*int64(TB) + 5*int64(GB) + 20*int64(MB) + 100,
			expectedGroupsAmounts: map[GroupType]int{
				KB: 10,
				MB: 10,
				GB: 5,
				TB: 5,
			},
			expectedGroupsTotalSizes: map[GroupType]int64{
				KB: 100,
				MB: 20 * int64(MB),
				GB: 5 * int64(GB),
				TB: 5 * int64(TB),
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			files := map[GroupType]BandwidthGroup{
				KB: generateGroupFiles(c.expectedGroupsAmounts[KB], c.expectedGroupsTotalSizes[KB]),
				MB: generateGroupFiles(c.expectedGroupsAmounts[MB], c.expectedGroupsTotalSizes[MB]),
				GB: generateGroupFiles(c.expectedGroupsAmounts[GB], c.expectedGroupsTotalSizes[GB]),
				TB: generateGroupFiles(c.expectedGroupsAmounts[TB], c.expectedGroupsTotalSizes[TB]),
			}

			weights, overallGroupsRemainingSize := getGroupsSortedWeights(files)

			if overallGroupsRemainingSize != c.expectedOverallGroupsRemainingSize {
				t.Fatalf("overallGroupsRemainingSize mismatch\ngot: %d\nexpected: %d", overallGroupsRemainingSize, c.expectedOverallGroupsRemainingSize)
			}

			sumGroupsTotalRemainingSize := (weights[KB].totalRemainingSize +
				weights[MB].totalRemainingSize +
				weights[GB].totalRemainingSize +
				weights[TB].totalRemainingSize)
			if sumGroupsTotalRemainingSize != c.expectedOverallGroupsRemainingSize {
				t.Fatalf("sumGroupsTotalRemainingSize mismatch\ngot: %d\nexpected: %d", sumGroupsTotalRemainingSize, c.expectedOverallGroupsRemainingSize)
			}

			validateWeightsPerGroup(t, KB, "KB", weights, c.expectedGroupsAmounts, c.expectedGroupsTotalSizes)
			validateWeightsPerGroup(t, MB, "MB", weights, c.expectedGroupsAmounts, c.expectedGroupsTotalSizes)
			validateWeightsPerGroup(t, GB, "GB", weights, c.expectedGroupsAmounts, c.expectedGroupsTotalSizes)
			validateWeightsPerGroup(t, TB, "TB", weights, c.expectedGroupsAmounts, c.expectedGroupsTotalSizes)
		})
	}
}

func generateGroupFiles(filesAmount int, filesTotalSize int64) BandwidthGroup {
	files := make(BandwidthGroup)
	for i := 0; i < filesAmount; i++ {
		files[int64(i)] = NewFile(NewFileReadCloser(nil, 0, nil), filesTotalSize/int64(filesAmount))
	}

	return files
}

func validateWeightsPerGroup(t *testing.T, group GroupType, groupName string,
	weights map[GroupType]fileWeights,
	expectedGroupsAmounts map[GroupType]int,
	expectedGroupsTotalSizes map[GroupType]int64) {
	if len(weights[group].weights) != expectedGroupsAmounts[group] {
		t.Fatalf("groupAmount %s mismatch\ngot: %d\nexpected: %d", groupName, len(weights[group].weights), expectedGroupsAmounts[group])
	}

	if weights[group].totalRemainingSize != expectedGroupsTotalSizes[group] {
		t.Fatalf("groupTotalRemainingSize %s mismatch\ngot: %d\nexpected: %d", groupName, weights[group].totalRemainingSize, expectedGroupsTotalSizes[group])
	}
}

func TestUtilsGetFilesSortedWeights(t *testing.T) {
	files := make(BandwidthGroup)
	var expectedTotalWeights float64
	var expectedTotalRemainingSize int64
	for i := 5; i > 0; i-- {
		files[int64(i)] = NewFile(NewFileReadCloser(nil, 0, nil), int64(i))
		expectedTotalWeights += 1.0 / float64(i)
		expectedTotalRemainingSize += int64(i)
	}

	fileWeights := getFilesSortedWeights(files)

	if fileWeights.totalRemainingSize != expectedTotalRemainingSize {
		t.Fatalf("totalRemainingSize is different then expected totalRemainingSize: %d expected: %d", fileWeights.totalRemainingSize, expectedTotalRemainingSize)
	}

	if fileWeights.totalWeights != expectedTotalWeights {
		t.Fatalf("totalWeights is different then expected totalWeights: %f expected: %f", fileWeights.totalWeights, expectedTotalWeights)
	}

	for i := 0; i < 5; i++ {
		if fileWeights.weights[i].weight != 1.0/float64(i+1) {
			t.Fatalf("weights is not sorted current weight: %f expected: %f", fileWeights.weights[i].weight, 1.0/float64(i+1))
		}
	}
}

func TestUtilsGetFilesSortedWeightsEmptyOnFinish(t *testing.T) {
	files := make(BandwidthGroup)
	for i := 5; i > 0; i-- {
		files[int64(i)] = NewFile(NewFileReadCloser(nil, 0, nil), 0)
	}

	fileWeights := getFilesSortedWeights(files)

	if fileWeights.totalRemainingSize != 0 {
		t.Fatalf("totalRemainingSize is more then 0, totalRemainingSize: %d", fileWeights.totalRemainingSize)
	}

	if fileWeights.totalWeights != 0 {
		t.Fatalf("totalWeights is more then 0, totalWeights: %f", fileWeights.totalWeights)
	}

	if len(fileWeights.weights) != 0 {
		t.Fatalf("weights length is more then 0, weights length: %d", len(fileWeights.weights))
	}
}

func TestUtilsGetGroup(t *testing.T) {
	cases := []struct {
		name          string
		input         int64
		expectedGroup GroupType
		expectedErr   error
	}{
		{
			name:          "TB filesize",
			input:         1_099_511_627_776,
			expectedGroup: TB,
			expectedErr:   nil,
		},
		{
			name:          "GB filesize",
			input:         1_073_741_824,
			expectedGroup: GB,
			expectedErr:   nil,
		},
		{
			name:          "MB filesize",
			input:         1_048_576,
			expectedGroup: MB,
			expectedErr:   nil,
		},
		{
			name:          "KB filesize",
			input:         1024,
			expectedGroup: KB,
			expectedErr:   nil,
		},
		{
			name:          "Invalid filesize",
			input:         0,
			expectedGroup: 0,
			expectedErr:   InvalidFileSize,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			group, err := getGroup(c.input)

			if group != c.expectedGroup {
				t.Fatalf("GroupType mismatch\ngot: %d\nexpected: %d", group, c.expectedGroup)
			}

			if err != c.expectedErr {
				t.Fatalf("error mismatch\ngot: %v\nexpected: %v", err, c.expectedErr)
			}
		})
	}
}
