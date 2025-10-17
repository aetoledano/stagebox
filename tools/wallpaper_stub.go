//go:build !windows

package tools

// StartWallpaper is a no-op on non-Windows.
// It returns a no-op stop function so callers can defer it safely.
func StartWallpaper(_ string, _ string) (func(), error) {
	return func() {}, nil
}
