package docker

type Function struct {
	UniqueName string
	ImageTag   string

	Replication int
	MaxShards   int
}
