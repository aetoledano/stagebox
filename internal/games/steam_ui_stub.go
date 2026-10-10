//go:build !windows

package games

import "context"

func BackgroundSteamClient(ctx context.Context, steamExe string) error { return nil }
