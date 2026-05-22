package sdk

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/SWC-GEKO/beaver/internal/utils"
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
	zip, err := utils.Zip(rt.function.path)
	if err != nil {
		return err
	}

	data := struct {
		Name string `json:"name"`
		Type int    `json:"type"`
		Zip  string `json:"zip"`
	}{
		Name: rt.function.name,
		Type: rt.function.functionType,
		Zip:  zip,
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return err
	}
	var buf bytes.Buffer
	buf.Write(jsonData)

	url := fmt.Sprintf("http://%s:%s/upload", c.addr, c.port)
	resp, err := c.client.Post(url, "application/json", &buf)
	if err != nil {
		return err
	}

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed with status: %s", resp.Status)
	}

	return nil
}
