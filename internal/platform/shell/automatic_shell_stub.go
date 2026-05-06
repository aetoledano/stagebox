//go:build !windows

package shell

func EnsureSelfAsShell() error { return nil }
