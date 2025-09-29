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
dsfsdf

</br>

## Configurations
sdfsdf

</br>

## Monitoring
sdfsdf

</br>

## Dependencies

[`github.com/imadmon/limitedreader`](https://github.com/imadmon/limitedreader) (by the same author)

</br>

## Author

Created by [@IdanMadmon](https://github.com/imadmon) for developers who juggle unpredictable streams like pros.








</br></br></br></br></br></br></br></br></br></br>

## What it does

This library gives you the ability to:

- Dynamically add new data streams

- Get a custom io.Reader with built-in rate limiting

- Automatically reallocate bandwidth in real time as streams are added or change

- Avoid data starvation or backpressure regardless of stream order or size

Unlike naive rate limiters, `bandwidthcontroller` adapts _on-the-fly_ using the `limitedreader`'s ability to update rate limits at runtime.

</br>


## Why it matters

If all your incoming data is uniform - same size, format and timing, you probably don’t need this. You barely even need a protocol. But in real-world applications where stream sizes vary, formats differ, and you want to keep bandwidth under control while maximizing throughput - **this is the solution**.

It strikes the perfect balance between **efficiency** and **generality**, so you can stop worrying about data starvation or stuck streams.

</br>


## How it works

You initialize the controller with a total bandwidth cap (bytes/sec). Every time you add a stream, it returns a rate limited reader that adapts in real-time. Under the hood, the controller updates rate limits as streams are added or finished, redistributing available bandwidth as needed.

</br>


## Installation

``` Bash
go get github.com/imadmon/bandwidthcontroller
```

## Usage Example

``` Go
package main 

import (
 	"fmt"
    "io"
    "os"
    "strings"
    "github.com/imadmon/bandwidthcontroller"
)

func main() {
    // Initialize controller with target total bandwidth in bytes per second
    ctrl := bandwidthcontroller.NewSmartBandwidthController(1024 * 100) // 100KB/s
    
    // Create some example readers
    stream1 := strings.NewReader("some stream data here...")
    stream2 := strings.NewReader("another stream with more data...")
    
    // Register the readers with the controller
    r1 := ctrl.AddStream(stream1)
    r2 := ctrl.AddStream(stream2)
    
    // Consume data from the readers
    io.Copy(os.Stdout, r1) 	
    fmt.Println("\n---")
    io.Copy(os.Stdout, r2) 
}
```

</br>


## Roadmap

- Priority-based stream scheduling
- Monitoring and metrics (bandwidth usage, active streams)
- Optional per-stream bandwidth hints

</br>


## Dependencies

[`github.com/imadmon/limitedreader`](https://github.com/imadmon/limitedreader) (by the same author)

</br>


## Author

Created by [@IdanMadmon](https://github.com/imadmon) for developers who juggle unpredictable streams like pros.
