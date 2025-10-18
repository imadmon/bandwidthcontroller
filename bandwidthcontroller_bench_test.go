package bandwidthcontroller

import (
	"bytes"
	"fmt"
	"io"
	"math"
	"testing"
	"time"
)

func BenchmarkAppendStreamReader(b *testing.B) {
	for _, existing := range []int{0, 10, 100, 1000} {
		b.Run(fmt.Sprintf("existing=%d", existing), func(b *testing.B) {
			bc := NewBandwidthController(10 << 20)
			r := bytes.NewReader(make([]byte, 1024))

			// pre-populate with 'existing' open streams
			for i := 0; i < existing; i++ {
				lr, err := bc.AppendStreamReader(r, 1024)
				if err != nil {
					b.Fatalf("append existing stream failed: %v", err)
				}

				// keep streams open to simulate contention
				_ = lr
			}

			b.ReportAllocs()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				lr, err := bc.AppendStreamReader(r, 1024)
				if err != nil {
					b.Fatalf("append stream failed: %v", err)
				}

				b.StopTimer()
				_ = lr.Close() // keep 'existing' constant across iterations
				b.StartTimer()
			}
		})
	}
}

func BenchmarkRateLimitAccuracy(b *testing.B) {
	streamSize := int64(32 << 20) // 32 MB
	bandwidths := []int64{
		streamSize / 4,                    // should take 4 seconds
		streamSize,                        // should take 1 seconds
		getStreamMaxBandwidth(streamSize), // should take 0 seconds
	}
	for _, bandwidth := range bandwidths {
		b.Run(fmt.Sprintf("bandwidth_MB=%d", bandwidth>>20), func(b *testing.B) {
			bc := NewBandwidthController(bandwidth)
			lr, err := bc.AppendStreamReader(&infiniteReader{size: int(streamSize)}, streamSize)
			if err != nil {
				b.Fatalf("append stream failed: %v", err)
			}
			defer lr.Close()

			var totalBytes int64
			buffer := make([]byte, 32<<10) // 32 KB

			b.ReportAllocs()
			start := time.Now()
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				n, err := lr.Read(buffer)
				totalBytes += int64(n)
				if err == io.EOF {
					break
				} else if err != nil {
					b.Fatalf("read failed: %v", err)
				}
			}

			b.StopTimer()
			elapsed := time.Since(start)
			totalBytesMB := float64(totalBytes) / 1024.0 / 1024.0
			b.ReportMetric(totalBytesMB, "MB_read")
			b.ReportMetric(totalBytesMB/elapsed.Seconds(), "MB_read_per_second")
		})
	}
}

func BenchmarkStreamMaxThroughput(b *testing.B) {
	bandwidth := int64(math.MaxInt64) // large limit - no limit
	bc := NewBandwidthController(bandwidth)
	lr, err := bc.AppendStreamReader(&infiniteReader{size: int(bandwidth)}, bandwidth)
	if err != nil {
		b.Fatalf("append stream failed: %v", err)
	}
	defer lr.Close()

	var totalBytes int64
	buffer := make([]byte, 32<<10) // 32 KB

	b.ReportAllocs()
	start := time.Now()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		n, err := lr.Read(buffer)
		totalBytes += int64(n)
		if err == io.EOF {
			break
		} else if err != nil {
			b.Fatalf("read failed: %v", err)
		}
	}

	b.StopTimer()
	elapsed := time.Since(start)
	totalBytesMB := float64(totalBytes) / 1024.0 / 1024.0
	b.ReportMetric(totalBytesMB, "MB_read")
	b.ReportMetric(totalBytesMB/elapsed.Seconds(), "MB_read_per_second")
}

type infiniteReader struct {
	size int
	i    int
}

func (ir *infiniteReader) Read(p []byte) (int, error) {
	if ir.i >= ir.size {
		return 0, io.EOF
	}

	var i int
	for i = range p {
		p[i] = 'A'
	}
	i++

	ir.i += i
	if ir.i >= ir.size {
		i -= ir.size - ir.i
		return i, io.EOF
	}

	return i, nil
}
