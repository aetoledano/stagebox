//go:build !windows

package bluetooth

func AnyConnectedDevice() bool      { return false }
func ConnectedControllerCount() int { return 0 }
