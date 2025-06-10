# Smart Bandwidth Controller

Smart Bandwidth Controller is a high performance Go library that enables **efficient and adaptive bandwidth usage** across multiple concurrent readers/streams. It leverages the dynamic rate limiting capabilities of [`github.com/imadmon/limitedreader`](https://github.com/imadmon/limitedreader) to adjust bandwidth on the fly depending on your stream consumption.

This library is designed for scenarios where data streams vary in size and format, making traditional static rate limiting approaches inefficient or prone to bottlenecks.

</br>


## What it does

This library gives you the ability to:

- Dynamically add new data streams

- Get a custom io.Reader with built-in rate limiting

- Automatically reallocate bandwidth in real time as streams are added or change

- Avoid data starvation or backpressure regardless of stream order or size

Unlike naive rate limiters, `bandwidthcontroller` adapts _on-the-fly_ using the `limitedreader`'s ability to update rate limits at runtime.

</br>


## Why it matters

If all your incoming data is uniform — same size, format and timing, you probably don’t need this. You barely even need a protocol. But in real-world applications where stream sizes vary, formats differ, and you want to keep bandwidth under control while maximizing throughput — **this is the solution**.

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
