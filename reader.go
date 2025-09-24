package bandwidthcontroller

import (
	"io"

	"github.com/imadmon/limitedreader"
)

type StreamReadCloser struct {
	reader   *limitedreader.LimitedReader
	callback func() // called on Close
}

func NewStreamReadCloser(r io.ReadCloser, limit int64, callback func()) *StreamReadCloser {
	return &StreamReadCloser{
		reader:   limitedreader.NewLimitedReadCloser(r, limit),
		callback: callback,
	}
}

func (fr *StreamReadCloser) Read(p []byte) (n int, err error) {
	return fr.reader.Read(p)
}

func (fr *StreamReadCloser) Close() error {
	err := fr.reader.Close()

	if fr.callback != nil {
		fr.callback()
	}

	return err
}

func (fr *StreamReadCloser) UpdateRateLimit(newLimit int64) {
	fr.reader.UpdateLimit(newLimit)
}

func (fr *StreamReadCloser) GetRateLimit() int64 {
	return fr.reader.GetLimit()
}

func (fr *StreamReadCloser) GetBytesRead() int64 {
	return fr.reader.GetTotalRead()
}
