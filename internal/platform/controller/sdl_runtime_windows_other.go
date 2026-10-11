//go:build windows && !amd64

package controller

import "fmt"

func sdlRuntimePath() (string, error) {
	return "", fmt.Errorf("bundled SDL3 runtime requires Windows amd64")
}
