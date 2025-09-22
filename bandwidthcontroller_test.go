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

func TestBandwidthControllerMultipleSameSizeFiles(t *testing.T) {
	const fileSize = 100 * 1024 // 100 KB
	const filesAmount = 4
	const bandwidth = fileSize // will take (fileSize * filesAmount)/bandwidth seconds

	expectedBandwidthAllocation := map[int]map[int]int64{
		0: {
			0: 102400,
		},
		1: {
			0: 51200,
			1: 51200,
		},
		2: {
			0: 34120,
			1: 34140,
			2: 34140,
		},
		3: {
			0: 25600,
			1: 25600,
			2: 25600,
			3: 25600,
		},
	}

	files := make([]*File, filesAmount)
	bc := NewBandwidthController(bandwidth)
	for i := 0; i < filesAmount; i++ {
		files[i], _ = bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), fileSize)
		waitUntilLimitsAreUpdated()
		for j := 0; j <= i; j++ {
			validateBandwidth(t, fmt.Sprintf("lap: #%d file #%d", i, j), files[j].Reader.GetRateLimit(), expectedBandwidthAllocation[i][j])
		}
	}

	if len(bc.files) != filesAmount {
		t.Fatalf("unexpected number of file in the bandwidthContorller, files: %d expected: %d", len(bc.files), filesAmount)
	}

	start := time.Now()
	readAllFiles(t, files)
	assertReadTimes(t, time.Since(start), filesAmount, filesAmount+1)
	validateEmpty(t, bc)
}

func TestBandwidthControllerRateLimit(t *testing.T) {
	const fileSize = 100 * 1024 // 100 KB
	const partsAmount = 3
	const bandwidth = fileSize / partsAmount // fileSize/partsAmount bytes per second

	bc := NewBandwidthController(bandwidth)
	file, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), fileSize)

	start := time.Now()
	readFile(t, file)
	assertReadTimes(t, time.Since(start), partsAmount, partsAmount+1)
	validateEmpty(t, bc)
}

func TestBandwidthControllerMaxFileBandwidth(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	smallFileMaxBandwidth := getFileMaxBandwidth(smallFileSize)
	largeFileMaxBandwidth := getFileMaxBandwidth(largeFileSize)
	bandwidth := (smallFileMaxBandwidth * 2) + largeFileMaxBandwidth // total

	bandwidthContorller := NewBandwidthController(bandwidth)

	smallFile1, _ := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	validateBandwidth(t, "first add: smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)

	smallFile2, _ := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	validateBandwidth(t, "second add: smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateBandwidth(t, "second add: smallFile2", smallFile2.Reader.GetRateLimit(), smallFileMaxBandwidth)

	largeFile1, _ := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), largeFileSize)
	validateBandwidth(t, "third add: smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateBandwidth(t, "third add: smallFile2", smallFile2.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateBandwidth(t, "third add: largeFile1", largeFile1.Reader.GetRateLimit(), largeFileMaxBandwidth)
}

func TestBandwidthControllerAppendFilesBandwidthAllocation(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	const bandwidth = (smallFileSize * 2) + largeFileSize
	expectedSmallFileBandwidth := getFileMaxBandwidth(smallFileSize)
	expectedLargeFileBandwidth := getFileBandwidthWithoutDeviation(bandwidth - (expectedSmallFileBandwidth * 2))

	var smallFile1 *File
	var smallFile2 *File
	var largeFile1 *File
	bc := NewBandwidthController(bandwidth)

	assertExpectedResult := func(testName string) {
		waitUntilLimitsAreUpdated()
		validateBandwidth(t, testName+": smallFile1", smallFile1.Reader.GetRateLimit(), expectedSmallFileBandwidth)
		validateBandwidth(t, testName+": smallFile2", smallFile2.Reader.GetRateLimit(), expectedSmallFileBandwidth)
		validateBandwidth(t, testName+": largeFile1", largeFile1.Reader.GetRateLimit(), expectedLargeFileBandwidth)
		validateBandwidth(t, testName+": KB group", bc.groupsBandwidth[KB], expectedSmallFileBandwidth*2)
		validateBandwidth(t, testName+": MB group", bc.groupsBandwidth[MB], bc.bandwidth-expectedSmallFileBandwidth*2-bc.freeBandwidth)
		validateBandwidth(t, testName+": GB group", bc.groupsBandwidth[GB], 0)
		validateBandwidth(t, testName+": TB group", bc.groupsBandwidth[TB], 0)
	}

	largeFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), largeFileSize)
	smallFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	smallFile2, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	assertExpectedResult("large first")

	emptyBandwidthController(bc)
	smallFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	smallFile2, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	largeFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), largeFileSize)
	assertExpectedResult("large last")

	emptyBandwidthController(bc)
	smallFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	largeFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), largeFileSize)
	smallFile2, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	assertExpectedResult("large middle")
}

func TestBandwidthControllerFilesCloseBandwidthAllocation(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	const bandwidth = ((smallFileSize * 2) + largeFileSize)
	expectedSmallFileBandwidth := getFileMaxBandwidth(smallFileSize)

	bc := NewBandwidthController(bandwidth)
	smallFile1, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	smallFile2, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), smallFileSize)
	largeFile1, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), largeFileSize)

	err := smallFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	waitUntilLimitsAreUpdated()
	testName := "first close"
	validateBandwidth(t, testName+": smallFile2", smallFile2.Reader.GetRateLimit(), expectedSmallFileBandwidth)
	validateBandwidth(t, testName+": largeFile1", largeFile1.Reader.GetRateLimit(), getFileBandwidthWithoutDeviation(bandwidth-expectedSmallFileBandwidth))
	validateBandwidth(t, testName+": KB group", bc.groupsBandwidth[KB], expectedSmallFileBandwidth)
	validateBandwidth(t, testName+": MB group", bc.groupsBandwidth[MB], bc.bandwidth-expectedSmallFileBandwidth-bc.freeBandwidth)
	validateBandwidth(t, testName+": GB group", bc.groupsBandwidth[GB], 0)
	validateBandwidth(t, testName+": TB group", bc.groupsBandwidth[TB], 0)

	err = smallFile2.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile2: %v", err)
	}

	waitUntilLimitsAreUpdated()
	testName = "second close"
	validateBandwidth(t, testName+": largeFile1", largeFile1.Reader.GetRateLimit(), getFileBandwidthWithoutDeviation(bandwidth))
	validateBandwidth(t, testName+": KB group", bc.groupsBandwidth[KB], 0)
	validateBandwidth(t, testName+": MB group", bc.groupsBandwidth[MB], bc.bandwidth-bc.freeBandwidth)
	validateBandwidth(t, testName+": GB group", bc.groupsBandwidth[GB], 0)
	validateBandwidth(t, testName+": TB group", bc.groupsBandwidth[TB], 0)

	err = largeFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing largeFile1: %v", err)
	}

	validateEmpty(t, bc)
}

func TestBandwidthControllerZeroBandwidthBehavior(t *testing.T) {
	bc := NewBandwidthController(0)
	_, err := bc.AppendFileReader(nil, 1024)
	if err != InvalidBandwidth {
		t.Fatalf("didn't get InvalidBandwidth error as expected, error: %v", err)
	}
}

func TestBandwidthControllerZeroFileSizeBehavior(t *testing.T) {
	bc := NewBandwidthController(1024)
	_, err := bc.AppendFileReader(nil, 0)
	if err != InvalidFileSize {
		t.Fatalf("didn't get InvalidFileSize error as expected, error: %v", err)
	}
}

func TestBandwidthControllerContextCancelation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bc := NewBandwidthController(1024, WithContext(ctx))

	smallFile1, err := bc.AppendFileReader(nil, 1)
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	cancel()

	_, err = bc.AppendFileReader(nil, 1)
	if err != context.Canceled {
		t.Fatalf("didn't get context.Canceled as expected, error: %v", err)
	}

	err = smallFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	validateEmpty(t, bc)
}

func TestBandwidthControllerWithConfigMergeDefaults(t *testing.T) {
	defaults := defaultConfig()

	cases := []struct {
		name     string
		input    Config
		expected Config
	}{
		{
			name:  "no overrides uses defaults",
			input: Config{},
			expected: Config{
				BandwidthUpdaterInterval:    defaults.BandwidthUpdaterInterval,
				MinGroupBandwidthPercentage: defaults.MinGroupBandwidthPercentage,
				MinFileBandwidthInBytes:     defaults.MinFileBandwidthInBytes,
			},
		},
		{
			name: "override only BandwidthUpdaterInterval",
			input: func() Config {
				interval := 500 * time.Millisecond
				return Config{BandwidthUpdaterInterval: &interval}
			}(),
			expected: func() Config {
				interval := 500 * time.Millisecond
				return Config{
					BandwidthUpdaterInterval:    &interval,
					MinGroupBandwidthPercentage: defaults.MinGroupBandwidthPercentage,
					MinFileBandwidthInBytes:     defaults.MinFileBandwidthInBytes,
				}
			}(),
		},
		{
			name: "override only MinGroupBandwidthPercentage",
			input: Config{
				MinGroupBandwidthPercentage: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
			expected: Config{
				BandwidthUpdaterInterval: defaults.BandwidthUpdaterInterval,
				MinFileBandwidthInBytes:  defaults.MinFileBandwidthInBytes,
				MinGroupBandwidthPercentage: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
		},
		{
			name: "override only MinFileBandwidthInBytes",
			input: Config{
				MinFileBandwidthInBytes: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
			},
			expected: Config{
				BandwidthUpdaterInterval:    defaults.BandwidthUpdaterInterval,
				MinGroupBandwidthPercentage: defaults.MinGroupBandwidthPercentage,
				MinFileBandwidthInBytes: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
			},
		},
		{
			name: "override both BandwidthUpdaterInterval and MinFileBandwidthInBytes",
			input: func() Config {
				interval := 1 * time.Second
				return Config{
					BandwidthUpdaterInterval: &interval,
					MinFileBandwidthInBytes: map[GroupType]int64{
						KB: 10,
						MB: 20,
						GB: 30,
						TB: 40,
					},
				}
			}(),
			expected: func() Config {
				interval := 1 * time.Second
				return Config{
					BandwidthUpdaterInterval:    &interval,
					MinGroupBandwidthPercentage: defaults.MinGroupBandwidthPercentage,
					MinFileBandwidthInBytes: map[GroupType]int64{
						KB: 10,
						MB: 20,
						GB: 30,
						TB: 40,
					},
				}
			}(),
		},
		{
			name: "override both MinFileBandwidthInBytes and MinGroupBandwidthPercentage",
			input: Config{
				MinFileBandwidthInBytes: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
				MinGroupBandwidthPercentage: map[GroupType]float64{
					KB: 0.10,
					MB: 0.20,
					GB: 0.30,
					TB: 0.40,
				},
			},
			expected: Config{
				BandwidthUpdaterInterval: defaults.BandwidthUpdaterInterval,
				MinFileBandwidthInBytes: map[GroupType]int64{
					KB: 10,
					MB: 20,
					GB: 30,
					TB: 40,
				},
				MinGroupBandwidthPercentage: map[GroupType]float64{
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
	const fileSize = 1 * 1024 // 1 KB
	const fileAmountPerSecond = 200
	const totalFileAmount = 1000
	const bandwidth = fileSize * 100 // will take (fileSize * totalFileAmount)/bandwidth seconds
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	fileSizeUpdateC := make(chan int, 1)
	fileAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendFiles(t, bc, stopC, doneC, fileSize, fileAmountPerSecond, fileSizeUpdateC, fileAmountPerIntervalUpdateC)
	time.Sleep(((totalFileAmount / fileAmountPerSecond) * time.Second) - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	expectedTime := totalFileAmount * fileSize / bandwidth
	validateEmpty(t, bc)

	if bc.fileCounter != totalFileAmount {
		t.Fatalf("file sent different then expected sent: %d expected: %d", bc.fileCounter, totalFileAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerAdaptiveThroughput(t *testing.T) {
	const fileSize = 1 * 1024 // 1 KB
	const timeToChangeFileSizeInSeconds = 2
	const newFileSize = 3 * 1024 // 3 KB
	const fileAmountPerSecond = 200
	const timeToChangeFileAmountInSeconds = 1
	const newFileAmountPerSecond = 100
	const timeToFinishInSeconds = 2
	const bandwidth = 100 * 1024 // 100 KB
	const expectedTotalFileAmount = (fileAmountPerSecond*
		(timeToChangeFileSizeInSeconds+timeToChangeFileAmountInSeconds) +
		newFileAmountPerSecond*timeToFinishInSeconds)
	const expectedTotalSize = (fileSize*fileAmountPerSecond*timeToChangeFileSizeInSeconds +
		newFileSize*fileAmountPerSecond*timeToChangeFileAmountInSeconds +
		newFileSize*newFileAmountPerSecond*timeToFinishInSeconds)
	const expectedTime = expectedTotalSize / bandwidth
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	fileSizeUpdateC := make(chan int, 1)
	fileAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendFiles(t, bc, stopC, doneC, fileSize, fileAmountPerSecond, fileSizeUpdateC, fileAmountPerIntervalUpdateC)
	time.Sleep(timeToChangeFileSizeInSeconds*time.Second - bufferTime)
	fileSizeUpdateC <- newFileSize
	time.Sleep(timeToChangeFileAmountInSeconds*time.Second - bufferTime)
	fileAmountPerIntervalUpdateC <- newFileAmountPerSecond
	time.Sleep(timeToFinishInSeconds*time.Second - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	validateEmpty(t, bc)

	if bc.fileCounter != expectedTotalFileAmount {
		t.Fatalf("file sent different then expected sent: %d expected: %d", bc.fileCounter, expectedTotalFileAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerAdaptiveBandwidth(t *testing.T) {
	const fileSize = 1 * 1024 // 1 KB
	const fileAmountPerSecond = 200
	const bandwidth = 100 * 1024    // 100 KB
	const newBandwidth = 200 * 1024 // 200 KB
	const timeToChangeBandwidthInSeconds = 2
	const timeToFinishInSeconds = 2
	const expectedTotalFileAmount = fileAmountPerSecond * (timeToChangeBandwidthInSeconds + timeToFinishInSeconds)
	const expectedTime = (fileSize*fileAmountPerSecond*timeToChangeBandwidthInSeconds/bandwidth +
		fileSize*fileAmountPerSecond*timeToFinishInSeconds/newBandwidth)
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	fileSizeUpdateC := make(chan int, 1)
	fileAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendFiles(t, bc, stopC, doneC, fileSize, fileAmountPerSecond, fileSizeUpdateC, fileAmountPerIntervalUpdateC)
	time.Sleep(timeToChangeBandwidthInSeconds*time.Second - bufferTime)
	bc.UpdateBandwidth(newBandwidth)
	time.Sleep(timeToFinishInSeconds*time.Second - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	validateEmpty(t, bc)

	if bc.fileCounter != expectedTotalFileAmount {
		t.Fatalf("file sent different then expected sent: %d expected: %d", bc.fileCounter, expectedTotalFileAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerBurstRecoveryThroughput(t *testing.T) {
	const fileSize = 1 * 1024 // 1 KB
	const fileAmountPerSecond = 200
	const burstFileAmountPerSecond = 1000
	const timeToBurstInSeconds = 2
	const timeOfBurstInSeconds = 1
	const timeToFinishInSeconds = 2
	const bandwidth = 100 * 1024 // 100 KB
	const expectedTotalFileAmount = (fileAmountPerSecond*
		(timeToBurstInSeconds+timeToFinishInSeconds) +
		burstFileAmountPerSecond*timeOfBurstInSeconds)
	const expectedTotalSize = fileSize * expectedTotalFileAmount
	const expectedTime = expectedTotalSize / bandwidth
	const bufferTime = 100 * time.Millisecond

	bc := NewBandwidthController(bandwidth)
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	fileSizeUpdateC := make(chan int, 1)
	fileAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendFiles(t, bc, stopC, doneC, fileSize, fileAmountPerSecond, fileSizeUpdateC, fileAmountPerIntervalUpdateC)
	time.Sleep(timeToBurstInSeconds*time.Second - bufferTime)
	fileAmountPerIntervalUpdateC <- burstFileAmountPerSecond
	time.Sleep(timeOfBurstInSeconds*time.Second - bufferTime)
	fileAmountPerIntervalUpdateC <- fileAmountPerSecond
	time.Sleep(timeToFinishInSeconds*time.Second - bufferTime)
	stopC <- struct{}{}
	<-doneC

	elapsed := time.Since(start)
	validateEmpty(t, bc)

	if bc.fileCounter != expectedTotalFileAmount {
		t.Fatalf("file sent different then expected sent: %d expected: %d", bc.fileCounter, expectedTotalFileAmount)
	}

	assertReadTimes(t, elapsed, expectedTime, expectedTime+1)
}

func TestBandwidthControllerStallingReader(t *testing.T) {
	const fileSize = 100 * 1024 // 100 KB
	const partsAmount = 3
	const bandwidth = fileSize / 3 // fileSize/partsAmount bytes per second
	const timeToStallInSeconds = 1
	const timeOfStallInSeconds = 2
	const expectedTime = fileSize/bandwidth + timeOfStallInSeconds

	bc := NewBandwidthController(bandwidth)
	file, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), fileSize)
	buffer := make([]byte, 1024)

	waitUntilLimitsAreUpdated()
	validateBandwidth(t, "file", file.Reader.GetRateLimit(), getFileBandwidthWithoutDeviation(bandwidth))

	start := time.Now()
	totalSize := 0
	for {
		if time.Since(start) > timeToStallInSeconds*time.Second &&
			time.Since(start) < (timeToStallInSeconds+timeOfStallInSeconds)*time.Second {
			time.Sleep(timeOfStallInSeconds * time.Second)
			continue
		}

		n, err := file.Reader.Read(buffer)
		totalSize += n
		if err != nil {
			if err != io.EOF {
				t.Fatalf("unexpected error while reading: %v", err)
			}
			break
		}
	}

	if int64(totalSize) != file.Size {
		t.Fatalf("read incomplete data, read: %d expected: %d", totalSize, file.Size)
	}

	err := file.Reader.Close()
	if err != nil {
		t.Fatalf("unexpected error while closing: %v", err)
	}

	assertReadTimes(t, time.Since(start), expectedTime, expectedTime+1)
	validateEmpty(t, bc)
}

func readAllFiles(t *testing.T, files []*File) {
	var wg sync.WaitGroup
	for _, f := range files {
		wg.Add(1)
		file := f
		go func() {
			readFile(t, file)
			wg.Done()
		}()
	}

	wg.Wait()
}

func readFile(t *testing.T, file *File) {
	n, err := io.Copy(io.Discard, file.Reader)

	if err != nil && err != io.EOF {
		t.Fatalf("unexpected error while reading: %v", err)
	}

	if n != file.Size {
		t.Fatalf("read incomplete data, read: %d expected: %d", n, file.Size)
	}

	err = file.Reader.Close()
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
	if len(bc.files[group]) != 0 {
		t.Fatalf("unexpected number of %s group files left in the bandwidthContorller, left: %d expected: 0", groupName, len(bc.files[group]))
	}
}

func validateBandwidth(t *testing.T, name string, bandwidth, expectedBandwidth int64) {
	if bandwidth != expectedBandwidth {
		t.Fatalf("%s appointed bandwidth different then expected. bandwidth: %d expected: %d", name, bandwidth, expectedBandwidth)
	}
}

func waitUntilLimitsAreUpdated() {
	time.Sleep(*defaultConfig().BandwidthUpdaterInterval + (2 * time.Millisecond))
}

func emptyBandwidthController(bc *BandwidthController) {
	bc.filesInSystems = 0
	for g, _ := range bc.files {
		bc.files[g] = make(map[int64]*File)
	}
}

func continuouslyAppendFiles(t *testing.T, bc *BandwidthController,
	stopC, doneC chan struct{},
	startingFileSize, startingFileAmountPerInterval int,
	fileSizeUpdateC, fileAmountPerIntervalUpdateC chan int) {

	var wg sync.WaitGroup
	fileSize := startingFileSize
	fileAmountPerInterval := startingFileAmountPerInterval

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
		case fileSize = <-fileSizeUpdateC:
		case fileAmountPerInterval = <-fileAmountPerIntervalUpdateC:
		case <-ticker.C:
			sendC <- struct{}{}
		case <-sendC:
			if fileSize <= 0 || fileAmountPerInterval <= 0 {
				continue
			}

			fmt.Printf("sending! fileAmount=%d fileSize=%d elapsed Milliseconds=%d\n", fileAmountPerInterval, fileSize, time.Since(start).Milliseconds())
			for i := 0; i < fileAmountPerInterval; i++ {
				wg.Add(1)
				go appendAndReadFile(t, bc, fileSize, &wg)
			}
		}
	}
}

func appendAndReadFile(t *testing.T, bc *BandwidthController, fileSize int, wg *sync.WaitGroup) {
	file, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), int64(fileSize))
	readFile(t, file)
	wg.Done()
}
