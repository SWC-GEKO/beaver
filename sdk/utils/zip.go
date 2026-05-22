package utils

import (
	"bytes"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"strings"
)

// Zip takes a filepath to a directory in which the function-code is written and compresses it
func Zip(funcPath string) (string, error) {
	funcPath = expandPath(funcPath)

	cmdStr := "zip -r - ./* | base64 | tr -d '\n'"
	cmd := exec.Command("bash", "-c", cmdStr)
	cmd.Dir = funcPath

	var zip bytes.Buffer
	cmd.Stdout = &zip
	err := cmd.Run()
	if err != nil {
		return "", err
	}

	return zip.String(), nil
}

func Unzip(zipPath string) (*os.File, error) {
	//TODO: implement me
	panic("implement me...")
	return nil, nil
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
