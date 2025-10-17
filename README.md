# Bandwidth Controller

**Global adaptive bandwidth control for concurrent I/O streams in Go.**
`bandwidthcontroller` automatically enforces a global bandwidth cap (bytes/sec) and redistributes available bandwidth among active readers in real-time - leveraging [`imadmon/limitedreader`](https://github.com/imadmon/limitedreader) for precise, adaptive per-stream pacing.

</br>

## Overview & Core Concepts

`bandwidthcontroller` is designed for scenarios where many concurrent I/O streams must share a single bandwidth limit fairly and efficiently, without starvation or unused bandwidth.
It coordinates multiple `io.ReadCloser`s under one controller, ensuring the total bandwidth never exceeds the configured cap while staying fully utilized.

### Core Mechanics
| Concept                              | Description                                                                                                                      |
| ------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------- |
| **Global Cap**                       | Maximum total throughput (bytes/second) shared across all active streams.                                                        |
| **Dynamic Allocation**       | New streams can join at any time and receive a bandwidth-aware `io.ReadCloser` whose limits are adjusted on-the-fly.      |
| **Groups (KB / MB / GB / TB)**       | Streams are categorized by their size into **Bandwidth Groups**. These use binary multiples (1024 bytes).      |
| **Low Overhead**                     | Centralized lightweight scheduling minimizes per-read locking and CPU usage.                                                                 |
| **Scheduler Pulse**                  | Every fixed interval (default = **200 ms**), the controller redistributes bandwidth between **active** streams.         |
| **Fairness & Starvation Prevention** | Each group and stream has a guaranteed minimum bandwidth share, unused capacity is automatically reallocated to active streams.  |
| **Allocation Efficiency** | Calculates the maximum efficient rate limit that can be applied by `limitedreader`, preventing unnecessary bandwidth allocation.  |
| **Adaptive Redistribution**          | When streams start, finish, or idle, allocations instantly rebalance.                                 |
| **Thread-Safe**                       | All public methods are thread-safe. |
| **Statistics**                       | The controller provides per-group usage data - including bandwidth allocations, throughput, active counts - via `Stats()`. |
| **Context Support**                  | Supports `context.Context` for graceful shutdown and cancellation.                                                               |


### Features
- Single global bandwidth limit for all streams
- Fair and adaptive bandwidth redistribution
- Categorizing streams into groups for starvation prevention
- Real-time statistics for visibility and tuning
- Low overhead, centralized scheduler
- Thread-safe and concurrent
- Context-aware for cancellation and lifecycle management

<br>

> Check out the [Bandwidth Control Mastery](https://github.com/imadmon/bandwidthcontroller) article - 
> a deep dive into the design principles and algorithms behind this library.

</br>

## Quickstart

### Install
``` Bash
go get github.com/imadmon/bandwidthcontroller
```

### Basic example
``` Go
package main

import (
	"fmt"
	"io"
	"log"
	"strings"
	"time"

	"github.com/imadmon/bandwidthcontroller"
)

func main() {
	// Create a controller with a global cap of ~100 KB/s.
	bc := bandwidthcontroller.NewBandwidthController(100_000)

	// Create new reader with size of ~500 KB, should take ~5 seconds.
	r := strings.NewReader(strings.Repeat("A", 500_000))
	lr, err := bc.AppendStreamReader(r, 500_000)
	if err != nil {
		log.Fatal(err)
	}
	defer lr.Close()

	// Copy data under bandwidth limitations
	start := time.Now()
	n, err := io.Copy(io.Discard, lr)
	if err != nil {
		log.Fatal(err)
	}
	
	fmt.Printf("copied bytes: %d, took: %v\n", n, time.Since(start))
}
```

That’s it! One controller, one global limit, automatically balanced across all active streams.

> See [`examples dir`](https://github.com/imadmon/bandwidthcontroller/examples) for runnable demonstrations.

</br>

## How It Works

You initialize the controller with a **total bandwidth cap** (bytes/second).
Each time you add a stream, it returns a **bandwidth-aware reader** that adapts in real-time as data flows.

#### Under the Hood:
1. The controller classifies new streams into **bandwidth groups**.
2. The controller can instantly allocate from the free-bandwidth pool at stream appending if any is available.
3. The controller tracks stream activation.
4. Every scheduler pulse (default **200 ms**), it recalculates fair bandwidth shares per group for active streams.
5. Redistribute bandwidth among **active** streams, reclaiming unused capacity from idle ones.
6. Ensure fair allocation by applying group and stream minimum bandwidth share to **prevent starvation**.
7. Calculates the maximum most efficient rate limit per reader that can be applied using `limitedreader`, and returns any unnecessary bandwidth to the _free-bandwidth_ pool.
8. Bandwidth allocations are applied to each reader via [`imadmon/limitedreader`](https://github.com/imadmon/limitedreader).
9. When streams close or idle, their unused bandwidth is redistributed to others instantly.

#### This Model Ensures:
- **Fair throughput** across all active readers.
- **Starvation prevention** even for small or slow streams.
- **Maximum utilization** of available bandwidth.
- **Simplicity** - no per-stream tuning or micromanagement required.

</br>

## Use Cases

Ideal for systems that need adaptive, shared I/O throttling:

- **Parallel downloads/uploads** - cap total bandwidth for many concurrent transfers.
- **Storage replication** - throttle total network/disk usage across workers.
- **Streaming pipelines** - multiple concurrent readers under one throughput budget.
- **Backup utilities** - global throttling to prevent saturating a network.
- **Simulation/testing** - emulate bandwidth constraints dynamically.

Each stream is independent yet cooperatively limited by a single global controller.

</br>

## API Overview
| Function / Type                                                                     | Description                                                     |
| ----------------------------------------------------------------------------------- | --------------------------------------------------------------- |
| `NewBandwidthController(bandwidth int64, opts ...Option)`                      | Create a new controller with the given global cap.              |
| `AppendStreamReader(src io.Reader, expectedSizeBytes int64) (io.ReadCloser, error)` | Wraps a reader with adaptive bandwidth control.                 |
| `UpdateBandwidth(bandwidth int64) error`                                                   | Update the global cap **instantly**.                        |
| `Stats() ControllerStats`                                      | Returns real-time statistics.                         |
| `ControllerConfig`                                                                  | Configuration struct for tuning scheduler intervals and shares. |

</br>

## Monitoring

The controller continuously tracks internal metrics and exposes them via `Stats()` method.
Provides **full transparency** into bandwidth allocation, stream activity, and scheduler behavior.

### ControllerStats
System statistics as well as per-group statistics. </br>
`Stats()` returns a `ControllerStats` struct, giving you access to the real-time bandwidth metrics:

``` Go
type ControllerStats struct {
	TotalStreams  int64
	OpenStreams   int64
	ActiveStreams int64
	GroupsStats   map[GroupType]*BandwidthStats
}
```

- **TotalStreams** - total number of streams created since controller startup.
- **OpenStreams** - number of streams currently in the system (not yet closed).
- **ActiveStreams** - number of streams that are actively reading.
- **GroupsStats** - per-group statistics.

### BandwidthStats
Per-group statistics let you monitor reserved bandwidth, actual allocations, and per-pulse activity.

``` Go
type BandwidthStats struct {
    ReservedBandwidth          int64
    AllocatedBandwidth         int64
    CurrentPulseIntervalAmount int64
    CurrentPulseIntervalSize   int64
    PulseIntervalsStats        *PulsesStats // 1 second stats
}
```

- **ReservedBandwidth** - bandwidth guaranteed from configuration or group minimums.
- **AllocatedBandwidth** - bandwidth actually granted after redistribution and `limitedreader` optimizations.
- **CurrentPulseIntervalAmount** - number of streams processed during the current update cycle.
- **CurrentPulseIntervalSize** - counter for the current update cycle total streams size.
- **PulseIntervalsStats** - rolling per-second stats, tracking how many streams and bytes were added across recent pulses.

### Pulse Interval

Rolling window of recent pulse intervals.
Allocations are updated periodically by the controller’s scheduler.
The default rolling window covers 5 scheduler pulses (1 second with 200 ms intervals), can be changed via `SchedulerInterval` in config.

``` Go
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

</br>

## Configuration

The library provides a configuration struct `ControllerConfig`, but **most users should stick with the defaults** - change these values only if you fully understand their effect.

``` Go
type ControllerConfig struct {
    SchedulerInterval               *time.Duration
	StreamIdleTimeout               *time.Duration
    MinGroupBandwidthPercentShare   map[GroupType]float64 // values in [0.01, 1.00]
    MinStreamBandwidthInBytesPerSec map[GroupType]int64
}
```

| Field                             | Default | Description                                                |
| --------------------------------- | ------- | ---------------------------------------------------------- |
| `SchedulerInterval`               | 200 ms  | How often the scheduler redistributes bandwidth allocation and updates statistics. |
| `StreamIdleTimeout`               | 100 ms  | Duration of inactivity after which a stream is considered idle. |
| `MinGroupBandwidthPercentShare`   | KB: 5% </br> MB: 5% </br> GB: 5% </br> TB: 5% | Minimum guaranteed bandwidth share per group (if has at least one active stream).              |
| `MinStreamBandwidthInBytesPerSec` | KB: 0.1 KiB/s </br> MB: 0.1 MiB/s </br> GB: 0.1 GiB/s </br> TB: 0.1 TiB/s | Minimum guaranteed rate per stream.                        |


Values use **binary multiples** (1 KiB = 1024 bytes).
If minimums exceed the total bandwidth cap, the controller automatically normalizes them proportionally.

</br>

## Dependencies
[![Go Reference](https://pkg.go.dev/badge/github.com/imadmon/bandwidthcontroller.svg)](https://pkg.go.dev/github.com/imadmon/bandwidthcontroller)
![Zero Dependencies](https://img.shields.io/badge/dependencies-0-green.svg)

**Lightweight by design**: zero external dependencies - relies only on [`github.com/imadmon/limitedreader`](https://github.com/imadmon/limitedreader), which itself has none.

</br>

## Author

Created by [@IdanMadmon](https://github.com/imadmon) for developers who juggle unpredictable streams like pros.
