package controlplane

import (
	"context"
	"testing"

	"github.com/SWC-GEKO/beaver/internal/utils"
)

func TestControlPlane_Upload(t *testing.T) {
	cp, err := New("FUNCTIONS", ":4222", "test-registry")
	if err != nil {
		t.Errorf("creating control-plane failed with err: %v", err)
	}

	fnZip, err := utils.Zip("/Users/stahlco/GolandProjects/beaver/test/echo")
	if err != nil {
		t.Errorf("creating zip from path failed with err: %v", err)
	}

	if err := cp.Start(context.Background()); err != nil {
		t.Errorf("starting control-plane failed with err: %v", err)
	}

	_, err = cp.Upload("echo", fnZip, 4, 256)
	if err != nil {
		t.Errorf("uploading function failed with err: %v", err)
	}
}
