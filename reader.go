package bandwidthcontroller

import (
	"fmt"
	"io"
	"sync/atomic"

	"github.com/imadmon/limitedreader"
)

type FileReadCloser struct {
	reader    *limitedreader.LimitedReader
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
	if newLimit > 0 {
		fmt.Printf("file new ratelimit: %v, will take %v seconds to finish, already read: %v\n", newLimit, ((1024-fr.GetBytesRead())/newLimit)+1, fr.GetBytesRead())
	} else {
		fmt.Printf("file new ratelimit: 0, already read: %v\n", fr.GetBytesRead())
	}
	fr.rateLimit.Store(newLimit)
	fr.reader.UpdateLimit(newLimit)
}

func (fr *FileReadCloser) GetRateLimit() int64 {
	return fr.rateLimit.Load()
}

func (fr *FileReadCloser) GetBytesRead() int64 {
	return fr.reader.GetTotalRead()
}
