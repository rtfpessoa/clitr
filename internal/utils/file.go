package utils

import (
	"os"
	"path/filepath"
	"strings"
)

func ResolvePath(path string) (string, error) {
	dir, err := expandHome(path)
	if err != nil {
		return "", err
	}

	return canonicalPath(dir)
}

func expandHome(path string) (string, error) {
	if strings.HasPrefix(path, "~/") {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", err
		}
		path = filepath.Join(home, path[2:])
	}
	return path, nil
}

func canonicalPath(path string) (string, error) {
	absPath, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	canonicPath := filepath.Clean(absPath)

	return canonicPath, nil
}
