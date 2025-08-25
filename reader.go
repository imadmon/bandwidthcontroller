package bandwidthcontroller

import (
	"io"

	"github.com/imadmon/limitedreader"
)

type FileReadCloser struct {
	reader   *limitedreader.LimitedReader
	callback func() // called on Close
}

func NewFileReadCloser(r io.ReadCloser, limit int64, callback func()) *FileReadCloser {
	return &FileReadCloser{
		reader:   limitedreader.NewLimitedReadCloser(r, limit),
		callback: callback,
	}
}

func (fr *FileReadCloser) Read(p []byte) (n int, err error) {
	return fr.reader.Read(p)
}

func (fr *FileReadCloser) Close() error {
	err := fr.reader.Close()

	if fr.callback != nil {
		fr.callback()
	}

	return err
}

func (fr *FileReadCloser) UpdateRateLimit(newLimit int64) {
	fr.reader.UpdateLimit(newLimit)
}

func (fr *FileReadCloser) GetRateLimit() int64 {
	return fr.reader.GetLimit()
}

func (fr *FileReadCloser) GetBytesRead() int64 {
	return fr.reader.GetTotalRead()
}
