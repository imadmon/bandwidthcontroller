package bandwidthcontroller

type Stream struct {
	Reader *StreamReadCloser
	Size   int64
}

func NewStream(reader *StreamReadCloser, streamSize int64) *Stream {
	return &Stream{
		Reader: reader,
		Size:   streamSize,
	}
}
