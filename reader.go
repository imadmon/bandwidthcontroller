package bandwidthcontroller

import (
	"io"
	"sync/atomic"
	"time"

	"github.com/imadmon/limitedreader"
)

type StreamReadCloser struct {
	reader             *limitedreader.LimitedReader
	callback           func() // called on Close
	lastReadEndingTime atomic.Int64
	Active             atomic.Bool
	Reading            atomic.Bool
}

func NewStreamReadCloser(r io.ReadCloser, limit int64, callback func()) *StreamReadCloser {
	sr := &StreamReadCloser{
		reader:   limitedreader.NewLimitedReadCloser(r, limit),
		callback: callback,
	}

	sr.Active.Store(true)
	sr.Reading.Store(false)
	sr.lastReadEndingTime.Store(time.Now().UnixNano())
	return sr
}

func (sr *StreamReadCloser) Read(p []byte) (n int, err error) {
	sr.Active.Store(true)
	sr.Reading.Store(true)
	n, err = sr.reader.Read(p)
	sr.Reading.Store(false)
	sr.lastReadEndingTime.Store(time.Now().UnixNano())
	return
}

func (sr *StreamReadCloser) Close() error {
	sr.Active.Store(false)
	err := sr.reader.Close()

	if sr.callback != nil {
		sr.callback()
	}

	return err
}

func (sr *StreamReadCloser) UpdateRateLimit(newLimit int64) {
	sr.reader.UpdateLimit(newLimit)
}

func (sr *StreamReadCloser) RateLimit() int64 {
	return sr.reader.GetLimit()
}

func (sr *StreamReadCloser) BytesRead() int64 {
	return sr.reader.GetTotalRead()
}
