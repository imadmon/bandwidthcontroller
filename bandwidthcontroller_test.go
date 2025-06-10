package bandwidthcontroller

import (
	"bytes"
	"fmt"
	"io"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestBandwidthController_BasicRead(t *testing.T) {
	dataSize := 100_000 // 100 KB of data
	filesAmount := 4
	files := make([]*File, filesAmount)
	bandwidth := dataSize

	bandwidthContorller := NewBandwidthController(int64(bandwidth))
	for i := 0; i < filesAmount; i++ {
		files[i] = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, dataSize)), int64(dataSize))
		if files[i].Reader.GetRateLimit() != int64(bandwidth/(i+1)) {
			t.Fatalf("file #%d got unexpected ratelimit: %d expected: %d", i, files[i].Reader.GetRateLimit(), bandwidth/(i+1))
		}
	}

	if len(bandwidthContorller.files) != filesAmount {
		t.Fatalf("unexpected number of file in the bandwidthContorller, files: %d expected: %d", len(bandwidthContorller.files), filesAmount)
	}

	readAllFiles(t, files, filesAmount, filesAmount+1)

	// time.Sleep(2 * time.Second) // since we're using locks it may take more time to sync the threads
	if len(bandwidthContorller.files) != 0 {
		t.Fatalf("unexpected number of file left in the bandwidthContorller, left: %d expected: 0", len(bandwidthContorller.files))
	}
}

func TestBandwidthController_RateLimit(t *testing.T) {
	dataSize := 100_000 // 100 KB of data
	partsAmount := 3
	bandwidth := dataSize / partsAmount

	bandwidthContorller := NewBandwidthController(int64(bandwidth))
	file := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, dataSize)), int64(dataSize))

	start := time.Now()
	readFile(t, file)
	assertReadTimes(t, time.Since(start), partsAmount, partsAmount+1)
}

func TestBandwidthController_MaxFileBandwidth(t *testing.T) {
	smallDataSize := 1_000       // 1 KB of data
	largeDataSize := 300_000_000 // 300 MB of data
	bandwidth := (smallDataSize * 2) + largeDataSize

	bandwidthContorller := NewBandwidthController(int64(bandwidth))

	smallFile1 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	if int(smallFile1.Reader.GetRateLimit()) != smallDataSize {
		t.Fatalf("smallFile1 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile1.Reader.GetRateLimit(), smallDataSize)
	}

	smallFile2 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	if int(smallFile1.Reader.GetRateLimit()) != smallDataSize {
		t.Fatalf("smallFile1 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile1.Reader.GetRateLimit(), smallDataSize)
	}

	if int(smallFile2.Reader.GetRateLimit()) != smallDataSize {
		t.Fatalf("smallFile2 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile2.Reader.GetRateLimit(), smallDataSize)
	}

	largeFile1 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeDataSize)), int64(largeDataSize))
	if int(smallFile1.Reader.GetRateLimit()) != smallDataSize {
		t.Fatalf("smallFile1 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile1.Reader.GetRateLimit(), smallDataSize)
	}

	if int(smallFile2.Reader.GetRateLimit()) != smallDataSize {
		t.Fatalf("smallFile2 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile2.Reader.GetRateLimit(), smallDataSize)
	}

	if int(largeFile1.Reader.GetRateLimit()) != largeDataSize {
		t.Fatalf("largeFile1 appointed bandwidth different then file size bandwidth: %d expected: %d", largeFile1.Reader.GetRateLimit(), largeDataSize)
	}
}

func TestBandwidthController_FilesBandwidthAllocation(t *testing.T) {
	smallDataSize := 1_000       // 1 KB of data
	largeDataSize := 300_000_000 // 300 MB of data
	partsAmount := 4             // from the large file
	bandwidth := ((smallDataSize * 2) + largeDataSize) / partsAmount

	var largeFile1 *File
	var smallFile1 *File
	var smallFile2 *File
	bandwidthContorller := NewBandwidthController(int64(bandwidth))

	assertExpectedResult := func() {
		if int(smallFile1.Reader.GetRateLimit()) != smallDataSize {
			t.Fatalf("smallFile1 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile1.Reader.GetRateLimit(), smallDataSize)
		}

		if int(smallFile2.Reader.GetRateLimit()) != smallDataSize {
			t.Fatalf("smallFile2 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile2.Reader.GetRateLimit(), smallDataSize)
		}

		if int(largeFile1.Reader.GetRateLimit()) != bandwidth-(smallDataSize*2) {
			t.Fatalf("largeFile1 appointed bandwidth different then expected bandwidth: %d expected: %d", largeFile1.Reader.GetRateLimit(), largeDataSize)
		}
	}

	largeFile1 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeDataSize)), int64(largeDataSize))
	smallFile1 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	smallFile2 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	assertExpectedResult()

	bandwidthContorller.files = make(map[uuid.UUID]*File)
	smallFile1 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	smallFile2 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	largeFile1 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeDataSize)), int64(largeDataSize))
	assertExpectedResult()

	bandwidthContorller.files = make(map[uuid.UUID]*File)
	smallFile1 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	largeFile1 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeDataSize)), int64(largeDataSize))
	smallFile2 = bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	assertExpectedResult()
}

func TestBandwidthController_FilesCloseBandwidthAllocation(t *testing.T) {
	smallDataSize := 1_000       // 1 KB of data
	largeDataSize := 300_000_000 // 300 MB of data
	partsAmount := 4             // from the large file
	bandwidth := ((smallDataSize * 2) + largeDataSize) / partsAmount

	bandwidthContorller := NewBandwidthController(int64(bandwidth))
	smallFile1 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	smallFile2 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallDataSize)), int64(smallDataSize))
	largeFile1 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeDataSize)), int64(largeDataSize))

	err := smallFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	if int(smallFile2.Reader.GetRateLimit()) != smallDataSize {
		t.Fatalf("smallFile2 appointed bandwidth different then file size bandwidth: %d expected: %d", smallFile2.Reader.GetRateLimit(), smallDataSize)
	}

	if int(largeFile1.Reader.GetRateLimit()) != bandwidth-smallDataSize {
		t.Fatalf("largeFile1 appointed bandwidth different then expected bandwidth: %d expected: %d", largeFile1.Reader.GetRateLimit(), largeDataSize)
	}

	err = smallFile2.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile2: %v", err)
	}

	if int(largeFile1.Reader.GetRateLimit()) != bandwidth {
		t.Fatalf("largeFile1 appointed bandwidth different then file size bandwidth: %d expected: %d", largeFile1.Reader.GetRateLimit(), largeDataSize)
	}
}

func readAllFiles(t *testing.T, files []*File, minTimeInSeconds, maxTimeInSeconds int) {
	var wg sync.WaitGroup
	start := time.Now()
	for _, f := range files {
		wg.Add(1)
		file := f
		go readOneFile(t, file, &wg)
	}

	wg.Wait()
	assertReadTimes(t, time.Since(start), minTimeInSeconds, maxTimeInSeconds)
}

func readOneFile(t *testing.T, file *File, wg *sync.WaitGroup) {
	readFile(t, file)
	wg.Done()
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
