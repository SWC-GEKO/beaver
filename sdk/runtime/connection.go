package runtime

import (
	"fmt"
	"net/http"
	"time"
)

// connection holds information about the connection to the control-plane.
// It manages all interaction between a user's program and the platform.
type connection struct {
	addr   string
	port   string
	client http.Client
}

func connect(addr, port string) (*connection, error) {
	client := http.Client{
		Transport: http.DefaultTransport,
		Timeout:   10 * time.Second,
	}

	url := fmt.Sprintf("http://%s:%s/health", addr, port)

	resp, err := client.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("response from control-plane not 200, got: %s", resp.Status)
	}

	return &connection{
		addr:   addr,
		port:   port,
		client: client,
	}, nil
}

func (c *connection) upload(rt *Runtime) error {
	return nil
}
