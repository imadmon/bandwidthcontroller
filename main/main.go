package main

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/imadmon/bandwidthcontroller"
)

func main() {
	// Initialize controller with target total bandwidth in bytes per second
	var bandwidth int64 = 100_000 // ~100KB/sec
	bc := bandwidthcontroller.NewBandwidthController(bandwidth)

	// Example streams (simulate with strings)
	data1 := strings.NewReader(strings.Repeat("A", 500_000)) // ~500KB
	data2 := strings.NewReader(strings.Repeat("B", 300_000)) // ~300KB

	// Register the readers with the controller
	r1, _ := bc.AppendStreamReader(data1, 500_000)
	r2, _ := bc.AppendStreamReader(data2, 300_000)

	// Read concurrently
	fmt.Println("Starting...")
	start := time.Now()
	read := func(name string, r io.ReadCloser) {
		written, _ := io.Copy(io.Discard, r)
		_ = r.Close()
		fmt.Printf("%s finished, bytes: %d, after: %v\n", name, written, time.Since(start))
	}

	go read("Stream1", r1)
	go read("Stream2", r2)

	// Let them flow for a while
	time.Sleep(10 * time.Second)
	fmt.Println("Finished...")
}
