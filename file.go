package bandwidthcontroller

type File struct {
	Reader *FileReader
	Size   int64
}

func NewFile(reader *FileReader, fileSize int64) *File {
	return &File{
		Reader: reader,
		Size:   fileSize,
	}
}
