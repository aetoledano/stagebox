//go:build !windows

package bluetooth

func State() string { return "unknown" }
func EnsureUp()     {}
