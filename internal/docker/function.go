package docker

type Function struct {
	UniqueName string
	FilePath   string // Could be changed to a S3/ObjectStoreAddr
	Image
}

type Image struct {
}
