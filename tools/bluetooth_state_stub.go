//go:build !windows

package tools

func BluetoothState() string { return "unknown" }
func EnsureBluetoothUp()     {}
