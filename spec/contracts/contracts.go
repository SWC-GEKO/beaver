package contracts

type FunctionType int

const (
	STATELESS FunctionType = iota
	STATEFUL
)

type UploadRequest struct {
	Name string `json:"name"`
	Type int    `json:"type"`
	Zip  string `json:"zip"`
	// TODO: add configuration variables
}
