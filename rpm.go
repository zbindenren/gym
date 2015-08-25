package gym

type rpm struct {
	relPath      string
	checksumType string
	checksum     string
	size         int
	downloadID   int
}

func newRPM(relPath string, checksum string, checksumType string, size int) *rpm {
	return &rpm{
		relPath:      relPath,
		checksum:     checksum,
		checksumType: checksumType,
		size:         size,
	}
}
