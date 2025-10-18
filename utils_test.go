package bandwidthcontroller

import (
	"math"
	"testing"
)

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

func TestUtilsGetStreamMaxBandwidth(t *testing.T) {
	if getStreamMaxBandwidth(1) != 20 {
		t.Fatalf("got unexpected MaxBandwidth for size 1, got: %d expected: %d", getStreamMaxBandwidth(1), 20)
	}

	if getStreamMaxBandwidth(math.MaxInt64) != math.MaxInt64 {
		t.Fatalf("got unexpected MaxBandwidth for size math.MaxInt64, got: %d expected: %d", getStreamMaxBandwidth(math.MaxInt64), math.MaxInt64)
	}
}
