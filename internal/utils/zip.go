package utils

import (
	"archive/zip"
	"bytes"
	"io"
	"log"
	"os"
	"os/exec"
	"os/user"
	"path"
	"path/filepath"
	"strings"
)

// Zip takes a filepath to a directory in which the fn-code is written and compresses it
func Zip(funcPath string) (string, error) {
	funcPath = expandPath(funcPath)

	cmdStr := "zip -r - ./* | base64 | tr -d '\n'"
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = funcPath

	var z bytes.Buffer
	cmd.Stdout = &z
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return z.String(), nil
}

func Unzip(zipPath string, p string) error {

	log.Printf("Unzipping %s to %s", zipPath, p)

	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return err
	}

	// extract zip
	for _, f := range archive.File {
		log.Printf("Extracting %s", f.Name)

		if f.FileInfo().IsDir() {
			pa := path.Join(p, f.Name)
			log.Printf("Creating directory %s in %s", f.Name, pa)

			err = os.MkdirAll(pa, 0777)
			if err != nil {
				return err
			}
			continue
		}

		// open file
		rc, err := f.Open()
		if err != nil {
			return err
		}

		// create file
		pa := path.Join(p, f.Name)
		// err = os.MkdirAll(path, 0777)
		// if err != nil {
		// return err
		// }

		// write file
		w, err := os.Create(pa)
		if err != nil {
			return err
		}

		// copy
		_, err = io.Copy(w, rc)
		if err != nil {
			return err
		}

		log.Printf("Extracted %s to %s", f.Name, pa)

		// close
		rc.Close()
		w.Close()
	}

	return nil
}

func expandPath(p string) string {
	if p == "" {
		panic("path provided is empty")
	}

	if strings.HasPrefix(p, "~") {
		usr, err := user.Current()
		if err == nil {
			if p == "~" {
				p = usr.HomeDir
			} else if strings.HasPrefix(p, "~/") {
				p = filepath.Join(usr.HomeDir, p[2:])
			}
		}
	}
	if !filepath.IsAbs(p) {
		abs, err := filepath.Abs(p)
		if err == nil {
			p = abs
		}
	}
	return filepath.Clean(p)
}
