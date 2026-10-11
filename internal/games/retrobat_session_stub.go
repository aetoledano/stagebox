//go:build !windows

package games

import (
	"context"
	"errors"
)

func LaunchRetroBatSession(ctx context.Context, exe string) (RunningGame, error) {
	return RunningGame{}, errors.New("RetroBat sessions are only supported on Windows")
}
