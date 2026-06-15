package controlplane

import "errors"

var (
	ErrAlreadyExists            = errors.New("function is already registered")
	ErrNotFound                 = errors.New("function not found")
	ErrBuildImage               = errors.New("building image failed")
	ErrAlreadyRunning           = errors.New("function is already running")
	ErrFunctionDeploymentFailed = errors.New("function-deployment failed")
)
