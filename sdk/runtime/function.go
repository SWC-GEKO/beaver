package runtime

const (
	STATELESS = iota
	STATEFUL
)

type function struct {
	name         string
	path         string
	functionType int
}
