package contracts

type UploadRequest struct {
	Name          string `json:"name"`
	Zip           string `json:"zip"`
	Replication   int    `json:"replication"`
	VirtualShards int    `json:"virtualShards"`
}

type UploadResponse struct {
	Address string `json:"address"`
}
