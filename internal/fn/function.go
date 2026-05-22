package fn

type Function interface {
	Start() error
	Stop() error
}
