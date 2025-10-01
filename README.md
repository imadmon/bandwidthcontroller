# Bandwidth Controller

`bandwidthcontroller` is a high-performance Go library for **dynamic bandwidth management** across multiple concurrent `io.Reader` streams.

It enforces a **global bandwidth cap (bytes/sec)** and automatically redistributes available bandwidth among active readers in real time, leveraging the capabilities of [`imadmon/limitedreader`](https://github.com/imadmon/limitedreader) for adaptive rate limiting.

This ensures fair throughput, prevents starvation, and simplifies handling of many concurrent, variably sized streams efficiently without manually managing per-stream limits.

> Check out the [bandwidth control mastery article here](https://github.com/imadmon/bandwidthcontroller) - explainig everything about how this library algorithms.

</br>

## How it works
You initialize the controller with a total bandwidth cap (bytes/sec). Every time you add a stream, it returns a rate limited reader that adapts in real-time.</br>
Under the hood, the controller automatically adjusts the rate limits as streams are added, read from or closed, redistributing available bandwidth as needed.

</br>

## Installation

``` Bash
go get github.com/imadmon/bandwidthcontroller
```

</br>

## Usage Example

``` Go
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
```

</br>

## Features

- **Global bandwidth cap**: Enforce a single throughput limit (bytes/sec) across all streams.
- **Dynamic allocation**: New readers can join at any time and receive a bandwidth-aware io.ReadCloser whose limits are adjusted on-the-fly.
- **Bandwidth groups by size**: Streams are categorized into KB, MB, GB, and TB groups to ensure fairness and prevent starvation.
- **Fairness + efficiency**: Each group and stream has a minimum guaranteed bandwidth, while maximizing use of limitedreader and returning unused capacity to the global pool.
- **Centralized lightweight scheduler**: A monitoring loop periodically updates allocations without adding per-read overhead.
- **Context support**: Integrates with context.Context for cancellation and control.
- **Statistics export**: Retrieve real-time metrics per group for visibility and tuning.

</br>

## Monitoring

The controller provides several functions to give you **full transparency** into bandwidth allocation and runtime behavior:

- `GetTotalStreamsInSystem()` - total number of streams created since startup.
- `GetCurrentStreamsInSystem()` - number of currently active streams.
- `GetStatistics()` - returns map[GroupType]BandwidthStats with per-group statistics.

### Statistics

Per-group statistics let you monitor reserved bandwidth, actual allocations, and per-pulse activity.
```Go
type BandwidthStats struct {
    ReservedBandwidth          int64
    AllocatedBandwidth         int64
    CurrentPulseIntervalAmount int64
    CurrentPulseIntervalSize   int64
    PulseIntervalsStats        *PulsesStats // 1 second stats
}

const PulsesIntervalsAmount = 5

type PulsesStats struct {
    TotalAmount    int64
    TotalSize      int64
    PulseAvgAmount int64
    PulseAvgSize   int64
    PulseIntervals [PulsesIntervalsAmount]*PulseStats // index 0 == newest
}

type PulseStats struct {
    NewStreamsAmount    int64
    NewStreamsTotalSize int64
}
```
- **ReservedBandwidth** - bandwidth guaranteed from configuration or group minimums.
- **AllocatedBandwidth** - bandwidth actually granted after redistribution and `limitedreader` optimizations.
- **CurrentPulseInterval*** - counters for the current update cycle.
- **PulseIntervalsStats** - rolling per-second stats, tracking how many streams and bytes were added across recent pulses.

### Pulse Interval

Allocations are updated periodically by the controller’s scheduler. The default interval is **200ms**, exposed as `SchedulerInterval`.

</br>

## Configuration

The library provides a configuration struct, but **most users should stick with the defaults** - change these values only if you fully understand their effect.


``` Go
type ControllerConfig struct {
    SchedulerInterval               *time.Duration
    MinGroupBandwidthPercentShare   map[GroupType]float64 // values in [0.01, 1.00]
    MinStreamBandwidthInBytesPerSec map[GroupType]int64
}
```

- **SchedulerInterval** - default 200ms; controls how often the monitor loop redistributes bandwidth and updates statistics.
- **MinGroupBandwidthPercentShare** - minimum share of global bandwidth that each group receives (if it has at least one active stream).
- **MinStreamBandwidthInBytesPerSec** - minimum bandwidth guaranteed for each stream.

Both minimum bandwidth settings exist to **prevent starvation**, ensuring that even small or low-priority streams always get a fair allocation.

</br>

## Dependencies

[`github.com/imadmon/limitedreader`](https://github.com/imadmon/limitedreader) (by the same author)

</br>

## Author

Created by [@IdanMadmon](https://github.com/imadmon) for developers who juggle unpredictable streams like pros.
