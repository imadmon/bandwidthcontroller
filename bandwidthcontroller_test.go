package bandwidthcontroller

import (
	"bytes"
	"fmt"
	"io"
	"math"
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
		files[i] = bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), int64(fileSize))
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
	file := bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), int64(fileSize))

	start := time.Now()
	readFile(t, file)
	assertReadTimes(t, time.Since(start), partsAmount, partsAmount+1)
	validateEmpty(t, bc)
}

func TestBandwidthControllerMaxFileBandwidth(t *testing.T) {
	const smallFileSize = 1 * 1024                   // 1 KB
	const largeFileSize = 300 * 1024 * 1024          // 300 MB
	bandwidth := (smallFileSize * 2) + largeFileSize // total

	bandwidthContorller := NewBandwidthController(int64(bandwidth))

	smallFile1 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	validateFileBandwidth(t, "first add: smallFile1", smallFile1.Reader.GetRateLimit(), int64(smallFileSize))

	smallFile2 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	validateFileBandwidth(t, "second add: smallFile1", smallFile1.Reader.GetRateLimit(), int64(smallFileSize))
	validateFileBandwidth(t, "second add: smallFile2", smallFile2.Reader.GetRateLimit(), int64(smallFileSize))

	largeFile1 := bandwidthContorller.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	validateFileBandwidth(t, "third add: smallFile1", smallFile1.Reader.GetRateLimit(), int64(smallFileSize))
	validateFileBandwidth(t, "third add: smallFile2", smallFile2.Reader.GetRateLimit(), int64(smallFileSize))
	validateFileBandwidth(t, "third add: largeFile1", largeFile1.Reader.GetRateLimit(), int64(largeFileSize))
}

func TestBandwidthControllerAppendFilesBandwidthAllocation(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	const partsAmount = 4
	bandwidth := ((smallFileSize * 2) + largeFileSize) / partsAmount

	var smallFile1 *File
	var smallFile2 *File
	var largeFile1 *File
	bc := NewBandwidthController(int64(bandwidth))

	assertExpectedResult := func(testName string) {
		validateFileBandwidth(t, testName+": smallFile1", smallFile1.Reader.GetRateLimit(), int64(smallFileSize))
		validateFileBandwidth(t, testName+": smallFile2", smallFile2.Reader.GetRateLimit(), int64(smallFileSize))
		validateFileBandwidth(t, testName+": largeFile1", largeFile1.Reader.GetRateLimit(), int64(bandwidth-(smallFileSize*2)))
	}

	largeFile1 = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	smallFile1 = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	smallFile2 = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	assertExpectedResult("large first")

	emptyBandwidthController(bc)
	smallFile1 = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	smallFile2 = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	largeFile1 = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	assertExpectedResult("large last")

	emptyBandwidthController(bc)
	smallFile1 = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	largeFile1 = bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))
	smallFile2 = bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	assertExpectedResult("large middle")
}

func TestBandwidthControllerFilesCloseBandwidthAllocation(t *testing.T) {
	const smallFileSize = 1 * 1024          // 1 KB
	const largeFileSize = 300 * 1024 * 1024 // 300 MB
	const partsAmount = 4
	bandwidth := ((smallFileSize * 2) + largeFileSize) / partsAmount

	bc := NewBandwidthController(int64(bandwidth))
	smallFile1 := bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	smallFile2 := bc.AppendFileReader(bytes.NewReader(make([]byte, smallFileSize)), int64(smallFileSize))
	largeFile1 := bc.AppendFileReader(bytes.NewReader(make([]byte, largeFileSize)), int64(largeFileSize))

	err := smallFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile1: %v", err)
	}

	validateFileBandwidth(t, "first close: smallFile2", smallFile2.Reader.GetRateLimit(), int64(smallFileSize))
	validateFileBandwidth(t, "first close: largeFile1", largeFile1.Reader.GetRateLimit(), int64(bandwidth-smallFileSize))

	err = smallFile2.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing smallFile2: %v", err)
	}

	validateFileBandwidth(t, "second close: largeFile1", largeFile1.Reader.GetRateLimit(), int64(bandwidth))

	err = largeFile1.Reader.Close()
	if err != nil {
		t.Fatalf("got error while closing largeFile1: %v", err)
	}

	validateEmpty(t, bc)
}

func TestBandwidthControllerStableThroughput(t *testing.T) {
	const fileSize = 1 * 1024 // 1 KB
	const fileAmountPerSecond = 2
	const totalFileAmount = 10
	const bandwidth = fileSize // will take (fileSize * totalFileAmount)/bandwidth seconds

	bc := NewBandwidthController(int64(bandwidth))
	stopC := make(chan struct{})
	doneC := make(chan struct{})
	fileSizeUpdateC := make(chan int, 1)
	fileAmountPerIntervalUpdateC := make(chan int, 1)
	fileSizeUpdateC <- fileSize
	fileAmountPerIntervalUpdateC <- fileAmountPerSecond

	start := time.Now()

	go continuouslyAppendFiles(t, bc, stopC, doneC, fileSizeUpdateC, fileAmountPerIntervalUpdateC)
	time.Sleep((totalFileAmount / fileAmountPerSecond) * time.Second)
	stopC <- struct{}{}
	<-doneC

	assertReadTimes(t, time.Since(start), totalFileAmount, totalFileAmount+1)
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
	minTime := time.Duration(minTimeInSeconds) * time.Second
	maxTime := time.Duration(maxTimeInSeconds) * time.Second
	if elapsed.Abs().Round(time.Second) < minTime { // round to second - has a deviation of up to half a second
		t.Errorf("read completed too quickly, elapsed time: %v < min time: %v", elapsed, minTime)
	} else if elapsed.Abs().Round(time.Second) > maxTime { // round to second - has a deviation of up to half a second
		t.Errorf("read completed too slow, elapsed time: %v > max time: %v", elapsed, maxTime)
	}
}

func validateEmpty(t *testing.T, bc *BandwidthController) {
	if len(bc.files) != 0 {
		t.Fatalf("unexpected number of file left in the bandwidthContorller, left: %d expected: 0", len(bc.files))
	}
}

func validateFileBandwidth(t *testing.T, fileName string, fileBandwidth, expectedBandwidth int64) {
	// consider deviation of 1 (remainder)
	if math.Abs(float64(fileBandwidth-expectedBandwidth)) > 1 {
		t.Fatalf("%s appointed bandwidth different then expected. bandwidth: %d expected: %d", fileName, fileBandwidth, expectedBandwidth)
	}
}

func emptyBandwidthController(bc *BandwidthController) {
	bc.files = make(map[int32]*File)
}

func continuouslyAppendFiles(t *testing.T, bc *BandwidthController, stopC, doneC chan struct{}, fileSizeUpdateC, fileAmountPerIntervalUpdateC chan int) {
	var fileSize int
	var fileAmountPerInterval int
	var i int
	var wg sync.WaitGroup
	ticker := time.NewTicker(time.Second)
	start := time.Now()
	for {
		select {
		case <-stopC:
			fmt.Printf("stop was called after: %v\n", time.Since(start))
			ticker.Stop()
			wg.Wait()
			fmt.Printf("finish after: %v\n", time.Since(start))
			doneC <- struct{}{}
			return
		case size := <-fileSizeUpdateC:
			fileSize = size
		case amountPerInterval := <-fileAmountPerIntervalUpdateC:
			fileAmountPerInterval = amountPerInterval
		case <-ticker.C:
			i++
			fmt.Printf("sending files #%d\n", i)
			if fileSize <= 0 || fileAmountPerInterval <= 0 {
				continue
			}

			for i := 0; i < fileAmountPerInterval; i++ {
				wg.Add(1)
				go appendAndReadFile(t, bc, fileSize, &wg)
			}
		}
	}
}

func appendAndReadFile(t *testing.T, bc *BandwidthController, fileSize int, wg *sync.WaitGroup) {
	file := bc.AppendFileReader(bytes.NewReader(make([]byte, fileSize)), int64(fileSize))
	readFile(t, file)
	wg.Done()
}
