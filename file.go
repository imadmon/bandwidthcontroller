package bandwidthcontroller

import "errors"

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

var InvalidFileSize = errors.New("invalid file size")
