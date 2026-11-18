package main

import (
	"archive/zip"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// runBeaverUpload does what sdk.Runtime.Start does (zip dir -> POST /upload) but
// returns the unique function name from the response body, which the SDK drops.
// stdout: ONLY the unique name (so scripts can capture it); logs go to stderr.
func runBeaverUpload(args []string) error {
	f := flag.NewFlagSet("beaver-upload", flag.ExitOnError)
	cp := f.String("cp", "http://localhost:8080", "control plane base URL")
	dir := f.String("dir", "", "directory with the function code (go.mod + *.go)")
	name := f.String("name", "stats", "function name")
	repl := f.Int("replication", 4, "number of processor containers (physical shards)")
	vsh := f.Int("vshards", 128, "number of virtual shards")
	_ = f.Parse(args)
	if *dir == "" {
		return errors.New("need -dir")
	}

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	err := filepath.WalkDir(*dir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(*dir, p)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			_, err := zw.Create(rel + "/")
			return err
		}
		w, err := zw.Create(filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(p)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if err != nil {
		return err
	}
	if err := zw.Close(); err != nil {
		return err
	}

	req := map[string]any{
		"name":          *name,
		"zip":           base64.StdEncoding.EncodeToString(buf.Bytes()),
		"replication":   *repl,
		"virtualShards": *vsh,
	}
	b, _ := json.Marshal(req)

	c := &http.Client{Timeout: 15 * time.Minute} // includes the docker image build
	fmt.Fprintf(os.Stderr, "uploading %s (replication=%d vshards=%d) to %s ...\n", *dir, *repl, *vsh, *cp)
	resp, err := c.Post(strings.TrimRight(*cp, "/")+"/upload", "application/json", bytes.NewReader(b))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("upload failed: %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	unique := strings.TrimSpace(string(body))
	if unique == "" {
		return errors.New("control plane returned an empty function name")
	}
	fmt.Println(unique)
	return nil
}
