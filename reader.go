package bandwidthcontroller

import (
	"io"
	"sync/atomic"

	"github.com/imadmon/limitedreader"
)

type FileReadCloser struct {
	reader    *limitedreader.LimitedReader
	bytesRead atomic.Int64
	rateLimit atomic.Int64
	callback  func() // called on Close
}

func NewFileReadCloser(r io.ReadCloser, limit int64, callback func()) *FileReadCloser {
	return &FileReadCloser{
		reader:   limitedreader.NewLimitedReadCloser(r, limit),
		callback: callback,
	}
}

func (fr *FileReadCloser) Read(p []byte) (n int, err error) {
	n, err = fr.reader.Read(p)
	fr.bytesRead.Add(int64(n))
	return n, err
}

func (fr *FileReadCloser) Close() error {
	err := fr.reader.Close()

	if fr.callback != nil {
		fr.callback()
	}

	return err
}

func (fr *FileReadCloser) UpdateRateLimit(newLimit int64) {
	fr.rateLimit.Store(newLimit)
	fr.reader.UpdateLimit(newLimit)
}

func (fr *FileReadCloser) GetRateLimit() int64 {
	return fr.rateLimit.Load()
}

func (fr *FileReadCloser) GetBytesRead() int64 {
	return fr.bytesRead.Load()
}
