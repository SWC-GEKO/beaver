package controlplane

import (
	"errors"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"

	"github.com/goccy/go-yaml"
)

type Registry struct {
	dir string
}

type FunctionRecord struct {
	UniqueName     string `yaml:"uniqueName"`
	ProcessorImage string `yaml:"processorImage"`
	Replication    int    `yaml:"replication"`
	VirtualShards  int    `yaml:"virtualShards"`
}

func NewRegistry(dir string) (*Registry, error) {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	log.Println("")
	return &Registry{dir: dir}, nil
}

func (r *Registry) path(uniqueName string) string {
	return filepath.Join(r.dir, uniqueName+".yaml")
}

func (r *Registry) Save(rec FunctionRecord) error {
	data, err := yaml.Marshal(rec)
	if err != nil {
		return fmt.Errorf("marshalling function record failed: %w", err)
	}

	tmp, err := os.CreateTemp(r.dir, "."+rec.UniqueName+"-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer os.Remove(tmpPath)

	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}

	if err := tmp.Close(); err != nil {
		return err
	}

	path := r.path(rec.UniqueName)

	if err := os.Rename(tmpPath, path); err != nil {
		return fmt.Errorf("renaming registry record failed: %w", err)
	}

	return nil
}

func (r *Registry) Delete(uniqueName string) error {
	err := os.Remove(r.path(uniqueName))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}

	return err
}

func (r *Registry) List() ([]FunctionRecord, error) {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return nil, err
	}

	var records []FunctionRecord
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(r.dir, e.Name()))
		if err != nil {
			log.Printf("reading file: %s failed with err: %v", e.Name(), err)
			continue
		}

		var rec FunctionRecord
		if err := yaml.Unmarshal(data, &rec); err != nil {
			log.Printf("unmarshalling file: %s failed with err: %v", e.Name(), err)
			continue
		}

		records = append(records, rec)
	}

	return records, nil
}
