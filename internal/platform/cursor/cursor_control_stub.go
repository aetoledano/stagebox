//go:build !windows

package cursor

import "time"

func SetCursorHidden(hidden bool) {}
func ForceCursorVisible()         {}
func ForceCursorHiddenFor(duration time.Duration) {
}
func HideSystemCursor()    {}
func RestoreSystemCursor() {}
