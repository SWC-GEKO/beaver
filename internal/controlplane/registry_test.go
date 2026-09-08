package controlplane

import (
	"log"
	"testing"
)

func ensureRegistry() *Registry {
	r, err := NewRegistry("test-registry")
	if err != nil {
		log.Fatalln(err)
	}

	return r
}

func TestRegistry_Save(t *testing.T) {
	r := ensureRegistry()

	record := FunctionRecord{
		UniqueName:     "test_save_2",
		ProcessorImage: "stateless-processor:latest",
		Replication:    8,
		VirtualShards:  256,
	}

	if err := r.Save(record); err != nil {
		t.Errorf("saving record failed with err: %v", err)
	}
}

func TestRegistry_Delete(t *testing.T) {
	r := ensureRegistry()

	if err := r.Delete("test_save_1"); err != nil {
		t.Errorf("deleting test-file failed with err: %v", err)
	}
}

func TestRegistry_List(t *testing.T) {
	r := ensureRegistry()

	records, err := r.List()
	if err != nil {
		t.Errorf("listing all records failed with err: %v", err)
	}

	for _, r := range records {
		log.Println(r)
	}
}
