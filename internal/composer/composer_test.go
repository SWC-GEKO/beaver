package composer

import (
	"log"
	"testing"

	"github.com/SWC-GEKO/beaver/internal/docker"
	"github.com/google/uuid"
)

func TestCalcTopics(t *testing.T) {
	n := 4
	for i := 0; i < n; i++ {
		log.Println(i, ": ", len(calcTopics(256, i, n, "test")))
	}
}

func TestNewComposer(t *testing.T) {
	c, err := NewComposer(DefaultConfig())
	if err != nil {
		t.Error(err)
	}

	log.Printf("%+v", c.Service)
}

func TestAddAndUp(t *testing.T) {
	c, err := NewComposer(DefaultConfig())
	if err != nil {
		t.Error(err)
	}

	functionName := uuid.New().String()
	f := &docker.Function{
		UniqueName:  functionName,
		ImageTag:    "stateless-processor:latest",
		Replication: 4,
		MaxShards:   256,
	}

	if err = c.Add(f); err != nil {
		t.Error(err)
	}

	if err = c.Up(functionName); err != nil {
		t.Error("up function failed with: ", err)
	}
}
