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
			name:                               "only KB streams",
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
			name:                               "both KB and GB streams",
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
			name:                               "all group types streams",
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
			streams := map[GroupType]BandwidthGroup{
				KB: generateGroupStreams(c.expectedGroupsAmounts[KB], c.expectedGroupsTotalSizes[KB]),
				MB: generateGroupStreams(c.expectedGroupsAmounts[MB], c.expectedGroupsTotalSizes[MB]),
				GB: generateGroupStreams(c.expectedGroupsAmounts[GB], c.expectedGroupsTotalSizes[GB]),
				TB: generateGroupStreams(c.expectedGroupsAmounts[TB], c.expectedGroupsTotalSizes[TB]),
			}

			weights, overallGroupsRemainingSize := getGroupsSortedWeights(streams)

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

func generateGroupStreams(streamsAmount int, streamsTotalSize int64) BandwidthGroup {
	streams := make(BandwidthGroup)
	for i := 0; i < streamsAmount; i++ {
		streams[int64(i)] = NewStream(NewStreamReadCloser(nil, 0, nil), streamsTotalSize/int64(streamsAmount))
	}

	return streams
}

func validateWeightsPerGroup(t *testing.T, group GroupType, groupName string,
	weights map[GroupType]streamWeights,
	expectedGroupsAmounts map[GroupType]int,
	expectedGroupsTotalSizes map[GroupType]int64) {
	if len(weights[group].weights) != expectedGroupsAmounts[group] {
		t.Fatalf("groupAmount %s mismatch\ngot: %d\nexpected: %d", groupName, len(weights[group].weights), expectedGroupsAmounts[group])
	}

	if weights[group].totalRemainingSize != expectedGroupsTotalSizes[group] {
		t.Fatalf("groupTotalRemainingSize %s mismatch\ngot: %d\nexpected: %d", groupName, weights[group].totalRemainingSize, expectedGroupsTotalSizes[group])
	}
}

func TestUtilsGetStreamsSortedWeights(t *testing.T) {
	streams := make(BandwidthGroup)
	var expectedTotalWeights float64
	var expectedTotalRemainingSize int64
	for i := 5; i > 0; i-- {
		streams[int64(i)] = NewStream(NewStreamReadCloser(nil, 0, nil), int64(i))
		expectedTotalWeights += 1.0 / float64(i)
		expectedTotalRemainingSize += int64(i)
	}

	streamWeights := getStreamsSortedWeights(streams)

	if streamWeights.totalRemainingSize != expectedTotalRemainingSize {
		t.Fatalf("totalRemainingSize is different then expected totalRemainingSize: %d expected: %d", streamWeights.totalRemainingSize, expectedTotalRemainingSize)
	}

	if streamWeights.totalWeights != expectedTotalWeights {
		t.Fatalf("totalWeights is different then expected totalWeights: %f expected: %f", streamWeights.totalWeights, expectedTotalWeights)
	}

	for i := 0; i < 5; i++ {
		if streamWeights.weights[i].weight != 1.0/float64(i+1) {
			t.Fatalf("weights is not sorted current weight: %f expected: %f", streamWeights.weights[i].weight, 1.0/float64(i+1))
		}
	}
}

func TestUtilsGetStreamsSortedWeightsEmptyOnFinish(t *testing.T) {
	streams := make(BandwidthGroup)
	for i := 5; i > 0; i-- {
		streams[int64(i)] = NewStream(NewStreamReadCloser(nil, 0, nil), 0)
	}

	streamWeights := getStreamsSortedWeights(streams)

	if streamWeights.totalRemainingSize != 0 {
		t.Fatalf("totalRemainingSize is more then 0, totalRemainingSize: %d", streamWeights.totalRemainingSize)
	}

	if streamWeights.totalWeights != 0 {
		t.Fatalf("totalWeights is more then 0, totalWeights: %f", streamWeights.totalWeights)
	}

	if len(streamWeights.weights) != 0 {
		t.Fatalf("weights length is more then 0, weights length: %d", len(streamWeights.weights))
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
			name:          "TB stream size",
			input:         1_099_511_627_776,
			expectedGroup: TB,
			expectedErr:   nil,
		},
		{
			name:          "GB stream size",
			input:         1_073_741_824,
			expectedGroup: GB,
			expectedErr:   nil,
		},
		{
			name:          "MB stream size",
			input:         1_048_576,
			expectedGroup: MB,
			expectedErr:   nil,
		},
		{
			name:          "KB stream size",
			input:         1024,
			expectedGroup: KB,
			expectedErr:   nil,
		},
		{
			name:          "Invalid stream size",
			input:         0,
			expectedGroup: 0,
			expectedErr:   InvalidStreamSize,
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
