package bandwidthcontroller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestBandwidthControllerMultipleSameSizeStreams(t *testing.T) {
	const streamSize = 100 * 1020 // 100 KB (1020 for divinding by 3 evenly)
	const streamsAmount = 4
	const bandwidth = streamSize // will take (streamSize * streamsAmount)/bandwidth seconds

	streams := make([]*Stream, streamsAmount)
	bc := NewBandwidthController(bandwidth)
	for i := 0; i < streamsAmount; i++ {
		streams[i], _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), streamSize)
		waitUntilLimitsAreUpdated()
		for j := 0; j <= i; j++ {
			validateBandwidth(t, fmt.Sprintf("lap: #%d stream #%d", i, j), streams[j].GetRateLimit(), bandwidth/int64(i+1))
		}
	}

	if len(bc.streams) != streamsAmount {
		t.Fatalf("unexpected number of stream in the bandwidthContorller, streams: %d expected: %d", len(bc.streams), streamsAmount)
	}

	start := time.Now()
	readAllStreams(t, streams)
	assertReadTimes(t, time.Since(start), streamsAmount, streamsAmount+1)
	validateEmpty(t, bc)
}

func TestBandwidthControllerRateLimit(t *testing.T) {
	const streamSize = 100 * 1024 // 100 KB
	const partsAmount = 3
	const bandwidth = streamSize / partsAmount // streamSize/partsAmount bytes per second

	bc := NewBandwidthController(bandwidth)
	stream, _ := bc.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), streamSize)

	start := time.Now()
	readStream(t, stream)
	assertReadTimes(t, time.Since(start), partsAmount, partsAmount+1)
	validateEmpty(t, bc)
}

func TestBandwidthControllerMaxStreamBandwidth(t *testing.T) {
	const smallStreamSize = 1 * 1024          // 1 KB
	const largeStreamSize = 300 * 1024 * 1024 // 300 MB
	smallStreamMaxBandwidth := getStreamMaxBandwidth(smallStreamSize)
	largeStreamMaxBandwidth := getStreamMaxBandwidth(largeStreamSize)
	bandwidth := (smallStreamMaxBandwidth * 2) + largeStreamMaxBandwidth // total

	bandwidthContorller := NewBandwidthController(bandwidth)

	smallStream1, _ := bandwidthContorller.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	validateBandwidth(t, "first add: smallStream1", smallStream1.GetRateLimit(), smallStreamMaxBandwidth)

	smallStream2, _ := bandwidthContorller.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	validateBandwidth(t, "second add: smallStream1", smallStream1.GetRateLimit(), smallStreamMaxBandwidth)
	validateBandwidth(t, "second add: smallStream2", smallStream2.GetRateLimit(), smallStreamMaxBandwidth)

	largeStream1, _ := bandwidthContorller.AppendStreamReader(bytes.NewReader(make([]byte, largeStreamSize)), largeStreamSize)
	validateBandwidth(t, "third add: smallStream1", smallStream1.GetRateLimit(), smallStreamMaxBandwidth)
	validateBandwidth(t, "third add: smallStream2", smallStream2.GetRateLimit(), smallStreamMaxBandwidth)
	validateBandwidth(t, "third add: largeStream1", largeStream1.GetRateLimit(), largeStreamMaxBandwidth)
}

func TestBandwidthControllerFreeBandwidthAllocation(t *testing.T) {
	const streamSize = 100 * 1024 // 100 KB
	const bandwidth = streamSize
	// this will divide unevenly since 102400 doesn't divide evenly to 3 and we consider the limitedreader deviation
	expectedBandwidthAmounts := map[int64]int{
		34120: 1,
		34140: 2,
	}
	bandwidthAmounts := map[int64]int{
		34120: 0,
		34140: 0,
	}

	bandwidthContorller := NewBandwidthController(bandwidth)
	stream1, _ := bandwidthContorller.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), streamSize)
	stream2, _ := bandwidthContorller.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), streamSize)
	stream3, _ := bandwidthContorller.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), streamSize)

	waitUntilLimitsAreUpdated()
	bandwidthAmounts[stream1.GetRateLimit()]++
	bandwidthAmounts[stream2.GetRateLimit()]++
	bandwidthAmounts[stream3.GetRateLimit()]++

	for b, amount := range expectedBandwidthAmounts {
		if bandwidthAmounts[b] != amount {
			t.Fatalf("amount of %d bandwidth allocated different then expected. amount: %d expected: %d", b, bandwidthAmounts[b], amount)
		}
	}
}

func TestBandwidthControllerAppendStreamsBandwidthAllocation(t *testing.T) {
	const smallStreamSize = 1 * 1024          // 1 KB
	const largeStreamSize = 300 * 1024 * 1024 // 300 MB
	const bandwidth = (smallStreamSize * 2) + largeStreamSize
	expectedSmallStreamBandwidth := getStreamMaxBandwidth(smallStreamSize)
	expectedLargeStreamBandwidth := getStreamBandwidthWithoutDeviation(bandwidth - (expectedSmallStreamBandwidth * 2))

	var smallStream1 *Stream
	var smallStream2 *Stream
	var largeStream1 *Stream
	bc := NewBandwidthController(bandwidth)

	assertExpectedResult := func(testName string) {
		waitUntilLimitsAreUpdated()
		validateBandwidth(t, testName+": smallStream1", smallStream1.GetRateLimit(), expectedSmallStreamBandwidth)
		validateBandwidth(t, testName+": smallStream2", smallStream2.GetRateLimit(), expectedSmallStreamBandwidth)
		validateBandwidth(t, testName+": largeStream1", largeStream1.GetRateLimit(), expectedLargeStreamBandwidth)
		validateBandwidth(t, testName+": KB group allocated bandwidth", bc.stats[KB].ReservedBandwidth, int64(float64(bc.bandwidth)*bc.cfg.MinGroupBandwidthPercentShare[KB]))
		validateBandwidth(t, testName+": KB group used bandwidth", bc.stats[KB].AllocatedBandwidth, expectedSmallStreamBandwidth*2)
		validateBandwidth(t, testName+": MB group allocated bandwidth", bc.stats[MB].ReservedBandwidth, bc.bandwidth-expectedSmallStreamBandwidth*2)
		validateBandwidth(t, testName+": MB group used bandwidth", bc.stats[MB].AllocatedBandwidth, getStreamBandwidthWithoutDeviation(bc.bandwidth-expectedSmallStreamBandwidth*2-bc.freeBandwidth))
		validateBandwidth(t, testName+": GB group allocated bandwidth", bc.stats[GB].ReservedBandwidth, 0)
		validateBandwidth(t, testName+": GB group used bandwidth", bc.stats[GB].AllocatedBandwidth, 0)
		validateBandwidth(t, testName+": TB group allocated bandwidth", bc.stats[TB].ReservedBandwidth, 0)
		validateBandwidth(t, testName+": TB group used bandwidth", bc.stats[TB].AllocatedBandwidth, 0)
	}

	largeStream1, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, largeStreamSize)), largeStreamSize)
	smallStream1, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	smallStream2, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	assertExpectedResult("large first")

	emptyBandwidthController(bc)
	smallStream1, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	smallStream2, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	largeStream1, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, largeStreamSize)), largeStreamSize)
	assertExpectedResult("large last")

	emptyBandwidthController(bc)
	smallStream1, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	largeStream1, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, largeStreamSize)), largeStreamSize)
	smallStream2, _ = bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	assertExpectedResult("large middle")
}

func TestBandwidthControllerStreamsCloseBandwidthAllocation(t *testing.T) {
	const smallStreamSize = 1 * 1024          // 1 KB
	const largeStreamSize = 300 * 1024 * 1024 // 300 MB
	const bandwidth = ((smallStreamSize * 2) + largeStreamSize)
	expectedSmallStreamBandwidth := getStreamMaxBandwidth(smallStreamSize)

	bc := NewBandwidthController(bandwidth)
	smallStream1, _ := bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	smallStream2, _ := bc.AppendStreamReader(bytes.NewReader(make([]byte, smallStreamSize)), smallStreamSize)
	largeStream1, _ := bc.AppendStreamReader(bytes.NewReader(make([]byte, largeStreamSize)), largeStreamSize)

	err := smallStream1.Close()
	if err != nil {
		t.Fatalf("got error while closing smallStream1: %v", err)
	}

	waitUntilLimitsAreUpdated()
	testName := "first close"
	validateBandwidth(t, testName+": smallStream2", smallStream2.GetRateLimit(), expectedSmallStreamBandwidth)
	validateBandwidth(t, testName+": largeStream1", largeStream1.GetRateLimit(), getStreamBandwidthWithoutDeviation(bandwidth-expectedSmallStreamBandwidth))
	validateBandwidth(t, testName+": KB group allocated bandwidth", bc.stats[KB].ReservedBandwidth, int64(float64(bc.bandwidth)*bc.cfg.MinGroupBandwidthPercentShare[KB]))
	validateBandwidth(t, testName+": KB group used bandwidth", bc.stats[KB].AllocatedBandwidth, expectedSmallStreamBandwidth)
	validateBandwidth(t, testName+": MB group allocated bandwidth", bc.stats[MB].ReservedBandwidth, bc.bandwidth-expectedSmallStreamBandwidth)
	validateBandwidth(t, testName+": MB group used bandwidth", bc.stats[MB].AllocatedBandwidth, getStreamBandwidthWithoutDeviation(bc.bandwidth-expectedSmallStreamBandwidth))
	validateBandwidth(t, testName+": GB group allocated bandwidth", bc.stats[GB].ReservedBandwidth, 0)
	validateBandwidth(t, testName+": GB group used bandwidth", bc.stats[GB].AllocatedBandwidth, 0)
	validateBandwidth(t, testName+": TB group allocated bandwidth", bc.stats[TB].ReservedBandwidth, 0)
	validateBandwidth(t, testName+": TB group used bandwidth", bc.stats[TB].AllocatedBandwidth, 0)

	err = smallStream2.Close()
	if err != nil {
		t.Fatalf("got error while closing smallStream2: %v", err)
	}

	waitUntilLimitsAreUpdated()
	testName = "second close"
	validateBandwidth(t, testName+": largeStream1", largeStream1.GetRateLimit(), getStreamBandwidthWithoutDeviation(bandwidth))
	validateBandwidth(t, testName+": KB group allocated bandwidth", bc.stats[KB].ReservedBandwidth, 0)
	validateBandwidth(t, testName+": KB group used bandwidth", bc.stats[KB].AllocatedBandwidth, 0)
	validateBandwidth(t, testName+": MB group allocated bandwidth", bc.stats[MB].ReservedBandwidth, bc.bandwidth)
	validateBandwidth(t, testName+": MB group used bandwidth", bc.stats[MB].AllocatedBandwidth, getStreamBandwidthWithoutDeviation(bc.bandwidth))
	validateBandwidth(t, testName+": GB group allocated bandwidth", bc.stats[GB].ReservedBandwidth, 0)
	validateBandwidth(t, testName+": GB group used bandwidth", bc.stats[GB].AllocatedBandwidth, 0)
	validateBandwidth(t, testName+": TB group allocated bandwidth", bc.stats[TB].ReservedBandwidth, 0)
	validateBandwidth(t, testName+": TB group used bandwidth", bc.stats[TB].AllocatedBandwidth, 0)

	err = largeStream1.Close()
	if err != nil {
		t.Fatalf("got error while closing largeStream1: %v", err)
	}

	validateEmpty(t, bc)
}

func TestBandwidthControllerZeroBandwidthBehavior(t *testing.T) {
	bc := NewBandwidthController(0)
	_, err := bc.AppendStreamReader(nil, 1024)
	if err != InvalidBandwidth {
		t.Fatalf("didn't get InvalidBandwidth error as expected, error: %v", err)
	}
}

func TestBandwidthControllerZeroStreamSizeBehavior(t *testing.T) {
	bc := NewBandwidthController(1024)
	_, err := bc.AppendStreamReader(nil, 0)
	if err != InvalidStreamSize {
		t.Fatalf("didn't get InvalidStreamSize error as expected, error: %v", err)
	}
}

func TestBandwidthControllerContextCancelation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bc := NewBandwidthController(1024, WithContext(ctx))

	smallStream1, err := bc.AppendStreamReader(nil, 1)
	if err != nil {
		t.Fatalf("got error while closing smallStream1: %v", err)
	}

	cancel()

	_, err = bc.AppendStreamReader(nil, 1)
	if err != context.Canceled {
		t.Fatalf("didn't get context.Canceled as expected, error: %v", err)
	}

	err = smallStream1.Close()
	if err != nil {
		t.Fatalf("got error while closing smallStream1: %v", err)
	}

	validateEmpty(t, bc)
}

func TestBandwidthControllerWithConfigMergeDefaults(t *testing.T) {
	defaults := defaultConfig()

	cases := []struct {
		name     string
		input    ControllerConfig
		expected ControllerConfig
	}{
		{
			name:  "no overrides uses defaults",
			input: ControllerConfig{},
			expected: ControllerConfig{
				SchedulerInterval:               defaults.SchedulerInterval,
				MinGroupBandwidthPercentShare:   defaults.MinGroupBandwidthPercentShare,
				MinStreamBandwidthInBytesPerSec: defaults.MinStreamBandwidthInBytesPerSec,
			},
		},
		{
			name: "override only SchedulerInterval",
			input: func() ControllerConfig {
				interval := 500 * time.Millisecond
				return ControllerConfig{SchedulerInterval: &interval}
			}(),
			expected: func() ControllerConfig {
				interval := 500 * time.Millisecond
				return ControllerConfig{
					SchedulerInterval:               &interval,
					MinGroupBandwidthPercentShare:   defaults.MinGroupBandwidthPercentShare,
					MinStreamBandwidthInBytesPerSec: defaults.MinStreamBandwidthInBytesPerSec,
				}
			}(),
		},
		{
			name: "override only MinGroupBandwidthPercentShare",
			input: ControllerConfig{
				MinGroupBandwidthPercentShare: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
			expected: ControllerConfig{
				SchedulerInterval:               defaults.SchedulerInterval,
				MinStreamBandwidthInBytesPerSec: defaults.MinStreamBandwidthInBytesPerSec,
				MinGroupBandwidthPercentShare: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
		},
		{
			name: "override only MinStreamBandwidthInBytesPerSec",
			input: ControllerConfig{
				MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
			},
			expected: ControllerConfig{
				SchedulerInterval:             defaults.SchedulerInterval,
				MinGroupBandwidthPercentShare: defaults.MinGroupBandwidthPercentShare,
				MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
			},
		},
		{
			name: "override both SchedulerInterval and MinStreamBandwidthInBytesPerSec",
			input: func() ControllerConfig {
				interval := 1 * time.Second
				return ControllerConfig{
					SchedulerInterval: &interval,
					MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
						KB: 10,
						MB: 20,
						GB: 30,
						TB: 40,
					},
				}
			}(),
			expected: func() ControllerConfig {
				interval := 1 * time.Second
				return ControllerConfig{
					SchedulerInterval:             &interval,
					MinGroupBandwidthPercentShare: defaults.MinGroupBandwidthPercentShare,
					MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
						KB: 10,
						MB: 20,
						GB: 30,
						TB: 40,
					},
				}
			}(),
		},
		{
			name: "override both MinStreamBandwidthInBytesPerSec and MinGroupBandwidthPercentShare",
			input: ControllerConfig{
				MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
				MinGroupBandwidthPercentShare: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
			expected: ControllerConfig{
				SchedulerInterval: defaults.SchedulerInterval,
				MinStreamBandwidthInBytesPerSec: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
				MinGroupBandwidthPercentShare: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			bc := NewBandwidthController(1024, WithConfig(c.input))

			if !reflect.DeepEqual(bc.cfg, c.expected) {
				t.Fatalf("config mismatch\ngot: %#v\nexpected: %#v", bc.cfg, c.expected)
			}
		})
	}
}

func TestBandwidthControllerStableThroughput(t *testing.T) {
	const streamSize = 1 * 1024 // 1 KB
	const streamAmountPerSecond = 200
	const totalStreamAmount = 1000
	const bandwidth = streamSize * 100 // will take (streamSize * totalStreamAmount)/bandwidth seconds
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	streamSizeUpdateC := make(chan int, 1)
	streamAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendStreams(t, bc, stopC, doneC, streamSize, streamAmountPerSecond, streamSizeUpdateC, streamAmountPerIntervalUpdateC)
	time.Sleep(((totalStreamAmount / streamAmountPerSecond) * time.Second) - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	expectedTime := totalStreamAmount * streamSize / bandwidth
	validateEmpty(t, bc)

	if bc.streamCounter != totalStreamAmount {
		t.Fatalf("stream sent different then expected sent: %d expected: %d", bc.streamCounter, totalStreamAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerAdaptiveThroughput(t *testing.T) {
	const streamSize = 1 * 1024 // 1 KB
	const timeToChangeStreamSizeInSeconds = 2
	const newStreamSize = 3 * 1024 // 3 KB
	const streamAmountPerSecond = 200
	const timeToChangeStreamAmountInSeconds = 1
	const newStreamAmountPerSecond = 100
	const timeToFinishInSeconds = 2
	const bandwidth = 100 * 1024 // 100 KB
	const expectedTotalStreamAmount = (streamAmountPerSecond*
		(timeToChangeStreamSizeInSeconds+timeToChangeStreamAmountInSeconds) +
		newStreamAmountPerSecond*timeToFinishInSeconds)
	const expectedTotalSize = (streamSize*streamAmountPerSecond*timeToChangeStreamSizeInSeconds +
		newStreamSize*streamAmountPerSecond*timeToChangeStreamAmountInSeconds +
		newStreamSize*newStreamAmountPerSecond*timeToFinishInSeconds)
	const expectedTime = expectedTotalSize / bandwidth
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	streamSizeUpdateC := make(chan int, 1)
	streamAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendStreams(t, bc, stopC, doneC, streamSize, streamAmountPerSecond, streamSizeUpdateC, streamAmountPerIntervalUpdateC)
	time.Sleep(timeToChangeStreamSizeInSeconds*time.Second - bufferTime)
	streamSizeUpdateC <- newStreamSize
	time.Sleep(timeToChangeStreamAmountInSeconds*time.Second - bufferTime)
	streamAmountPerIntervalUpdateC <- newStreamAmountPerSecond
	time.Sleep(timeToFinishInSeconds*time.Second - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	validateEmpty(t, bc)

	if bc.streamCounter != expectedTotalStreamAmount {
		t.Fatalf("stream sent different then expected sent: %d expected: %d", bc.streamCounter, expectedTotalStreamAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerAdaptiveBandwidth(t *testing.T) {
	const streamSize = 1 * 1024 // 1 KB
	const streamAmountPerSecond = 200
	const bandwidth = 100 * 1024    // 100 KB
	const newBandwidth = 200 * 1024 // 200 KB
	const timeToChangeBandwidthInSeconds = 2
	const timeToFinishInSeconds = 2
	const expectedTotalStreamAmount = streamAmountPerSecond * (timeToChangeBandwidthInSeconds + timeToFinishInSeconds)
	const expectedTime = (streamSize*streamAmountPerSecond*timeToChangeBandwidthInSeconds/bandwidth +
		streamSize*streamAmountPerSecond*timeToFinishInSeconds/newBandwidth)
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	streamSizeUpdateC := make(chan int, 1)
	streamAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendStreams(t, bc, stopC, doneC, streamSize, streamAmountPerSecond, streamSizeUpdateC, streamAmountPerIntervalUpdateC)
	time.Sleep(timeToChangeBandwidthInSeconds*time.Second - bufferTime)
	bc.UpdateBandwidth(newBandwidth)
	time.Sleep(timeToFinishInSeconds*time.Second - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	validateEmpty(t, bc)

	if bc.streamCounter != expectedTotalStreamAmount {
		t.Fatalf("stream sent different then expected sent: %d expected: %d", bc.streamCounter, expectedTotalStreamAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerBurstRecoveryThroughput(t *testing.T) {
	const streamSize = 1 * 1024 // 1 KB
	const streamAmountPerSecond = 200
	const burstStreamAmountPerSecond = 1000
	const timeToBurstInSeconds = 2
	const timeOfBurstInSeconds = 1
	const timeToFinishInSeconds = 2
	const bandwidth = 100 * 1024 // 100 KB
	const expectedTotalStreamAmount = (streamAmountPerSecond*
		(timeToBurstInSeconds+timeToFinishInSeconds) +
		burstStreamAmountPerSecond*timeOfBurstInSeconds)
	const expectedTotalSize = streamSize * expectedTotalStreamAmount
	const expectedTime = expectedTotalSize / bandwidth
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	streamSizeUpdateC := make(chan int, 1)
	streamAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendStreams(t, bc, stopC, doneC, streamSize, streamAmountPerSecond, streamSizeUpdateC, streamAmountPerIntervalUpdateC)
	time.Sleep(timeToBurstInSeconds*time.Second - bufferTime)
	streamAmountPerIntervalUpdateC <- burstStreamAmountPerSecond
	time.Sleep(timeOfBurstInSeconds*time.Second - bufferTime)
	streamAmountPerIntervalUpdateC <- streamAmountPerSecond
	time.Sleep(timeToFinishInSeconds*time.Second - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	validateEmpty(t, bc)

	if bc.streamCounter != expectedTotalStreamAmount {
		t.Fatalf("stream sent different then expected sent: %d expected: %d", bc.streamCounter, expectedTotalStreamAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerStallingReader(t *testing.T) {
	const streamSize = 100 * 1024 // 100 KB
	const partsAmount = 3
	const bandwidth = streamSize / 3 // streamSize/partsAmount bytes per second
	const timeToStallInSeconds = 1
	const timeOfStallInSeconds = 2
	const expectedTime = streamSize/bandwidth + timeOfStallInSeconds

	bc := NewBandwidthController(bandwidth)
	stream, _ := bc.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), streamSize)
	buffer := make([]byte, 1024)

	waitUntilLimitsAreUpdated()
	validateBandwidth(t, "stream", stream.GetRateLimit(), getStreamBandwidthWithoutDeviation(bandwidth))

	start := time.Now()
	totalSize := 0
	for {
		if time.Since(start) > timeToStallInSeconds*time.Second &&
			time.Since(start) < (timeToStallInSeconds+timeOfStallInSeconds)*time.Second {
			time.Sleep(timeOfStallInSeconds * time.Second)
			continue
		}

		n, err := stream.Read(buffer)
		totalSize += n
		if err != nil {
			if err != io.EOF {
				t.Fatalf("unexpected error while reading: %v", err)
			}
			break
		}
	}

	if int64(totalSize) != stream.Size {
		t.Fatalf("read incomplete data, read: %d expected: %d", totalSize, stream.Size)
	}

	err := stream.Close()
	if err != nil {
		t.Fatalf("unexpected error while closing: %v", err)
	}

	assertReadTimes(t, time.Since(start), expectedTime, expectedTime+1)
	validateEmpty(t, bc)
}

func readAllStreams(t *testing.T, streams []*Stream) {
	var wg sync.WaitGroup
	for _, f := range streams {
		wg.Add(1)
		stream := f
		go func() {
			readStream(t, stream)
			wg.Done()
		}()
	}

	wg.Wait()
}

func readStream(t *testing.T, stream *Stream) {
	n, err := io.Copy(io.Discard, stream)

	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error while reading: %v", err)
	}

	if n != stream.Size {
		t.Fatalf("read incomplete data, read: %d expected: %d", n, stream.Size)
	}

	err = stream.Close()
	if err != nil {
		t.Fatalf("unexpected error while closing: %v", err)
	}
}

func assertReadTimes(t *testing.T, elapsed time.Duration, minTimeInSeconds, maxTimeInSeconds int) {
	fmt.Printf("Took %v\n", elapsed)
	minTime := time.Duration(minTimeInSeconds) * time.Second
	maxTime := time.Duration(maxTimeInSeconds) * time.Second
	if elapsed.Abs().Round(time.Second) < minTime { // round to second - has a deviation of up to half a second
		t.Errorf("read completed too quickly, elapsed time: %v < min time: %v", elapsed, minTime)
	} else if elapsed.Abs().Round(time.Second) > maxTime { // round to second - has a deviation of up to half a second
		t.Errorf("read completed too slow, elapsed time: %v > max time: %v", elapsed, maxTime)
	}
}

func validateEmpty(t *testing.T, bc *BandwidthController) {
	validateGroupEmpty(t, bc, KB, "KB")
	validateGroupEmpty(t, bc, MB, "MB")
	validateGroupEmpty(t, bc, GB, "GB")
	validateGroupEmpty(t, bc, TB, "TB")
}

func validateGroupEmpty(t *testing.T, bc *BandwidthController, group GroupType, groupName string) {
	if len(bc.streams[group]) != 0 {
		t.Fatalf("unexpected number of %s group streams left in the bandwidthContorller, left: %d expected: 0", groupName, len(bc.streams[group]))
	}
}

func validateBandwidth(t *testing.T, name string, bandwidth, expectedBandwidth int64) {
	if bandwidth != expectedBandwidth {
		t.Fatalf("%s appointed bandwidth different then expected. bandwidth: %d expected: %d", name, bandwidth, expectedBandwidth)
	}
}

func waitUntilLimitsAreUpdated() {
	time.Sleep(*defaultConfig().SchedulerInterval + (2 * time.Millisecond))
}

func emptyBandwidthController(bc *BandwidthController) {
	bc.streamsInSystems = 0
	for g, _ := range bc.streams {
		bc.streams[g] = make(map[int64]*Stream)
	}
}

func continuouslyAppendStreams(t *testing.T, bc *BandwidthController,
	stopC, doneC chan struct{},
	startingStreamSize, startingStreamAmountPerInterval int,
	streamSizeUpdateC, streamAmountPerIntervalUpdateC chan int) {

	var wg sync.WaitGroup
	streamSize := startingStreamSize
	streamAmountPerInterval := startingStreamAmountPerInterval

	// start sending immediately
	sendC := make(chan struct{}, 1)
	sendC <- struct{}{}

	start := time.Now()
	ticker := time.NewTicker(time.Second)
	for {
		select {
		case <-stopC:
			ticker.Stop()
			wg.Wait()
			doneC <- struct{}{}
			return
		case streamSize = <-streamSizeUpdateC:
		case streamAmountPerInterval = <-streamAmountPerIntervalUpdateC:
		case <-ticker.C:
			sendC <- struct{}{}
		case <-sendC:
			if streamSize <= 0 || streamAmountPerInterval <= 0 {
				continue
			}

			fmt.Printf("sending! streamAmount=%d streamSize=%d elapsed Milliseconds=%d\n", streamAmountPerInterval, streamSize, time.Since(start).Milliseconds())
			for i := 0; i < streamAmountPerInterval; i++ {
				wg.Add(1)
				go appendAndReadStream(t, bc, streamSize, &wg)
			}
		}
	}
}

func appendAndReadStream(t *testing.T, bc *BandwidthController, streamSize int, wg *sync.WaitGroup) {
	stream, _ := bc.AppendStreamReader(bytes.NewReader(make([]byte, streamSize)), int64(streamSize))
	readStream(t, stream)
	wg.Done()
}
