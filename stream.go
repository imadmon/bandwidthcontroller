package bandwidthcontroller

type Stream struct {
	*StreamReadCloser
	Size int64
}

func NewStream(reader *StreamReadCloser, streamSize int64) *Stream {
	return &Stream{
		reader,
		streamSize,
	}
}
