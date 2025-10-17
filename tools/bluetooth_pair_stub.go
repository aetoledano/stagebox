//go:build !windows

package tools

func AutoPairStart(whitelist string) error { return nil }
func AutoPairStop()                        {}
func AutoPairRunning() bool                { return false }
func AutoPairStatus() string               { return "off" }
