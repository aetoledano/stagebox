//go:build !windows

package display

func ForcePrimary1080pOrBest() error {
	return nil
}
