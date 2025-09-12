package bandwidthcontroller

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestBandwidthControllerMultipleSameSizeFiles(t *testing.T) {
	const fileSize = 100 * 1024 // 100 KB
	const filesAmount = 4
	const bandwidth = fileSize // will take (fileSize * filesAmount)/bandwidth seconds

	files := make([]*File, filesAmount)
	bc := NewBandwidthController(int64(bandwidth))
	for i := 0; i < filesAmount; i++ {
		files[i], _ = bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), int64(fileSize))
		waitUntilLimitsAreUpdated()
		validateFileBandwidth(t, fmt.Sprintf("file #%d", i), files[i].Reader.GetRateLimit(), int64(bandwidth/(i+1)))
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

	bc := NewBandwidthController(int64(bandwidth))
	file, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), int64(fileSize))

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

	bandwidthContorller := NewBandwidthController(int64(bandwidth))

	smallFile1, _ := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	validateFileBandwidth(t, "first add: smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)

	smallFile2, _ := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	validateFileBandwidth(t, "second add: smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateFileBandwidth(t, "second add: smallFile2", smallFile2.Reader.GetRateLimit(), smallFileMaxBandwidth)

	largeFile1, _ := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	validateFileBandwidth(t, "third add: smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateFileBandwidth(t, "third add: smallFile2", smallFile2.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateFileBandwidth(t, "third add: largeFile1", largeFile1.Reader.GetRateLimit(), largeFileMaxBandwidth)
}

func TestBandwidthControllerAppendFilesBandwidthAllocation(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	smallFileMaxBandwidth := getFileMaxBandwidth(smallFileSize)
	bandwidth := (smallFileSize * 2) + largeFileSize

	var smallFile1 *File
	var smallFile2 *File
	var largeFile1 *File
	bc := NewBandwidthController(int64(bandwidth))

	assertExpectedResult := func(testName string) {
		waitUntilLimitsAreUpdated()
		validateFileBandwidth(t, testName+": smallFile1", smallFile1.Reader.GetRateLimit(), smallFileMaxBandwidth)
		validateFileBandwidth(t, testName+": smallFile2", smallFile2.Reader.GetRateLimit(), smallFileMaxBandwidth)
		validateFileBandwidth(t, testName+": largeFile1", largeFile1.Reader.GetRateLimit(), int64(bandwidth)-(smallFileMaxBandwidth*2))
	}

	largeFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	smallFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	smallFile2, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	assertExpectedResult("large first")

	emptyBandwidthController(bc)
	smallFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	smallFile2, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	largeFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	assertExpectedResult("large last")

	emptyBandwidthController(bc)
	smallFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	largeFile1, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	smallFile2, _ = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	assertExpectedResult("large middle")
}

func TestBandwidthControllerFilesCloseBandwidthAllocation(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	smallFileMaxBandwidth := getFileMaxBandwidth(smallFileSize)
	bandwidth := ((smallFileSize * 2) + largeFileSize)

	bc := NewBandwidthController(int64(bandwidth))
	smallFile1, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	smallFile2, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	largeFile1, _ := bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))

	err := smallFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	waitUntilLimitsAreUpdated()
	validateFileBandwidth(t, "first close: smallFile2", smallFile2.Reader.GetRateLimit(), smallFileMaxBandwidth)
	validateFileBandwidth(t, "first close: largeFile1", largeFile1.Reader.GetRateLimit(), int64(bandwidth)-smallFileMaxBandwidth)

	err = smallFile2.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile2: %v", err)
	}

	waitUntilLimitsAreUpdated()
	validateFileBandwidth(t, "second close: largeFile1", largeFile1.Reader.GetRateLimit(), int64(bandwidth))

	err = largeFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing largeFile1: %v", err)
	}

	validateEmpty(t, bc)
}

func TestBandwidthControllerZeroFileSizeBehavior(t *testing.T) {
	bc := NewBandwidthController(0)
	_, err := bc.AppendFileReader(nil, 0)
	if err != InvalidFileSize {
		t.Fatalf("didn't get InvalidFileSize error as expected, error: %v", err)
	}
}

func TestBandwidthControllerContextCancelation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	bc := NewBandwidthController(0, WithContext(ctx))

	smallFile1, err := bc.AppendFileReader(nil, 0)
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	cancel()

	_, err = bc.AppendFileReader(nil, 0)
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
			bc := NewBandwidthController(0, WithConfig(c.input))

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

	bc := NewBandwidthController(int64(bandwidth))
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	fileSizeUpdateC := make(chan int, 1)
	fileAmountPerIntervalUpdateC := make(chan int, 1)

	start := time.Now()

	go continuouslyAppendFiles(t, bc, stopC, doneC, fileSize, fileAmountPerSecond, fileSizeUpdateC, fileAmountPerIntervalUpdateC)
	time.Sleep(((totalFileAmount / fileAmountPerSecond) * time.Second) - 500*time.Millisecond)
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

func validateFileBandwidth(t *testing.T, fileName string, fileBandwidth, expectedBandwidth int64) {
	expectedBandwidth = getFileBandwidthWithoutDeviation(expectedBandwidth)

	// consider deviation of 1 (remainder)
	if math.Abs(float64(fileBandwidth-expectedBandwidth)) > 1 {
		t.Fatalf("%s appointed bandwidth different then expected. bandwidth: %d expected: %d", fileName, fileBandwidth, expectedBandwidth)
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
