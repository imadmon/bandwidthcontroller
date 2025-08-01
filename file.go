package bandwidthcontroller

type File struct {
	Reader *FileReadCloser
	Size   int64
}

func NewFile(reader *FileReadCloser, fileSize int64) *File {
	return &File{
		Reader: reader,
		Size:   fileSize,
	}
}
